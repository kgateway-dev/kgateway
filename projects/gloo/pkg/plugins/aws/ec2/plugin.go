package ec2

import (
	"context"
	"reflect"

	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/rotisserie/eris"

	"github.com/solo-io/gloo/projects/gloo/pkg/xds"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/factory"
)

var (
	_ plugins.Plugin            = new(plugin)
	_ plugins.UpstreamPlugin    = new(plugin)
	_ discovery.DiscoveryPlugin = new(plugin)
)

const (
	ExtensionName = "aws_ec2"
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
}

func NewPlugin(ctx context.Context, secretFactory factory.ResourceClientFactory) (*plugin, error) {
	p := &plugin{}
	var err error

	if secretFactory == nil {
		return p, ConstructorInputError("secret")
	}
	p.secretClient, err = v1.NewSecretClient(ctx, secretFactory)
	if err != nil {
		return p, ConstructorGetClientError("secret", err)
	}
	if err = p.secretClient.Register(); err != nil {
		return p, ConstructorRegisterClientError("secret", err)
	}

	return p, nil
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) {
	p.settings = params.Settings
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
