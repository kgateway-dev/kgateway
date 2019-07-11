package awscache

import (
	"context"
	"sync"

	"github.com/aws/aws-sdk-go/service/ec2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws/glooec2"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// a credential batch stores the credentialMap available to a given credential spec
// it is possible that there will be duplicate resource records, for example, if two credentials have access to the same
// resource, then that resource will be present in both credentialInstanceGroup entries. For simplicity, we will let that be.
type localStore struct {
	credentialMap map[credentialSpec]*credentialInstanceGroup
	secrets       v1.SecretList
	mutex         sync.Mutex
	ctx           context.Context
}

func newCredentialInstanceGroup() *credentialInstanceGroup {
	return &credentialInstanceGroup{
		upstreams: make(map[core.ResourceRef]*glooec2.UpstreamSpecRef),
	}
}

// credentialInstanceGroup represents the instances available to a given credentialSpec
type credentialInstanceGroup struct {
	// all upstreams having the same credential spec (secret and aws region) will be listed here
	upstreams map[core.ResourceRef]*glooec2.UpstreamSpecRef

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

func newLocalStore(ctx context.Context, secrets v1.SecretList) *localStore {
	m := &localStore{
		ctx:     ctx,
		secrets: secrets,
	}
	m.credentialMap = make(map[credentialSpec]*credentialInstanceGroup)
	return m
}

func (c *localStore) addUpstream(upstream *glooec2.UpstreamSpecRef) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	key := credentialSpecFromUpstreamSpec(upstream.Spec)

	if v, ok := c.credentialMap[key]; ok {
		v.upstreams[upstream.Ref] = upstream
	} else {
		cr := newCredentialInstanceGroup()
		cr.upstreams[upstream.Ref] = upstream
		c.credentialMap[key] = cr
	}
	return nil
}

func (c *localStore) addInstances(credentialSpec credentialSpec, instances []*ec2.Instance) error {
	filterMaps := generateFilterMaps(instances)
	c.mutex.Lock()
	cr := c.credentialMap[credentialSpec]
	if cr == nil {
		// should not happen
		return ResourceMapInitializationError
	}
	cr.instances = instances
	cr.instanceFilterMaps = filterMaps
	c.mutex.Unlock()
	return nil
}
