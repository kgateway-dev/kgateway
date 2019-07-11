package awscache

import (
	"context"
	"sync"
	"time"

	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws/ec2/awslister"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/aws/aws-sdk-go/service/ec2"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws/glooec2"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// a credential batch stores the credentialMap available to a given credential spec
// it is possible that there will be duplicate resource records, for example, if two credentials have access to the same
// resource, then that resource will be present in both CredentialInstanceGroup entries. For simplicity, we will let that be.
type Cache struct {
	credentialMap map[credentialSpec]*CredentialInstanceGroup
	secrets       v1.SecretList
	mutex         sync.Mutex
	ctx           context.Context
}

func NewCredentialInstanceGroup() *CredentialInstanceGroup {
	return &CredentialInstanceGroup{
		upstreams: make(map[core.ResourceRef]*glooec2.UpstreamSpecRef),
	}
}

// CredentialInstanceGroup represents the instances available to a given credentialSpec
type CredentialInstanceGroup struct {
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

func New(ctx context.Context, secrets v1.SecretList, upstreams map[core.ResourceRef]*glooec2.UpstreamSpecRef, ec2InstanceLister awslister.Ec2InstanceLister) (*Cache, error) {
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
	m.credentialMap = make(map[credentialSpec]*CredentialInstanceGroup)
	return m
}

func (c *Cache) build(upstreams map[core.ResourceRef]*glooec2.UpstreamSpecRef, ec2InstanceLister awslister.Ec2InstanceLister) error {
	for _, upstream := range upstreams {
		if err := c.addUpstream(upstream); err != nil {
			return err
		}
	}
	contextutils.LoggerFrom(c.ctx).Debugw("local store", zap.Any("count", len(c.credentialMap)))
	// 2. query the AWS API for each credential set
	errChan := make(chan error)
	defer close(errChan)
	eg := errgroup.Group{}
	go func() {
		// first copy from map to a slice in order to avoid a race condition
		var creds []credentialSpec
		for cred := range c.credentialMap {
			creds = append(creds, cred)
		}
		for _, cred := range creds {
			eg.Go(func() error {
				instances, err := ec2InstanceLister.ListForCredentials(c.ctx, cred.region, cred.secretRef, c.secrets)
				if err != nil {
					return err
				}
				if err := c.addInstances(cred, instances); err != nil {
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

func (c *Cache) addUpstream(upstream *glooec2.UpstreamSpecRef) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	key := credentialSpecFromUpstreamSpec(upstream.Spec)

	if v, ok := c.credentialMap[key]; ok {
		v.upstreams[upstream.Ref] = upstream
	} else {
		cr := NewCredentialInstanceGroup()
		cr.upstreams[upstream.Ref] = upstream
		c.credentialMap[key] = cr
	}
	return nil
}

func (c *Cache) addInstances(credentialSpec credentialSpec, instances []*ec2.Instance) error {
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
