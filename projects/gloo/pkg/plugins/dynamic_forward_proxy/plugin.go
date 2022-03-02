package dynamic_forward_proxy

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_extensions_filters_http_dynamic_forward_proxy_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_forward_proxy/v3"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/dynamic_forward_proxy"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
)

var (
	_ plugins.Plugin      = new(plugin)
	_ plugins.RoutePlugin = new(plugin)
)

const (
	ExtensionName = "dynamic-forward-proxy"
	FilterName    = "envoy.filters.http.dynamic_forward_proxy"
)

type plugin struct{}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	dfpCfg := in.GetOptions().GetDynamicForwardProxy()
	if dfpCfg == nil {
		return nil
	}
	dfpRouteCfg := &envoy_extensions_filters_http_dynamic_forward_proxy_v3.PerRouteConfig{}
	switch d := dfpCfg.HostRewriteSpecifier.(type) {
	case *dynamic_forward_proxy.PerRouteConfig_HostRewrite:
		dfpRouteCfg.HostRewriteSpecifier = &envoy_extensions_filters_http_dynamic_forward_proxy_v3.PerRouteConfig_HostRewriteLiteral{
			HostRewriteLiteral: d.HostRewrite,
		}
	case *dynamic_forward_proxy.PerRouteConfig_AutoHostRewriteHeader:
		dfpRouteCfg.HostRewriteSpecifier = &envoy_extensions_filters_http_dynamic_forward_proxy_v3.PerRouteConfig_HostRewriteHeader{
			HostRewriteHeader: d.AutoHostRewriteHeader,
		}
	default:
		return eris.Errorf("unimplemented dynamic forward proxy route config type %T", d)
	}
	return pluginutils.SetRoutePerFilterConfig(out, FilterName, dfpRouteCfg)
}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(_ plugins.InitParams) error {
	return nil
}
