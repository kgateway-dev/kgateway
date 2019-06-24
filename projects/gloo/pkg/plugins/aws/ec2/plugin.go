package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
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
	err := run(core.ResourceRef{}, nil)
	if err != nil {
		contextutils.LoggerFrom(ctx).Fatalw("failure while running", zap.Error(err))
	}
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

// TODO
func run(secretRef core.ResourceRef, secrets v1.SecretList) error {
	region := "us-east-1"
	sess, err := aws2.GetAwsSession(secretRef, secrets, &aws.Config{Region: &region})
	if err != nil {
		return err
	}
	svc := ec2.New(sess)
	tag := tagFilterName("Name")
	val := tagFilterValue("openshift-master")
	input := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   tag,
				Values: val,
			},
		},
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

	fmt.Println(result)
	return nil
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
	return []*string{&tagValue}
}

// Helper for getting a filter that selects all instances that have a given tag and tag-value pair
func tagFiltersKeyValue(tagName, tagValue string) []*ec2.Filter {
	return []*ec2.Filter{
		{
			Name:   tagFilterName(tagName),
			Values: tagFilterValue(tagValue),
		},
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
	glooTag string
}

var _ plugins.Plugin = new(plugin)
var _ plugins.UpstreamPlugin = new(plugin)
var _ discovery.DiscoveryPlugin = new(plugin)

const Ec2DiscoveryGlooTagKey = "gloo-tag-key"

func (p *plugin) Init(params plugins.InitParams) error {
	// TODO(set tag from config)
	//p.glooTag = params.ExtensionsSettings.Configs[Ec2DiscoveryGlooTagKey]
	return nil
}
func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoyapi.Cluster) error {

	return nil
}

func (p *plugin) DiscoverUpstreams(watchNamespaces []string, writeNamespace string, opts clients.WatchOpts, discOpts discovery.Opts) (chan v1.UpstreamList, chan error, error) {

}

func (p *plugin) UpdateUpstream(original, desired *v1.Upstream) (bool, error) {
	return false, nil
}

// EDS API
// start the EDS watch which sends a new list of endpoints on any change
// will send only endpoints for upstreams configured with TrackUpstreams
func (p *plugin) WatchEndpoints(writeNamespace string, upstreamsToTrack v1.UpstreamList, opts clients.WatchOpts) (<-chan v1.EndpointList, <-chan error, error) {

}
