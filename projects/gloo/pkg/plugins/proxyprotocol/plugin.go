package proxyprotocol

import (
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_listener_proxy_protocol "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/proxy_protocol/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

func NewPlugin() *plugin {
	return &plugin{}
}

// Compile-time assertion
var (
	_ plugins.Plugin         = new(plugin)
	_ plugins.ListenerPlugin = new(plugin)
)

const (
	ErrEnterpriseOnly = "detecting the presence of proxy protocol is an enterprise-only feature"
	ExtensionName     = "proxy_protocol"
)

type plugin struct{}

func (p *plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *plugin) PluginName() string {
	return ExtensionName
}

func (p *plugin) IsUpgrade() bool {
	return false
}

func (p *plugin) ProcessListener(params plugins.Params, in *v1.Listener, out *envoy_config_listener_v3.Listener) error {
	if !in.GetUseProxyProto().GetValue() && in.GetOptions().GetProxyProtocol() == nil {
		// If UseProxyProto is not defined on the listener, or it is false, do not append the filter
		return nil
	}

	if UsesEnterpriseOnlyFeatures(in) {
		return eris.New(ErrEnterpriseOnly)
	}

	listenerFilter, err := GenerateProxyProtocolListenerFilter(in)
	if err != nil {
		return err
	}
	out.ListenerFilters = append(out.GetListenerFilters(), listenerFilter)
	return nil
}

func GenerateProxyProtocolListenerFilter(in *v1.Listener) (*envoy_config_listener_v3.ListenerFilter, error) {
	envoyProxyProtocol := &envoy_listener_proxy_protocol.ProxyProtocol{}

	if pp := in.GetOptions().GetProxyProtocol(); pp != nil {

		var rules []*envoy_listener_proxy_protocol.ProxyProtocol_Rule
		for _, rule := range pp.GetRules() {

			var tlvPresent *envoy_listener_proxy_protocol.ProxyProtocol_KeyValuePair
			if rule.GetOnTlvPresent() != nil {
				tlvPresent = &envoy_listener_proxy_protocol.ProxyProtocol_KeyValuePair{
					MetadataNamespace: rule.GetOnTlvPresent().GetMetadataNamespace(),
					Key:               rule.GetOnTlvPresent().GetKey(),
				}
			}

			rules = append(rules, &envoy_listener_proxy_protocol.ProxyProtocol_Rule{
				TlvType:      rule.GetTlvType(),
				OnTlvPresent: tlvPresent,
			})
		}
		envoyProxyProtocol.Rules = rules
	}

	msg, err := utils.MessageToAny(envoyProxyProtocol)
	if err != nil {
		return nil, err
	}

	return &envoy_config_listener_v3.ListenerFilter{
		Name: wellknown.ProxyProtocol,
		ConfigType: &envoy_config_listener_v3.ListenerFilter_TypedConfig{
			TypedConfig: msg,
		},
	}, nil
}

func UsesEnterpriseOnlyFeatures(in *v1.Listener) bool {
	return in.GetOptions().GetProxyProtocol().GetAllowRequestsWithoutProxyProtocol()
}
