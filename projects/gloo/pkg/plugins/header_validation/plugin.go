package header_validation

import (

	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

// Plugin for header validation.
// This plugin is intended to serve as a location for header validation in
// Envoy. Currently, it is quite bare and only sets a single option in Envoy
// that disables the HTTP/1 parser from filtering requests with custom HTTP
// methods. However, when Universal Header Validation is enabled in upstream
// Envoy, this plugin will serve as a location to configure all potential UHV
// features (including allowing custom HTTP methods).

var (
	_ plugins.Plugin                      = new(plugin)
	_ plugins.HttpConnectionManagerPlugin = new(plugin)
)

const (
  ExtensionName = "header_validation"
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

func (p *plugin) ProcessHcmNetworkFilter(params plugins.Params, _ *v1.Listener, listener *v1.HttpListener, out *envoyhttp.HttpConnectionManager) error {
  in := listener.GetOptions().GetHeaderValidationSettings()
  if allow_custom_methods := in.GetAllowCustomHeaderMethods(); allow_custom_methods {
		if out.GetHttpProtocolOptions() == nil {
      out.HttpProtocolOptions = &envoycore.Http1ProtocolOptions{}
    }
    out.HttpProtocolOptions.AllowCustomMethods = allow_custom_methods
  }
  return nil
}
