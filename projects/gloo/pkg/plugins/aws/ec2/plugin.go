package ec2

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/solo-io/go-utils/kubeutils"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws/glooec2"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	aws2 "github.com/solo-io/gloo/projects/gloo/pkg/utils/aws"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"go.uber.org/zap"
)

// TEMP - TODO REMOVE
func main() {
	ctx := context.Background()
	tmpUpstream := &glooec2.UpstreamSpec{
		Region:    "us-east-1",
		SecretRef: core.ResourceRef{},
		Filters: []*glooec2.Filter{{
			Spec: &glooec2.Filter_KvPair_{
				KvPair: &glooec2.Filter_KvPair{
					Key:   "Name",
					Value: "openshift-master",
				},
			},
		}},
	}
	result, err := ListEc2InstancesForCredentials(tmpUpstream, nil)
	if err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("failure while running", zap.Error(err))
	}
	fmt.Println(result)
}

func getLocalAwsSession(region string) (*session.Session, error) {
	return session.NewSession(&aws.Config{
		Region: &region,
	})
}

/*
Steps:
- register a tag name that will be used to indicate EC2 instances that gloo should turn into upstreams
  - this can be passed as a plugin setting
  - for the purpose of discussion, let's say we're using "gloostream"
- find all instances that match the tag with DescribeInstances
  - can use the tagFiltersKey helper to do this
- create an upstream for each unique tag value
- create an endpoint for each instance

Interfaces to implement:
- plugins.Plugin
- plugins.UpstreamPlugin
- discovery.DiscoveryPlugin

Upstream type to create:
- UpstreamSpec_AwsEc2
  - Note: we already have UpstreamSpec_Aws for lambdas. We had not been thinking about EC2 at the time
*/

func ListEc2InstancesForCredentials(ec2Upstream *glooec2.UpstreamSpec, secrets v1.SecretList) ([]*ec2.Instance, error) {
	sess, err := aws2.GetAwsSession(ec2Upstream.SecretRef, secrets, &aws.Config{Region: aws.String(ec2Upstream.Region)})
	if err != nil {
		return nil, err
	}
	svc := ec2.New(sess)
	input := &ec2.DescribeInstancesInput{
		Filters: convertFiltersFromSpec(ec2Upstream),
	}
	result, err := svc.DescribeInstances(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
	}

	return getInstancesFromDescription(result), nil
}

func getInstancesFromDescription(desc *ec2.DescribeInstancesOutput) []*ec2.Instance {
	var instances []*ec2.Instance
	for _, reservation := range desc.Reservations {
		for _, instance := range reservation.Instances {
			instances = append(instances, instance)
		}
	}
	return instances
}

func convertFiltersFromSpec(upstreamSpec *glooec2.UpstreamSpec) []*ec2.Filter {
	var filters []*ec2.Filter
	for _, filterSpec := range upstreamSpec.Filters {
		var currentFilter *ec2.Filter
		switch x := filterSpec.Spec.(type) {
		case *glooec2.Filter_Key:
			currentFilter = tagFiltersKeyValue(x.Key, "")
		case *glooec2.Filter_KvPair_:
			currentFilter = tagFiltersKeyValue(x.KvPair.Key, x.KvPair.Value)
		}
		filters = append(filters, currentFilter)
	}
	return filters
}

// EC2 Describe Instance filters expect a particular key format:
//   https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeInstances.html
//   tag:<key> - The key/value combination of a tag assigned to the resource. Use the tag key in the filter name and the
//   tag value as the filter value. For example, to find all resources that have a tag with the key Owner and the value
//   TeamA, specify tag:Owner for the filter name and TeamA for the filter value.
func tagFilterName(tagName string) *string {
	str := fmt.Sprintf("tag:%v", tagName)
	return &str
}

func tagFilterValue(tagValue string) []*string {
	if tagValue == "" {
		return nil
	}
	return []*string{&tagValue}
}

// Helper for getting a filter that selects all instances that have a given tag and tag-value pair
func tagFiltersKeyValue(tagName, tagValue string) *ec2.Filter {
	return &ec2.Filter{
		Name:   tagFilterName(tagName),
		Values: tagFilterValue(tagValue),
	}
}

// Helper for getting a filter that selects all instances that have a given tag
// How to find all instances that have a given tag-key, regardless of the tag value:
//   https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeInstances.html
//   tag-key - The key of a tag assigned to the resource. Use this filter to find all resources that have a tag with a
//   specific key, regardless of the tag value.
func tagFiltersKey(tagName string) []*ec2.Filter {
	tagKey := "tag-key"
	return []*ec2.Filter{
		{
			Name:   &tagKey,
			Values: []*string{&tagName},
		},
	}
}

