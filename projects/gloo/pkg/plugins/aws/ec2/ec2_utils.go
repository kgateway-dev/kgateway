package ec2

import (
	"fmt"

	"github.com/rotisserie/eris"

	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	glooec2 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws/ec2"
	aws2 "github.com/solo-io/gloo/projects/gloo/pkg/utils/aws"
)

func GetEc2Client(cred *CredentialSpec, secrets v1.SecretList) (*ec2.EC2, error) {
	regionConfig := &aws.Config{Region: aws.String(cred.Region())}
	secretRef := cred.SecretRef()
	sess, err := aws2.GetAwsSession(secretRef, secrets, regionConfig)
	if err != nil {
		if secretRef == nil {
			return nil, CreateSessionFromEnvError(err)
		}
		return nil, CreateSessionFromSecretError(err)
	}
	if cred.Arn() != "" {
		cred := stscreds.NewCredentials(sess, cred.Arn())
		config := &aws.Config{Credentials: cred}
		return ec2.New(sess, config), nil
	}
	return ec2.New(sess), nil
}

func GetInstancesFromDescription(desc *ec2.DescribeInstancesOutput) []*ec2.Instance {
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
	if instance.PublicIpAddress != nil {
		return true
	}
	if instance.PrivateIpAddress != nil {
		return true
	}
	return false
}

// generate an ec2 filter spec for a given upstream.
// not currently used since we are batching API calls by credentials, without filters
func convertFiltersFromSpec(upstreamSpec *glooec2.UpstreamSpec) []*ec2.Filter {
	var filters []*ec2.Filter
	for _, filterSpec := range upstreamSpec.GetFilters() {
		var currentFilter *ec2.Filter
		switch x := filterSpec.GetSpec().(type) {
		case *glooec2.TagFilter_Key:
			currentFilter = tagFiltersKey(x.Key)
		case *glooec2.TagFilter_KvPair_:
			currentFilter = tagFiltersKeyValue(x.KvPair.GetKey(), x.KvPair.GetValue())
		}
		filters = append(filters, currentFilter)
	}
	return filters
}

// EC2 Describe Instance filters expect a particular key format:
//
//	https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeInstances.html
//	tag:<key> - The key/value combination of a tag assigned to the resource. Use the tag key in the filter name and the
//	tag value as the filter value. For example, to find all credentialMap that have a tag with the key Owner and the value
//	TeamA, specify tag:Owner for the filter name and TeamA for the filter value.
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
  tag-key - The key of a tag assigned to the resource. Use this filter to find all credentialMap that have a tag with a
  specific key, regardless of the tag value.
*/
// generate a filter that selects all elements that contain a given tag
func tagFiltersKey(tagName string) *ec2.Filter {
	return &ec2.Filter{
		Name:   aws.String("tag-key"),
		Values: []*string{aws.String(tagName)},
	}
}

var (
	CreateSessionFromEnvError = func(err error) error {
		return eris.Wrapf(err, "unable to create a session with credentials taken from env")
	}

	CreateSessionFromSecretError = func(err error) error {
		return eris.Wrapf(err, "unable to create a session with credentials taken from secret ref")
	}
)
