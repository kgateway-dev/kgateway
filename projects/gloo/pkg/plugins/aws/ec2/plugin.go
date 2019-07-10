package ec2

import (
	"reflect"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"

	"github.com/pkg/errors"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
)

/*
Steps:
- User creates an EC2 upstream
  - describes the instances that should be made into Endpoints
- Discovery finds all instances that match the description with DescribeInstances
- Gloo plugin creates an endpoint for each instance
*/

type plugin struct {
	secretClient v1.SecretClient

	// pre-initialization only
	// we need to register the secret while creating the plugin, otherwise our EDS poll will fail (requires a secret client)
	// since Init can be called after our poll begins (race condition) we cannot create the client there
	// since NewPlugin does not return an error, we will store any non-nil errors from initializing the secret client in the plugin struct
	// we will check those errors during the Init call
	constructorErr error
}

// checks to ensure interfaces are implemented
var _ plugins.Plugin = new(plugin)
var _ plugins.UpstreamPlugin = new(plugin)
var _ discovery.DiscoveryPlugin = new(plugin)

func NewPlugin(secretFactory factory.ResourceClientFactory) *plugin {
	p := &plugin{}
	var err error
	p.secretClient, err = v1.NewSecretClient(secretFactory)
	if err != nil {
		p.constructorErr = err
		return p
	}
	if err := p.secretClient.Register(); err != nil {
		p.constructorErr = err
		return p
	}
	return p
}

func (p *plugin) Init(params plugins.InitParams) error {
	return p.constructorErr
}

// we do not need to update any fields, just check that the input is valid
func (p *plugin) UpdateUpstream(original, desired *v1.Upstream) (bool, error) {
	originalSpec, ok := original.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_AwsEc2)
	if !ok {
		return false, errors.Errorf("internal error: expected *v1.UpstreamSpec_AwsEc2, got %v", reflect.TypeOf(original.UpstreamSpec.UpstreamType).Name())
	}
	desiredSpec, ok := desired.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_AwsEc2)
	if !ok {
		return false, errors.Errorf("internal error: expected *v1.UpstreamSpec_AwsEc2, got %v", reflect.TypeOf(original.UpstreamSpec.UpstreamType).Name())
	}
	if !originalSpec.Equal(desiredSpec) {
		return false, errors.New("expected no difference between *v1.UpstreamSpec_AwsEc2 upstreams")
	}
	return false, nil
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoyapi.Cluster) error {
	_, ok := in.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_AwsEc2)
	if !ok {
		return nil
	}

	// configure the cluster to use EDS:ADS and call it a day
	xds.SetEdsOnCluster(out)
	return nil
}
