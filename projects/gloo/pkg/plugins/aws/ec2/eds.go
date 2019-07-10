package ec2

import (
	"context"
	"crypto/md5"
	"fmt"
	"strings"
	"sync"
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
		// consolidate api calls into batches by credential spec
		credBatch, err := c.getInstancesForCredentials(secrets)
		if err != nil {
			errs <- err
			return
		}
		// apply filters to the instance batches
		var allEndpoints v1.EndpointList
		for upstreamRef, upstreamSpec := range c.upstreams {
			instancesForUpstream := credBatch.filterEndpointsForUpstream(upstreamSpec)
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

// credentialResources represents the resources available to a given credential spec (secret and aws region pair)
type credentialResources struct {
	// all upstreams having the same credential spec (secret and aws region) will be listed here
	// key: upstream resource ref, value: ec2 upstream spec
	upstreams map[core.ResourceRef]*glooec2.UpstreamSpec

	// instances contains all of the EC2 instances available for the given credential spec
	instances []*ec2.Instance
	// instanceFilterMaps contains one filter map for each instance
	// indices correspond: instanceFilterMap[i] == filterMap(instance[i])
	// we store the filter map so that it can be reused across upstreams when determining if a given instance should be
	// associated with a given upstream
	instanceFilterMaps []filterMap
}

// a filterMap is created for each EC2 instance so we can efficiently filter the instances associated with a given
// upstream's filter spec
// filter maps are generated from tag lists, the keys are the tag keys, the values are the tag values
type filterMap map[string]string

func newCredentialResources() *credentialResources {
	return &credentialResources{
		upstreams: make(map[core.ResourceRef]*glooec2.UpstreamSpec),
	}
}

// a credential batch stores the resources available to a given credentials
// it is possible that there will be duplicate resource records, for example, if two credentials have access to the same
// resource, then that resource will be present in both credentialResources entries. For simplicity, we will let that be.
type credentialBatch struct {
	resources map[credentialSpec]*credentialResources
	secrets   v1.SecretList
	mutex     sync.Mutex
}

// a credential spec represents an AWS client's view into AWS resources
// we expect multiple upstreams to share the same view (so we batch the queries and apply filters locally)
type credentialSpec struct {
	// secretRef identifies the AWS secret that should be used to authenticate the client
	secretRef core.ResourceRef
	// region is the AWS region where our resources live
	region string
}

func credentialSpecFromUpstreamSpec(ec2Spec *glooec2.UpstreamSpec) credentialSpec {
	return credentialSpec{
		secretRef: ec2Spec.SecretRef,
		region:    ec2Spec.Region,
	}
}

func newCredentialBatch(secrets v1.SecretList) *credentialBatch {
	m := &credentialBatch{
		secrets: secrets,
	}
	m.resources = make(map[credentialSpec]*credentialResources)
	return m
}

func (c *credentialBatch) addUpstreamSpec(upstreamRef core.ResourceRef, ec2Spec *glooec2.UpstreamSpec) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	key := credentialSpecFromUpstreamSpec(ec2Spec)

	if v, ok := c.resources[key]; ok {
		v.upstreams[upstreamRef] = ec2Spec
	} else {
		cr := newCredentialResources()
		cr.upstreams[upstreamRef] = ec2Spec
		c.resources[key] = cr
	}
	return nil
}

func generateFilterMap(instance *ec2.Instance) filterMap {
	m := make(filterMap)
	for _, t := range instance.Tags {
		m[aws.StringValue(t.Key)] = aws.StringValue(t.Value)
	}
	return m
}

func generateFilterMaps(instances []*ec2.Instance) []filterMap {
	var maps []filterMap
	for _, instance := range instances {
		maps = append(maps, generateFilterMap(instance))
	}
	return maps
}

func (c *credentialBatch) addInstances(credentialSpec credentialSpec, instances []*ec2.Instance) {
	filterMaps := generateFilterMaps(instances)
	c.mutex.Lock()
	c.resources[credentialSpec].instances = instances
	c.resources[credentialSpec].instanceFilterMaps = filterMaps
	c.mutex.Unlock()
}

var awsCallTimeout = 10 * time.Second

func (c *edsWatcher) getInstancesForCredentials(secrets v1.SecretList) (*credentialBatch, error) {
	// 1. group upstreams by secret ref
	credMap := newCredentialBatch(secrets)
	for upstreamRef, upstreamSpec := range c.upstreams {
		if err := credMap.addUpstreamSpec(upstreamRef, upstreamSpec); err != nil {
			return nil, err
		}
	}
	contextutils.LoggerFrom(c.watchContext).Debugw("batched credentials", zap.Any("count", len(credMap.resources)))
	// 2. query the AWS API for each credential set
	wg := sync.WaitGroup{}
	waitChan := make(chan struct{})
	errChan := make(chan error)
	go func() {
		for credentialSpec := range credMap.resources {
			wg.Add(1)
			go func() {
				instances, err := c.ec2InstanceLister.ListForCredentials(c.watchContext, credentialSpec.region, credentialSpec.secretRef, secrets)
				if err != nil {
					errChan <- err
				}
				credMap.addInstances(credentialSpec, instances)
				wg.Done()
			}()
		}
		wg.Wait()
		close(waitChan)
	}()
	select {
	case <-waitChan:
		return credMap, nil
	case <-time.After(awsCallTimeout):
		return nil, fmt.Errorf("timed out while waiting for response from aws")
	}
}

func (c *credentialBatch) filterEndpointsForUpstream(ec2Upstream *glooec2.UpstreamSpec) []*ec2.Instance {
	credSpec := credentialSpecFromUpstreamSpec(ec2Upstream)
	credRes, ok := c.resources[credSpec]
	if !ok {
		// This should never happen
		contextutils.LoggerFrom(context.TODO()).Errorw("bad map construction in EC2 filter")
	}
	var list []*ec2.Instance
	// sweep through each filter map, if all the upstream's filters are matched, add the corresponding instance to the list
	for i, fm := range credRes.instanceFilterMaps {
		candidateInstance := credRes.instances[i]
		for _, filter := range ec2Upstream.Filters {
			switch filterSpec := filter.Spec.(type) {
			case *glooec2.Filter_Key:
				if _, ok := fm[filterSpec.Key]; ok {
					list = append(list, candidateInstance)
				}
			case *glooec2.Filter_KvPair_:
				if val, ok := fm[filterSpec.KvPair.Key]; ok && val == filterSpec.KvPair.Value {
					list = append(list, candidateInstance)
				}
			}
		}
	}
	return list
}

const defaultPort = 80

func (c *edsWatcher) convertInstancesToEndpoints(upstreamRef core.ResourceRef, ec2UpstreamSpec *glooec2.UpstreamSpec, ec2InstancesForUpstream []*ec2.Instance) v1.EndpointList {
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
