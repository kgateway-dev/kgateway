package ec2

import (
	"context"
	"reflect"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/rotisserie/eris"

	"github.com/solo-io/gloo/projects/gloo/pkg/xds"

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

	settings *v1.Settings

	// pre-initialization only
	// we need to register the clients while creating the plugin, otherwise our EDS poll and upstream watch will fail
	// since Init can be called after our poll begins (race condition) we cannot create the client there
	// since NewPlugin does not return an error, we will store any non-nil errors from initializing the secret client in the plugin struct
	// we will check those errors during the Init call
	constructorErr error
}

// checks to ensure interfaces are implemented
var _ plugins.Plugin = new(plugin)
var _ plugins.UpstreamPlugin = new(plugin)
var _ discovery.DiscoveryPlugin = new(plugin)

func NewPlugin(ctx context.Context, secretFactory factory.ResourceClientFactory) *plugin {
	p := &plugin{}
	var err error
	if secretFactory == nil {
		p.constructorErr = ConstructorInputError("secret")
		return p
	}
	p.secretClient, err = v1.NewSecretClient(ctx, secretFactory)
	if err != nil {
		p.constructorErr = ConstructorGetClientError("secret", err)
		return p
	}
	if err := p.secretClient.Register(); err != nil {
		p.constructorErr = ConstructorRegisterClientError("secret", err)
		return p
	}
	return p
}

func (p *plugin) Init(params plugins.InitParams) error {
	p.settings = params.Settings
	return p.constructorErr
}

// we do not need to update any fields, just check that the input is valid
func (p *plugin) UpdateUpstream(original, desired *v1.Upstream) (bool, error) {
	originalSpec, ok := original.GetUpstreamType().(*v1.Upstream_AwsEc2)
	if !ok {
		return false, WrongUpstreamTypeError(original)
	}
	desiredSpec, ok := desired.GetUpstreamType().(*v1.Upstream_AwsEc2)
	if !ok {
		return false, WrongUpstreamTypeError(desired)
	}
	if !originalSpec.AwsEc2.Equal(desiredSpec.AwsEc2) {
		return false, UpstreamDeltaError()
	}
	return false, nil
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {
	_, ok := in.GetUpstreamType().(*v1.Upstream_AwsEc2)
	if !ok {
		return nil
	}

	// configure the cluster to use EDS:ADS and call it a day
	xds.SetEdsOnCluster(out, p.settings)
	return nil
}

var (
	ConstructorInputError = func(factoryType string) error {
		return eris.Errorf("must provide %v factory for EC2 plugin", factoryType)
	}

	ConstructorGetClientError = func(name string, err error) error {
		return eris.Wrapf(err, "unable to get %v client for EC2 plugin", name)
	}

	ConstructorRegisterClientError = func(name string, err error) error {
		return eris.Wrapf(err, "unable to register %v client for EC2 plugin", name)
	}

	WrongUpstreamTypeError = func(upstream *v1.Upstream) error {
		return eris.Errorf("internal error: expected *v1.Upstream_AwsEc2, got %v", reflect.TypeOf(upstream.GetUpstreamType()).Name())
	}

	UpstreamDeltaError = func() error {
		return eris.New("expected no difference between *v1.Upstream_AwsEc2 upstreams")
	}
)
