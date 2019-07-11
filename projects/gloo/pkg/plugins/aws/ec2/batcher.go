package ec2

import (
	"context"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws/glooec2"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

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

func (c *credentialBatch) addInstances(credentialSpec credentialSpec, instances []*ec2.Instance) {
	filterMaps := generateFilterMaps(instances)
	c.mutex.Lock()
	cr := c.resources[credentialSpec]
	if cr == nil {
		// should not happen
		contextutils.LoggerFrom(context.TODO()).Errorw("credential resource map not initialized correctly", zap.Any("credSpec", credentialSpec))
		return
	}
	cr.instances = instances
	cr.instanceFilterMaps = filterMaps
	c.mutex.Unlock()
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
		matchesAll := true
	ScanFilters: // label so that we can break out of the for loop rather than the switch
		for _, filter := range ec2Upstream.Filters {
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
	return list
}

// AWS tag keys are not case-sensitive so cast them all to lowercase
// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/iam-policy-structure.html#amazon-ec2-keys
func awsKeyCase(input string) string {
	return strings.ToLower(input)
}
