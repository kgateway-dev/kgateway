package upstreamssl

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
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

func (p *Plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoyapi.Cluster) error {
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
