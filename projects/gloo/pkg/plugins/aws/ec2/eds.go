package ec2

import (
	"context"
	"crypto/md5"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

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
	upstreams         map[core.ResourceRef]*glooec2.UpstreamSpec
	watchContext      context.Context
	secretClient      *v1.SecretClient
	refreshRate       time.Duration
	writeNamespace    string
	ec2InstanceLister Ec2InstanceLister
}

// TODO implement an alternate version of this for tests
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
		upstreams:         upstreamSpecs,
		watchContext:      watchCtx,
		secretClient:      secretClient,
		refreshRate:       getRefreshRate(parentRefreshRate),
		writeNamespace:    writeNamespace,
		ec2InstanceLister: NewEc2InstanceLister(),
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
		if c.secretClient == nil || *c.secretClient == nil {
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
	ec2InstancesForUpstream, err := c.ec2InstanceLister.ListForCredentials(c.watchContext, ec2Upstream, secrets)
	if err != nil {
		return nil, err
	}
	contextutils.LoggerFrom(c.watchContext).Debugw("list of EC2 instances",
		zap.Any("ec2_instances", ec2InstancesForUpstream),
		zap.Any("upstreamRef", upstreamRef))
	return c.convertInstancesToEndpoints(upstreamRef, ec2Upstream, ec2InstancesForUpstream), nil
}

const defaultPort = 80

func (c *edsWatcher) convertInstancesToEndpoints(upstreamRef *core.ResourceRef, ec2UpstreamSpec *glooec2.UpstreamSpec, ec2InstancesForUpstream []*ec2.Instance) v1.EndpointList {
	var list v1.EndpointList
	contextutils.LoggerFrom(c.watchContext).Debugw("begin listing EC2 endpoints in CITE")
	for _, instance := range ec2InstancesForUpstream {
		ipAddr := instance.PrivateIpAddress
		if ec2UpstreamSpec.PublicIp {
			ipAddr = instance.PublicIpAddress
		}
		port := ec2UpstreamSpec.GetPort()
		if port == 0 {
			port = defaultPort
		}
		endpoint := &v1.Endpoint{
			Upstreams: []*core.ResourceRef{upstreamRef},
			Address:   aws.StringValue(ipAddr),
			Port:      ec2UpstreamSpec.GetPort(),
			Metadata: core.Metadata{
				Name:      generateName(upstreamRef, aws.StringValue(ipAddr)),
				Namespace: c.writeNamespace,
			},
		}
		contextutils.LoggerFrom(c.watchContext).Debugw("EC2 endpoint", zap.Any("ep", endpoint))
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