type plugin struct {
	// glooTag is the tag-key that indicates an EC2 instance should be turned into a Endpoint and that the tag's value
	// should correspond to an Upstream
	glooTag      string
	secretClient v1.SecretClient

	// pre-initialization only
	secretFactory factory.ResourceClientFactory
}

func NewPlugin(secretFactory factory.ResourceClientFactory) *plugin {
	return &plugin{secretFactory: secretFactory}
}

var _ plugins.Plugin = new(plugin)
var _ plugins.UpstreamPlugin = new(plugin)
var _ discovery.DiscoveryPlugin = new(plugin)

const Ec2DiscoveryGlooTagKey = "gloo-tag-key"

func (p *plugin) Init(params plugins.InitParams) error {
	// TODO(set tag from config)
	//p.glooTag = params.ExtensionsSettings.Configs[Ec2DiscoveryGlooTagKey]
	var err error
	p.secretClient, err = v1.NewSecretClient(p.secretFactory)
	if err != nil {
		return err
	}
	if err := p.secretClient.Register(); err != nil {
		return err
	}
	return nil
}
func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoyapi.Cluster) error {
	// not ours
	_, ok := in.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_AwsEc2)
	if !ok {
		return nil
	}

	// configure the cluster to use EDS:ADS and call it a day
	xds.SetEdsOnCluster(out)
	return nil
}

const DiscoveryLoggerName = "ec2-uds"

func (p *plugin) DiscoverUpstreams(watchNamespaces []string, writeNamespace string, opts clients.WatchOpts, discOpts discovery.Opts) (chan v1.UpstreamList, chan error, error) {
	return nil, nil, nil
}

// we do not need to update any fields, just check that the input is valid
func (p *plugin) UpdateUpstream(original, desired *v1.Upstream) (bool, error) {
	originalSpec, ok := original.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_AwsEc2)
	if !ok {
		return false, errors.Errorf("internal error: expected *v1.UpstreamSpec_AwsEc2, got %v", reflect.TypeOf(original.UpstreamSpec.UpstreamType).Name())
	}
	desiredSpec, ok := desired.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_AwsEc2)
	if !ok {
		return false, errors.Errorf("internal error: expected *v1.UpstreamSpec_AwsEc2, got %v", reflect.TypeOf(original.UpstreamSpec.UpstreamType).Name())
	}
	if !originalSpec.Equal(desiredSpec) {
		return false, errors.New("expected no difference between *v1.UpstreamSpec_AwsEc2 upstreams")
	}
	return false, nil
}

// EDS API
// start the EDS watch which sends a new list of endpoints on any change
func (p *plugin) WatchEndpoints(writeNamespace string, upstreams v1.UpstreamList, opts clients.WatchOpts) (<-chan v1.EndpointList, <-chan error, error) {
	// TODO
	return newEndpointsWatcher(opts.Ctx, writeNamespace, upstreams, p.secretClient, opts.RefreshRate).poll()
}

type edsWatcher struct {
	upstreams      map[core.ResourceRef]*glooec2.UpstreamSpec
	watchContext   context.Context
	secretClient   v1.SecretClient
	refreshRate    time.Duration
	writeNamespace string
}

func newEndpointsWatcher(watchCtx context.Context, writeNamespace string, upstreams v1.UpstreamList, secretClient v1.SecretClient, parentRefreshRate time.Duration) *edsWatcher {
	upstreamSpecs := make(map[core.ResourceRef]*glooec2.UpstreamSpec)
	for _, us := range upstreams {
		ec2Upstream, ok := us.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_AwsEc2)
		// only care about kube upstreams
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
		secrets, err := c.secretClient.List(tmpTODOAllNamespaces, clients.ListOpts{})
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
	ec2InstancesForUpstream, err := ListEc2InstancesForCredentials(ec2Upstream, secrets)
	if err != nil {
		return nil, err
	}
	return c.convertInstancesToEndpoints(upstreamRef, ec2InstancesForUpstream), nil
}

func (c *edsWatcher) convertInstancesToEndpoints(upstreamRef *core.ResourceRef, ec2InstancesForUpstream []*ec2.Instance) v1.EndpointList {
	// TODO - get port from upstream, instance tag, or elsewhere
	var tmpTODOPort uint32 = 8080
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
	return kubeutils.SanitizeName(fmt.Sprintf("%v-%v-%v", ec2EndpointNamePrefix, upstreamRef.String()+publicIpAddress))
}

/*
What about uniqueness?
- can multiple upstreams point to the same ec2 targets?

Reporting activity?
- can gloo write to the upstream to say which ec2 instances it points to?
  - if so, how should those be identified? Would it be considered a leak to show the id of an instance that is not visible to some of the people who can view the upstream?

Need to watch secrets to make sure that it has the latest credentials
[ ] how to do this in a platform-agnostic way?
  - though we have generated clients, how to we access something like secrets without first knowing which client to choose?

*/
