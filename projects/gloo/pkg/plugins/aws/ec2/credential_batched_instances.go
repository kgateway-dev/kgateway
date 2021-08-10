package ec2

import (
	"context"
	"strings"

	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	glooec2 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws/ec2"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const InstanceIdAnnotationKey = "instanceId"

// In order to minimize calls to the AWS API, we group calls by credentials and apply tag filters locally.
// This function groups upstreams by credentials, calls the AWS API, maps the instances to upstreams, and returns the
// endpoints associated with the provided upstream list
// NOTE: MUST filter the upstreamList to ONLY EC2 upstreams before calling this function
func getLatestEndpoints(ctx context.Context, lister Ec2InstanceLister, secrets v1.SecretList, writeNamespace string, upstreamList v1.UpstreamList) (v1.EndpointList, error) {
	// we want unique creds so we can query api once per unique cred
	// we need to make sure we maintain the association between those unique creds and the upstreams that share them
	// so that when we get the instances associated with the creds, we will know which upstreams have access to those
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
			instancesForUpstream := filterInstancesForUpstream(ctx, upstream, credGroup)
			for _, instance := range instancesForUpstream {
				if endpoint := upstreamInstanceToEndpoint(ctx, writeNamespace, upstream, instance); endpoint != nil {
					allEndpoints = append(allEndpoints, endpoint)
				}
			}
		}
	}
	return allEndpoints, nil
}

// credentialGroup exists to support batched calls to the AWS API
// one credentialGroup should be made for each unique credentialSpec
type credentialGroup struct {
	// a unique credential spec
	credentialSpec *CredentialSpec
	// all the upstreams that share the CredentialSpec
	upstreams v1.UpstreamList
	// all the instances visible to the given credentials
	instances []*ec2.Instance
	// one filter map exists for each instance in order to support client-side filtering
	filterMaps []FilterMap
}

// Initializes the credentialGroups
// Credential groups are returned as a map to enforce the "one credentialGroup per unique credential" property that is
// required in order to realize the benefits of batched AWS API calls.
// NOTE: assumes that upstreams are EC2 upstreams
func getCredGroupsFromUpstreams(upstreams v1.UpstreamList) (map[CredentialKey]*credentialGroup, error) {
	credGroups := make(map[CredentialKey]*credentialGroup)
	for _, upstream := range upstreams {
		cred := NewCredentialSpecFromEc2UpstreamSpec(upstream.GetAwsEc2())
		key := cred.GetKey()
		if _, ok := credGroups[key]; ok {
			credGroups[key].upstreams = append(credGroups[key].upstreams, upstream)
		} else {
			credGroups[key] = &credentialGroup{
				upstreams:      v1.UpstreamList{upstream},
				credentialSpec: cred,
			}
		}
	}
	return credGroups, nil
}

// calls the AWS API and attaches the output to the the provided list of credentialGroups. Modifications include:
// - adds the instances for each credentialGroup's credential
// - adds tag filters for each instance for later use when refining the list of instances that an upstream has
// permission to describe to the list of instances that the upstream should route to
func getInstancesForCredentialGroups(ctx context.Context, lister Ec2InstanceLister, secrets v1.SecretList, credGroups map[CredentialKey]*credentialGroup) error {
	for _, credGroup := range credGroups {
		instances, err := lister.ListForCredentials(ctx, credGroup.credentialSpec, secrets)
		if err != nil {
			return err
		}
		credGroup.instances = instances
		credGroup.filterMaps = generateFilterMaps(instances)
	}
	return nil
}

