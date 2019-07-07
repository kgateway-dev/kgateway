package ec2

import (
	"context"
	"crypto/md5"
	"fmt"
	"strings"
	"time"

	"github.com/solo-io/go-utils/contextutils"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws/glooec2"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// EDS API
// start the EDS watch which sends a new list of endpoints on any change
func (p *plugin) WatchEndpoints(writeNamespace string, upstreams v1.UpstreamList, opts clients.WatchOpts) (<-chan v1.EndpointList, <-chan error, error) {
	contextutils.LoggerFrom(opts.Ctx).Infow("calling WatchEndpoints on EC2")
	return newEndpointsWatcher(opts.Ctx, writeNamespace, upstreams, &p.secretClient, opts.RefreshRate).poll()
}

type edsWatcher struct {
	upstreams      map[core.ResourceRef]*glooec2.UpstreamSpec
	watchContext   context.Context
	secretClient   *v1.SecretClient
	refreshRate    time.Duration
	writeNamespace string
}

func newEndpointsWatcher(watchCtx context.Context, writeNamespace string, upstreams v1.UpstreamList, secretClient *v1.SecretClient, parentRefreshRate time.Duration) *edsWatcher {
	upstreamSpecs := make(map[core.ResourceRef]*glooec2.UpstreamSpec)
	for _, us := range upstreams {
		ec2Upstream, ok := us.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_AwsEc2)
		// only care about ec2 upstreams
		if !ok {
			continue
		}
		upstreamSpecs[us.Metadata.Ref()] = ec2Upstream.AwsEc2
	}
	return &edsWatcher{
		upstreams:      upstreamSpecs,
		watchContext:   watchCtx,
		secretClient:   secretClient,
		refreshRate:    getRefreshRate(parentRefreshRate),
		writeNamespace: writeNamespace,
	}
}

const minRefreshRate = 30 * time.Second

// unlike the other plugins, we are calling an external service (AWS) during our watches.
// since we don't expect EC2 changes to happen very frequently, and to avoid ratelimit concerns, we set a minimum
// refresh rate of thirty seconds
func getRefreshRate(parentRefreshRate time.Duration) time.Duration {
	if parentRefreshRate < minRefreshRate {
		return minRefreshRate
	}
	return parentRefreshRate
}

// NOTE - optimization opportunity:
// do a "master-credential" poll first, if there are no changes there, do not do the sub-credential polls

// need to poll for each upstream, since each will have a different view
func (c *edsWatcher) poll() (<-chan v1.EndpointList, <-chan error, error) {

	endpointsChan := make(chan v1.EndpointList)
	errs := make(chan error)
	updateResourceList := func() {
		tmpTODOAllNamespaces := ""
		if c.secretClient == nil {
			contextutils.LoggerFrom(c.watchContext).Infow("waiting for ec2 plugin to init")
			return
		}
		secrets, err := (*c.secretClient).List(tmpTODOAllNamespaces, clients.ListOpts{})
		if err != nil {
			errs <- err
			return
		}
		var allEndpoints v1.EndpointList
		for upstreamRef, upstreamSpec := range c.upstreams {
			// TODO - call this asynchronously
			// TODO - add timeouts
			endpointsForUpstream, err := c.getEndpointsForUpstream(&upstreamRef, upstreamSpec, secrets)
			if err != nil {
				errs <- err
				return
			}
			allEndpoints = append(allEndpoints, endpointsForUpstream...)
		}

		select {
		case <-c.watchContext.Done():
			return
		case endpointsChan <- allEndpoints:
		}
	}

	go func() {
		defer close(endpointsChan)
		defer close(errs)

		updateResourceList()
		ticker := time.NewTicker(c.refreshRate)
		defer ticker.Stop()

		for {
			select {
			case _, ok := <-ticker.C:
				if !ok {
					return
				}
				updateResourceList()
			case <-c.watchContext.Done():
				return
			}
		}

	}()
	return endpointsChan, errs, nil
}
func (c *edsWatcher) getEndpointsForUpstream(upstreamRef *core.ResourceRef, ec2Upstream *glooec2.UpstreamSpec, secrets v1.SecretList) (v1.EndpointList, error) {
	session, err := GetEc2Session(ec2Upstream, secrets)
	if err != nil {
		return nil, err
	}
	ec2InstancesForUpstream, err := ListEc2InstancesForCredentials(c.watchContext, session, ec2Upstream)
	if err != nil {
		return nil, err
	}
	return c.convertInstancesToEndpoints(upstreamRef, ec2InstancesForUpstream), nil
}

func (c *edsWatcher) convertInstancesToEndpoints(upstreamRef *core.ResourceRef, ec2InstancesForUpstream []*ec2.Instance) v1.EndpointList {
	// TODO - get port from upstream, instance tag, or elsewhere
	// using 80 for now since it is a common default
	var tmpTODOPort uint32 = 80
	var list v1.EndpointList
	for _, instance := range ec2InstancesForUpstream {
		endpoint := &v1.Endpoint{
			Upstreams: []*core.ResourceRef{upstreamRef},
			Address:   aws.StringValue(instance.PublicIpAddress),
			Port:      tmpTODOPort,
			Metadata: core.Metadata{
				Name:      generateName(upstreamRef, aws.StringValue(instance.PublicIpAddress)),
				Namespace: c.writeNamespace,
			},
		}
		list = append(list, endpoint)
	}
	return list
}

// TODO (separate pr) - update the EDS interface to include a registration function which would ensure uniqueness among prefixes
// ... also include a function to ensure that the endpoint name conforms to the spec (is unique, begins with expected prefix)
const ec2EndpointNamePrefix = "ec2"

func generateName(upstreamRef *core.ResourceRef, publicIpAddress string) string {
	return SanitizeName(fmt.Sprintf("%v-%v-%v", ec2EndpointNamePrefix, upstreamRef.String(), publicIpAddress))
}

// use function from go-utils when update merges
// DEPRECATED
func SanitizeName(name string) string {
	name = strings.Replace(name, "*", "-", -1)
	name = strings.Replace(name, "/", "-", -1)
	name = strings.Replace(name, ".", "-", -1)
	name = strings.Replace(name, "[", "", -1)
	name = strings.Replace(name, "]", "", -1)
	name = strings.Replace(name, ":", "-", -1)
	name = strings.Replace(name, " ", "-", -1)
	name = strings.Replace(name, "\n", "", -1)
	// This is the new content
	// begin diff
	name = strings.Replace(name, "\"", "", -1)
	// end diff
	if len(name) > 63 {
		hash := md5.Sum([]byte(name))
		name = fmt.Sprintf("%s-%x", name[:31], hash)
		name = name[:63]
	}
	name = strings.Replace(name, ".", "-", -1)
	name = strings.ToLower(name)
	return name
}
