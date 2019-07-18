package awscache

import (
	"strings"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws/ec2/awslister"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws/glooec2"
)

func (c *Cache) FilterEndpointsForUpstream(upstreamSpec *glooec2.UpstreamSpec) ([]*ec2.Instance, error) {
	credSpec := awslister.NewCredentialSpecFromEc2UpstreamSpec(upstreamSpec)
	credRes, ok := c.instanceGroups[credSpec.GetKey()]
	if !ok {
		// This should never happen
		return nil, ResourceMapInitializationError
	}
	var list []*ec2.Instance
	// sweep through each filter map, if all the upstream's filters are matched, add the corresponding instance to the list
	for i, fm := range credRes.instanceFilterMaps {
		candidateInstance := credRes.instances[i]
		matchesAll := true
	ScanFilters: // label so that we can break out of the for loop rather than the switch
		for _, filter := range upstreamSpec.Filters {
			switch filterSpec := filter.Spec.(type) {
			case *glooec2.TagFilter_Key:
				if _, ok := fm[AwsKeyCase(filterSpec.Key)]; !ok {
					matchesAll = false
					break ScanFilters
				}
			case *glooec2.TagFilter_KvPair_:
				if val, ok := fm[AwsKeyCase(filterSpec.KvPair.Key)]; !ok || val != filterSpec.KvPair.Value {
					matchesAll = false
					break ScanFilters
				}
			}
		}
		if matchesAll {
			list = append(list, candidateInstance)
		}
	}
	return list, nil
}

func generateFilterMap(instance *ec2.Instance) FilterMap {
	m := make(FilterMap)
	for _, t := range instance.Tags {
		m[AwsKeyCase(aws.StringValue(t.Key))] = aws.StringValue(t.Value)
	}
	return m
}

func GenerateFilterMaps(instances []*ec2.Instance) []FilterMap {
	var maps []FilterMap
	for _, instance := range instances {
		maps = append(maps, generateFilterMap(instance))
	}
	return maps
}

// AWS tag keys are not case-sensitive so cast them all to lowercase
// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/iam-policy-structure.html#amazon-ec2-keys
func AwsKeyCase(input string) string {
	return strings.ToLower(input)
}
