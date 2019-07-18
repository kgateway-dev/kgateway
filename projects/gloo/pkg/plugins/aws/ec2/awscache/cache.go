package awscache

import (
	"context"
	"sync"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws/glooec2/utils"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws/ec2/awslister"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/aws/aws-sdk-go/service/ec2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

// a credential batch stores the credentialMap available to a given credential spec
// it is possible that there will be duplicate resource records, for example, if two credentials have access to the same
// resource, then that resource will be present in both CredentialInstanceGroup entries. For simplicity, we will let that be.
type Cache struct {
	instanceGroups  map[awslister.CredentialKey]*CredentialInstanceGroup
	credentialSpecs map[awslister.CredentialKey]*awslister.CredentialSpec
	secrets         v1.SecretList
	mutex           sync.Mutex
	ctx             context.Context
}

func NewCredentialInstanceGroup() *CredentialInstanceGroup {
	return &CredentialInstanceGroup{
		upstreams: make(utils.InvertedEc2UpstreamRefMap),
	}
}

// CredentialInstanceGroup represents the instances available to a given credentialSpec
type CredentialInstanceGroup struct {
	// all upstreams having the same credential spec (secret and aws region) will be listed here
	upstreams utils.InvertedEc2UpstreamRefMap

	// instances contains all of the EC2 instances available for the given credential spec
	instances []*ec2.Instance
	// instanceFilterMaps contains one filter map for each instance
	// indices correspond: instanceFilterMap[i] == FilterMap(instance[i])
	// we store the filter map so that it can be reused across upstreams when determining if a given instance should be
	// associated with a given upstream
	instanceFilterMaps []FilterMap
}

// a FilterMap is created for each EC2 instance so we can efficiently filter the instances associated with a given
// upstream's filter spec
// filter maps are generated from tag lists, the keys are the tag keys, the values are the tag values
type FilterMap map[string]string

func New(ctx context.Context, secrets v1.SecretList, upstreams utils.InvertedEc2UpstreamRefMap, ec2InstanceLister awslister.Ec2InstanceLister) (*Cache, error) {
	m := newCache(ctx, secrets)
	if err := m.build(upstreams, ec2InstanceLister); err != nil {
		return nil, err
	}
	return m, nil
}

// break out this function for easier testing
func newCache(ctx context.Context, secrets v1.SecretList) *Cache {
	m := &Cache{
		ctx:     ctx,
		secrets: secrets,
	}
	m.instanceGroups = make(map[awslister.CredentialKey]*CredentialInstanceGroup)
	m.credentialSpecs = make(map[awslister.CredentialKey]*awslister.CredentialSpec)
	return m
}

func (c *Cache) build(upstreams utils.InvertedEc2UpstreamRefMap, ec2InstanceLister awslister.Ec2InstanceLister) error {
	for _, upstream := range upstreams {
		if err := c.addUpstream(upstream); err != nil {
			return err
		}
	}
	contextutils.LoggerFrom(c.ctx).Debugw("local store",
		zap.Any("instance group count", len(c.instanceGroups)))
	// 2. query the AWS API for each credential set
	errChan := make(chan error)
	defer close(errChan)
	eg := errgroup.Group{}
	go func() {
		// first copy from map to a slice in order to avoid a race condition
		var creds []*awslister.CredentialSpec
		for _, cred := range c.credentialSpecs {
			creds = append(creds, cred)
		}
		for _, cred := range creds {
			// make a copy to avoid race condition
			credCopy := cred.Clone()
			eg.Go(func() error {
				instances, err := ec2InstanceLister.ListForCredentials(c.ctx, credCopy, c.secrets)
				if err != nil {
					return err
				}
				if err := c.addInstances(credCopy, instances); err != nil {
					return err
				}
				return nil
			})
		}
		errChan <- eg.Wait()
	}()
	select {
	case err := <-errChan:
		if err != nil {
			return ListCredentialError(err)
		}
		return nil
	case <-time.After(awsCallTimeout):
		return TimeoutError
	}
}

func (c *Cache) addUpstream(upstream *utils.InvertedEc2Upstream) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	credSpec := awslister.NewCredentialSpecFromEc2UpstreamSpec(upstream.AwsEc2Spec)
	key := credSpec.GetKey()

	if v, ok := c.instanceGroups[key]; ok {
		v.upstreams[upstream.Base.Metadata.Ref()] = upstream
	} else {
		cr := NewCredentialInstanceGroup()
		cr.upstreams[upstream.Base.Metadata.Ref()] = upstream
		c.instanceGroups[key] = cr
		c.credentialSpecs[key] = credSpec
	}
	return nil
}

func (c *Cache) addInstances(credentialSpec *awslister.CredentialSpec, instances []*ec2.Instance) error {
	filterMaps := GenerateFilterMaps(instances)
	key := credentialSpec.GetKey()
	c.mutex.Lock()
	defer c.mutex.Unlock()
	ci := c.instanceGroups[key]
	if ci == nil {
		// should not happen
		return ResourceMapInitializationError
	}
	ci.instances = instances
	ci.instanceFilterMaps = filterMaps
	return nil
}
