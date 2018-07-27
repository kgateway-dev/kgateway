package kubernetes

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/pkg/plugins"
)

func init() {
	plugins.Register(&Plugin{})
}

//go:generate protoc -I=./ -I=${GOPATH}/src/github.com/gogo/protobuf/ -I=${GOPATH}/src/github.com/gogo/protobuf/protobuf/ --gogo_out=Mgoogle/protobuf/wrappers.proto=github.com/gogo/protobuf/types:${GOPATH}/src spec.proto

func (p *Plugin) SetupEndpointDiscovery(opts bootstrap.Options) (endpointdiscovery.Interface, error) {
	kubeConfig := opts.KubeOptions.KubeConfig
	masterUrl := opts.KubeOptions.MasterURL
	resyncDuration := opts.ConfigStorageOptions.SyncFrequency
	disc, err := NewEndpointDiscovery(masterUrl, kubeConfig, resyncDuration)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Kubernetes endpoint discovery")
	}
	return disc, err
}

type Plugin struct{
	opts bootstrap.Options
}

const (
	// define Upstream type name
	UpstreamTypeKube = "kubernetes"
)

func (p *Plugin) Init(options bootstrap.Options) error{
	p.opts = options
	return nil
}

func (p *Plugin) ProcessUpstream(_ *plugins.UpstreamPluginParams, in *v1.Upstream, out *envoyapi.Cluster) error {
	if in.Type != UpstreamTypeKube {
		return nil
	}
	// decode does validation for us
	if _, err := DecodeUpstreamSpec(in.Spec); err != nil {
		return errors.Wrap(err, "invalid kubernetes upstream spec")
	}

	// just configure the cluster to use EDS:ADS and call it a day
	out.Type = envoyapi.Cluster_EDS
	out.EdsClusterConfig = &envoyapi.Cluster_EdsClusterConfig{
		EdsConfig: &envoycore.ConfigSource{
			ConfigSourceSpecifier: &envoycore.ConfigSource_Ads{
				Ads: &envoycore.AggregatedConfigSource{},
			},
		},
	}
	return nil
}