// applies filter logic equivalent to the tag filter logic used in AWS's DescribeInstances API
// NOTE: assumes that upstreams are EC2 upstreams
func filterInstancesForUpstream(ctx context.Context, upstream *v1.Upstream, credGroup *credentialGroup) []*ec2.Instance {
	var instances []*ec2.Instance
	logger := contextutils.LoggerFrom(ctx)
	// sweep through each filter map, if all the upstream's filters are matched, add the corresponding instance to the list
	for i, fm := range credGroup.filterMaps {
		candidateInstance := credGroup.instances[i]
		logger.Debugw("considering instance for upstream", "upstream", upstream.GetMetadata().Ref().Key(), "instance-tags", candidateInstance.Tags, "instance-id", candidateInstance.InstanceId)
		matchesAll := true
	ScanFilters: // label so that we can break out of the for loop rather than the switch
		for _, filter := range upstream.GetAwsEc2().GetFilters() {
			switch filterSpec := filter.GetSpec().(type) {
			case *glooec2.TagFilter_Key:
				if _, ok := fm[awsKeyCase(filterSpec.Key)]; !ok {
					matchesAll = false
					break ScanFilters
				}
			case *glooec2.TagFilter_KvPair_:
				if val, ok := fm[awsKeyCase(filterSpec.KvPair.GetKey())]; !ok || val != filterSpec.KvPair.GetValue() {
					matchesAll = false
					break ScanFilters
				}
			}
		}
		if matchesAll {
			instances = append(instances, candidateInstance)
			logger.Debugw("instance for upstream accepted", "upstream", upstream.GetMetadata().Ref().Key(), "instance-tags", candidateInstance.Tags, "instance-id", candidateInstance.InstanceId)
		} else {
			logger.Debugw("instance for upstream filtered out", "upstream", upstream.GetMetadata().Ref().Key(), "instance-tags", candidateInstance.Tags, "instance-id", candidateInstance.InstanceId)
		}
	}
	return instances
}

// NOTE: assumes that upstreams are EC2 upstreams
func upstreamInstanceToEndpoint(ctx context.Context, writeNamespace string, upstream *v1.Upstream, instance *ec2.Instance) *v1.Endpoint {
	ipAddr := instance.PrivateIpAddress
	if upstream.GetAwsEc2().GetPublicIp() {
		ipAddr = instance.PublicIpAddress
	}
	if ipAddr == nil {
		contextutils.LoggerFrom(ctx).Warnw("no ip found for config",
			zap.Any("upstreamRef", upstream.GetMetadata().Ref()),
			zap.Any("instanceId", aws.StringValue(instance.InstanceId)),
			zap.Any("upstream.usePublicIp", upstream.GetAwsEc2().GetPublicIp()))
		return nil
	}
	port := upstream.GetAwsEc2().GetPort()
	if port == 0 {
		port = DefaultPort
	}
	ref := upstream.GetMetadata().Ref()
	// for easier debugging, add the instance id to the xds output
	instanceInfo := make(map[string]string)
	instanceInfo[InstanceIdAnnotationKey] = aws.StringValue(instance.InstanceId)
	endpoint := v1.Endpoint{
		Upstreams: []*core.ResourceRef{ref},
		Address:   aws.StringValue(ipAddr),
		Port:      port,
		Metadata: &core.Metadata{
			Name:        generateName(ref, aws.StringValue(ipAddr)),
			Namespace:   writeNamespace,
			Annotations: instanceInfo,
		},
	}
	contextutils.LoggerFrom(ctx).Debugw("instance from upstream",
		zap.Any("upstream", upstream),
		zap.Any("instance", instance),
		zap.Any("endpoint", endpoint))
	return &endpoint
}

// a FilterMap is created for each EC2 instance so we can efficiently filter the instances associated with a given
// upstream's filter spec
// filter maps are generated from tag lists, the keys are the tag keys, the values are the tag values
type FilterMap map[string]string

func generateFilterMap(instance *ec2.Instance) FilterMap {
	m := make(FilterMap)
	for _, t := range instance.Tags {
		m[awsKeyCase(aws.StringValue(t.Key))] = aws.StringValue(t.Value)
	}
	return m
}

func generateFilterMaps(instances []*ec2.Instance) []FilterMap {
	var maps []FilterMap
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
