package tls_inspector

import (
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_tls_inspector "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

func NewPlugin() *plugin {
	return &plugin{}
}

var (
	_ plugins.Plugin         = new(plugin)
	_ plugins.ListenerPlugin = new(plugin)
)

type plugin struct{}

func (p *plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *plugin) ProcessListener(params plugins.Params, in *v1.Listener, out *envoy_config_listener_v3.Listener) error {
	configEnvoy := &envoy_tls_inspector.TlsInspector{}
	msg, err := utils.MessageToAny(configEnvoy)
	if err != nil {
		return err
	}
	tlsInspector := &envoy_config_listener_v3.ListenerFilter{
		Name: wellknown.TlsInspector,
		ConfigType: &envoy_config_listener_v3.ListenerFilter_TypedConfig{
			TypedConfig: msg,
		},
	}

	// automatically add tls inspector when ssl is enabled
	if in.GetSslConfigurations() != nil {
		out.ListenerFilters = append(out.ListenerFilters, tlsInspector)
	} else {
		// check if ssl config is set on tcp host
		switch in.GetListenerType().(type) {
		case *v1.Listener_TcpListener:
			for _, host := range in.GetTcpListener().GetTcpHosts() {
				if host.GetSslConfig() != nil || host.GetDestination().GetForwardSniClusterName() != nil {
					out.ListenerFilters = append(out.ListenerFilters, tlsInspector)
					break
				}
			}
		}
	}

	return nil
}
