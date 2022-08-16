package tls_inspector

import (
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_tls_inspector "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var (
	_ plugins.Plugin         = new(plugin)
	_ plugins.ListenerPlugin = new(plugin)
)

const (
	ExtensionName = "tls_inspector"
)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(_ plugins.InitParams) {
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

	// the method for inspecting each listener for ssl config varies by listener type
	if shouldIncludeTlsInspectorListenerFilter(in) {
		out.ListenerFilters = append(out.GetListenerFilters(), tlsInspector)
	}

	return nil
}

func shouldIncludeTlsInspectorListenerFilter(in *v1.Listener) bool {
	return includeTlsInspectorForListener(in) ||
		includeTlsInspectorForTcpListener(in.GetTcpListener()) ||
		includeTlsInspectorForHybridListener(in.GetHybridListener())
}

func includeTlsInspectorForListener(in *v1.Listener) bool {
	// automatically add tls inspector when ssl is enabled
	return in.GetSslConfigurations() != nil
}

func includeTlsInspectorForTcpListener(in *v1.TcpListener) bool {
	// check if ssl config is set on tcp host
	for _, host := range in.GetTcpHosts() {
		if host.GetSslConfig() != nil || host.GetDestination().GetForwardSniClusterName() != nil {
			return true
		}
	}
	return false
}

func includeTlsInspectorForHybridListener(in *v1.HybridListener) bool {
	for _, matchedListener := range in.GetMatchedListeners() {
		switch matchedListener.GetListenerType().(type) {
		case *v1.MatchedListener_TcpListener:
			// If a sub-TCPListener includes ssl config, return true
			if includeTlsInspectorForTcpListener(matchedListener.GetTcpListener()) ||
				matchedListener.GetMatcher().GetSslConfig() != nil {
				return true
			}

		case *v1.MatchedListener_HttpListener:
			// If a sub-HttpListener includes ssl config, return true
			if matchedListener.GetMatcher().GetSslConfig() != nil {
				return true
			}

		}
	}

	return false
}
