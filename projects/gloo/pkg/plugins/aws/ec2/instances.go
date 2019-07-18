package ec2

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws/glooec2"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws/ec2/awscache"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws/ec2/awslister"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// TODO(cleanup): move retained content of awscache pkg into this pkg
// TODO(cleanup): move retained content of awslister pkg into this pkg (MAYBE - keep separate, tbd)

// one credentialGroup should be made for each unique credentialSpec
type credentialGroup struct {
	// a unique credential spec
	credentialSpec *awslister.CredentialSpec
	// all the upstreams that share the CredentialSpec
	upstreams v1.UpstreamList
	// all the instances visible to the given credentials
	instances []*ec2.Instance
	// one filter map exists for each instance in order to support client-side filtering
	filterMaps []awscache.FilterMap
}

// assumes that upstream are EC2 upstreams
// initializes the credentialGroups
func getCredGroupsFromUpstreams(upstreams v1.UpstreamList) ([]*credentialGroup, error) {
	uniqueCredentialsMap := make(map[awslister.CredentialKey]*credentialGroup)
	for _, upstream := range upstreams {
		cred := getCredForUpstream(upstream)
		key := cred.GetKey()
		if _, ok := uniqueCredentialsMap[key]; ok {
			uniqueCredentialsMap[key].upstreams = append(uniqueCredentialsMap[key].upstreams, upstream)
		} else {
			uniqueCredentialsMap[key] = &credentialGroup{
				upstreams:      v1.UpstreamList{upstream},
				credentialSpec: cred,
			}
		}
	}
	var credGroups []*credentialGroup
	for _, cred := range uniqueCredentialsMap {
		credGroups = append(credGroups, cred)
	}
	return credGroups, nil
}

func getCredForUpstream(upstream *v1.Upstream) *awslister.CredentialSpec {
	return awslister.NewCredentialSpecFromEc2UpstreamSpec(upstream.UpstreamSpec.GetAwsEc2())
}

//define a function to get all instances from a list of unique credentials
func getInstancesForCredentialGroups(ctx context.Context, lister awslister.Ec2InstanceLister, secrets v1.SecretList, credGroups []*credentialGroup) error {
	for _, credGroup := range credGroups {
		instances, err := lister.ListForCredentials(ctx, credGroup.credentialSpec, secrets)
		if err != nil {
			return err
		}
		credGroup.instances = instances
		credGroup.filterMaps = awscache.GenerateFilterMaps(instances)
	}
	return nil
}

func filterInstancesForUpstream(upstream *v1.Upstream, credGroup *credentialGroup) []*ec2.Instance {
	var instances []*ec2.Instance
	// sweep through each filter map, if all the upstream's filters are matched, add the corresponding instance to the list
	for i, fm := range credGroup.filterMaps {
		candidateInstance := credGroup.instances[i]
		matchesAll := true
	ScanFilters: // label so that we can break out of the for loop rather than the switch
		for _, filter := range upstream.UpstreamSpec.GetAwsEc2().Filters {
			switch filterSpec := filter.Spec.(type) {
			case *glooec2.TagFilter_Key:
				if _, ok := fm[awscache.AwsKeyCase(filterSpec.Key)]; !ok {
					matchesAll = false
					break ScanFilters
				}
			case *glooec2.TagFilter_KvPair_:
				if val, ok := fm[awscache.AwsKeyCase(filterSpec.KvPair.Key)]; !ok || val != filterSpec.KvPair.Value {
					matchesAll = false
					break ScanFilters
				}
			}
		}
		if matchesAll {
			instances = append(instances, candidateInstance)
		}
	}
	return instances
}

func upstreamInstanceToEndpoint(writeNamespace string, upstream *v1.Upstream, instance *ec2.Instance) *v1.Endpoint {
	ipAddr := instance.PrivateIpAddress
	if upstream.UpstreamSpec.GetAwsEc2().PublicIp {
		ipAddr = instance.PublicIpAddress
	}
	port := upstream.UpstreamSpec.GetAwsEc2().GetPort()
	if port == 0 {
		port = defaultPort
	}
	ref := upstream.Metadata.Ref()
	return &v1.Endpoint{
		Upstreams: []*core.ResourceRef{&ref},
		Address:   aws.StringValue(ipAddr),
		Port:      upstream.UpstreamSpec.GetAwsEc2().GetPort(),
		Metadata: core.Metadata{
			Name:      generateName(ref, aws.StringValue(ipAddr)),
			Namespace: writeNamespace,
		},
	}
}

// MUST filter the upstreamList to ONLY EC2 upstreams before calling this function
func getLatestEndpoints(ctx context.Context, lister awslister.Ec2InstanceLister, secrets v1.SecretList, writeNamespace string, upstreamList v1.UpstreamList) (v1.EndpointList, error) {
	// we want unique creds so we can query api once per unique cred
	// we need to make sure we maintain the association between those unique creds and the upstreams that share them
	// so that when we get the instances associated with the creds we will know which upstreams have access to those
	// instances.
	credGroups, err := getCredGroupsFromUpstreams(upstreamList)
	if err != nil {
		return nil, err
	}
	// call the EC2 DescribeInstances once for each set of credentials and apply the output to the credential groups
	if err := getInstancesForCredentialGroups(ctx, lister, secrets, credGroups); err != nil {
		return nil, err
	}
	// produce the endpoints list
	var allEndpoints v1.EndpointList
	for _, credGroup := range credGroups {
		for _, upstream := range credGroup.upstreams {
			instancesForUpstream := filterInstancesForUpstream(upstream, credGroup)
			for _, instance := range instancesForUpstream {
				allEndpoints = append(allEndpoints, upstreamInstanceToEndpoint(writeNamespace, upstream, instance))
			}
		}
	}
	return allEndpoints, nil
}
