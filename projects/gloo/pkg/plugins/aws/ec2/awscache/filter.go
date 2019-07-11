package awscache

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws/glooec2"
)

func (c *localStore) filterEndpointsForUpstream(upstream *glooec2.UpstreamSpecRef) ([]*ec2.Instance, error) {
	credSpec := credentialSpecFromUpstreamSpec(upstream.Spec)
	credRes, ok := c.credentialMap[credSpec]
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
		for _, filter := range upstream.Spec.Filters {
			switch filterSpec := filter.Spec.(type) {
			case *glooec2.TagFilter_Key:
				if _, ok := fm[awsKeyCase(filterSpec.Key)]; !ok {
					matchesAll = false
					break ScanFilters
				}
			case *glooec2.TagFilter_KvPair_:
				if val, ok := fm[awsKeyCase(filterSpec.KvPair.Key)]; !ok || val != filterSpec.KvPair.Value {
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

func generateFilterMap(instance *ec2.Instance) filterMap {
	m := make(filterMap)
	for _, t := range instance.Tags {
		m[awsKeyCase(aws.StringValue(t.Key))] = aws.StringValue(t.Value)
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

// AWS tag keys are not case-sensitive so cast them all to lowercase
// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/iam-policy-structure.html#amazon-ec2-keys
func awsKeyCase(input string) string {
	return strings.ToLower(input)
}
