package ec2

import (
	"context"
	"fmt"

	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"

	"github.com/aws/aws-sdk-go/aws/session"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws/glooec2"
	aws2 "github.com/solo-io/gloo/projects/gloo/pkg/utils/aws"
)

func GetEc2Session(ec2Upstream *glooec2.UpstreamSpec, secrets v1.SecretList) (*session.Session, error) {
	return aws2.GetAwsSession(ec2Upstream.SecretRef, secrets, &aws.Config{Region: aws.String(ec2Upstream.Region)})
}
func ListEc2InstancesForCredentials(ctx context.Context, sess *session.Session, ec2Upstream *glooec2.UpstreamSpec) ([]*ec2.Instance, error) {
	svc := ec2.New(sess)
	contextutils.LoggerFrom(ctx).Debugw("ec2Upstream", zap.Any("spec", ec2Upstream))
	input := &ec2.DescribeInstancesInput{
		Filters: convertFiltersFromSpec(ec2Upstream),
	}
	contextutils.LoggerFrom(ctx).Debugw("ec2Upstream input", zap.Any("value", input))
	result, err := svc.DescribeInstances(input)
	contextutils.LoggerFrom(ctx).Debugw("ec2Upstream result", zap.Any("value", result))
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			// TODO - handle specific aws error codes
			default:
				contextutils.LoggerFrom(ctx).Errorw("unable to describe instances, aws error", zap.Error(aerr))
			}
		} else {
			contextutils.LoggerFrom(ctx).Errorw("unable to describe instances, other error", zap.Error(err))
		}
	}
	return getInstancesFromDescription(result), nil
}

func getInstancesFromDescription(desc *ec2.DescribeInstancesOutput) []*ec2.Instance {
	var instances []*ec2.Instance
	for _, reservation := range desc.Reservations {
		for _, instance := range reservation.Instances {
			if validInstance(instance) {
				instances = append(instances, instance)
			}
		}
	}
	return instances
}

// this filter function defines what gloo considers a valid EC2 instance
func validInstance(instance *ec2.Instance) bool {
	if instance.PublicIpAddress == nil {
		return false
	}
	return true
}

func convertFiltersFromSpec(upstreamSpec *glooec2.UpstreamSpec) []*ec2.Filter {
	var filters []*ec2.Filter
	for _, filterSpec := range upstreamSpec.Filters {
		var currentFilter *ec2.Filter
		switch x := filterSpec.Spec.(type) {
		case *glooec2.Filter_Key:
			currentFilter = tagFiltersKey(x.Key)
		case *glooec2.Filter_KvPair_:
			currentFilter = tagFiltersKeyValue(x.KvPair.Key, x.KvPair.Value)
		}
		filters = append(filters, currentFilter)
	}
	fmt.Printf("filters are:\n%v\n", filters)
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

/*
NOTE on EC2
How to find all instances that have a given tag-key, regardless of the tag value:
  https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeInstances.html
  tag-key - The key of a tag assigned to the resource. Use this filter to find all resources that have a tag with a
  specific key, regardless of the tag value.
*/
// generate a filter that selects all elements that contain a given tag
func tagFiltersKey(tagName string) *ec2.Filter {
	return &ec2.Filter{
		Name:   aws.String("tag-key"),
		Values: []*string{aws.String(tagName)},
	}
}
