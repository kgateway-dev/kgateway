package ec2

import (
	"context"
	"crypto/md5"
	"fmt"
	"strings"
	"time"

	"github.com/solo-io/go-utils/errors"

	"golang.org/x/sync/errgroup"

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
	upstreams         map[core.ResourceRef]*glooec2.UpstreamSpec
	watchContext      context.Context
	secretClient      v1.SecretClient
	refreshRate       time.Duration
	writeNamespace    string
	ec2InstanceLister Ec2InstanceLister
}

func newEndpointsWatcher(watchCtx context.Context, writeNamespace string, upstreams v1.UpstreamList, secretClient v1.SecretClient, parentRefreshRate time.Duration) *edsWatcher {
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
		// consolidate api calls into batches by credential spec
		credBatch, err := c.getInstancesForCredentials(secrets)
		if err != nil {
			errs <- err
			return
		}
		// apply filters to the instance batches
		var allEndpoints v1.EndpointList
		for upstreamRef, upstreamSpec := range c.upstreams {
			instancesForUpstream, err := credBatch.filterEndpointsForUpstream(upstreamSpec)
			if err != nil {
				errs <- err
				return
			}
			endpointsForUpstream := c.convertInstancesToEndpoints(upstreamRef, upstreamSpec, instancesForUpstream)
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

var awsCallTimeout = 10 * time.Second

func (c *edsWatcher) getInstancesForCredentials(secrets v1.SecretList) (*credentialBatch, error) {
	// 1. group upstreams by secret ref
	credMap := newCredentialBatch(c.watchContext, secrets)
	for upstreamRef, upstreamSpec := range c.upstreams {
		if err := credMap.addUpstreamSpec(upstreamRef, upstreamSpec); err != nil {
			return nil, err
		}
	}
	contextutils.LoggerFrom(c.watchContext).Debugw("batched credentials", zap.Any("count", len(credMap.resources)))
	// 2. query the AWS API for each credential set
	errChan := make(chan error)
	eg := errgroup.Group{}
	go func() {
		// first copy from map to a slice in order to avoid a race condition
		var credentialSpecs []credentialSpec
		for credentialSpec := range credMap.resources {
			credentialSpecs = append(credentialSpecs, credentialSpec)
		}
		for _, cSpec := range credentialSpecs {
			eg.Go(func() error {
				instances, err := c.ec2InstanceLister.ListForCredentials(c.watchContext, cSpec.region, cSpec.secretRef, secrets)
				if err != nil {
					return err
				}
				if err := credMap.addInstances(cSpec, instances); err != nil {
					return err
				}
				return nil
			})
		}
		errChan <- eg.Wait()
	}()
	select {
	case err := <-errChan:
		if err != nil {
			return nil, ListCredentialError(err)
		}
		return credMap, nil
	case <-time.After(awsCallTimeout):
		return nil, TimeoutError
	}
}

const defaultPort = 80

func (c *edsWatcher) convertInstancesToEndpoints(upstreamRef core.ResourceRef, ec2UpstreamSpec *glooec2.UpstreamSpec, ec2InstancesForUpstream []*ec2.Instance) v1.EndpointList {
	var list v1.EndpointList
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
			Upstreams: []*core.ResourceRef{&upstreamRef},
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

func generateName(upstreamRef core.ResourceRef, publicIpAddress string) string {
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

var (
	ListCredentialError = func(err error) error {
		return errors.Wrapf(err, "unable to list credentials")
	}

	TimeoutError = errors.New("timed out while waiting for response from aws")
)
