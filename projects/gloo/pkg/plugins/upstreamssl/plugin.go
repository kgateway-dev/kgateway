package upstreamssl

import (
	envoycluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

type Plugin struct{}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoycluster.Cluster) error {
	// not ours
	sslConfig := in.SslConfig
	if sslConfig == nil {
		return nil
	}

	cfg, err := utils.NewSslConfigTranslator().ResolveUpstreamSslConfig(params.Snapshot.Secrets, sslConfig)
	if err != nil {
		return err
	}
	out.TransportSocket = &envoycore.TransportSocket{
		Name:       pluginutils.TlsTransportSocket,
		ConfigType: &envoycore.TransportSocket_TypedConfig{TypedConfig: pluginutils.MustMessageToAny(cfg)},
	}
	return nil
}
