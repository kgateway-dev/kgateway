package ec2

import (
	"context"
	"fmt"
	"time"

	"github.com/solo-io/go-utils/kubeutils"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws/ec2/awslister"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws/ec2/awscache"

	"go.uber.org/zap"

	"github.com/solo-io/go-utils/contextutils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	return newEndpointsWatcher(opts.Ctx, writeNamespace, upstreams, p.secretClient, opts.RefreshRate).poll()
}

type edsWatcher struct {
	upstreams         map[core.ResourceRef]*glooec2.UpstreamSpecRef
	watchContext      context.Context
	secretClient      v1.SecretClient
	refreshRate       time.Duration
	writeNamespace    string
	ec2InstanceLister awslister.Ec2InstanceLister
}

func newEndpointsWatcher(watchCtx context.Context, writeNamespace string, upstreams v1.UpstreamList, secretClient v1.SecretClient, parentRefreshRate time.Duration) *edsWatcher {
	upstreamSpecs := make(map[core.ResourceRef]*glooec2.UpstreamSpecRef)
	for _, us := range upstreams {
		ec2Upstream, ok := us.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_AwsEc2)
		// only care about ec2 upstreams
		if !ok {
			continue
		}
		ref := us.Metadata.Ref()
		upstreamSpecs[ref] = &glooec2.UpstreamSpecRef{
			Spec: ec2Upstream.AwsEc2,
			Ref:  ref,
		}
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

func (c *edsWatcher) poll() (<-chan v1.EndpointList, <-chan error, error) {
	endpointsChan := make(chan v1.EndpointList)
	errs := make(chan error)
	updateResourceList := func() {
		// TODO(mitchdraft) refine the secret ingestion strategy. TBD if the AWS secret will come from a crd, env var, or file
		tmpTODOAllNamespaces := metav1.NamespaceAll
		secrets, err := c.secretClient.List(tmpTODOAllNamespaces, clients.ListOpts{Ctx: c.watchContext})
		if err != nil {
			errs <- err
			return
		}
		// query the source of truth and build a local representation of the EC2 instances, grouped by credentials
		store, err := c.buildCache(secrets)
		if err != nil {
			errs <- err
			return
		}
		// apply filters to the instance batches
		var allEndpoints v1.EndpointList
		for _, upstreamSpecRef := range c.upstreams {
			instancesForUpstream, err := store.FilterEndpointsForUpstream(upstreamSpecRef)
			if err != nil {
				errs <- err
				return
			}
			endpointsForUpstream := c.convertInstancesToEndpoints(upstreamSpecRef, instancesForUpstream)
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

// call AWS APIs
func (c *edsWatcher) buildCache(secrets v1.SecretList) (*awscache.Cache, error) {
	return awscache.New(c.watchContext, secrets, c.upstreams, c.ec2InstanceLister)
}

const defaultPort = 80

func (c *edsWatcher) convertInstancesToEndpoints(upstream *glooec2.UpstreamSpecRef, ec2InstancesForUpstream []*ec2.Instance) v1.EndpointList {
	var list v1.EndpointList
	for _, instance := range ec2InstancesForUpstream {
		ipAddr := instance.PrivateIpAddress
		if upstream.Spec.PublicIp {
			ipAddr = instance.PublicIpAddress
		}
		port := upstream.Spec.GetPort()
		if port == 0 {
			port = defaultPort
		}
		endpoint := &v1.Endpoint{
			Upstreams: []*core.ResourceRef{&upstream.Ref},
			Address:   aws.StringValue(ipAddr),
			Port:      upstream.Spec.GetPort(),
			Metadata: core.Metadata{
				Name:      generateName(upstream.Ref, aws.StringValue(ipAddr)),
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

func generateName(upstreamRef core.ResourceRef, publicIpAddress string) string {
	return kubeutils.SanitizeNameV2(fmt.Sprintf("%v-%v-%v", ec2EndpointNamePrefix, upstreamRef.String(), publicIpAddress))
}
