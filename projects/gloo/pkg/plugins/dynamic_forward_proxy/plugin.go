package dynamic_forward_proxy

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_extensions_clusters_dynamic_forward_proxy_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dynamic_forward_proxy/v3"
	envoy_extensions_common_dynamic_forward_proxy_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/dynamic_forward_proxy/v3"
	envoy_extensions_filters_http_dynamic_forward_proxy_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_forward_proxy/v3"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/dynamic_forward_proxy"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var (
	_ plugins.Plugin                  = new(plugin)
	_ plugins.RoutePlugin             = new(plugin)
	_ plugins.HttpFilterPlugin        = new(plugin)
	_ plugins.ResourceGeneratorPlugin = new(plugin)
)

const (
	ExtensionName = "dynamic-forward-proxy"
	FilterName    = "envoy.filters.http.dynamic_forward_proxy"
)

var (
	pluginStage = plugins.DuringStage(plugins.OutAuthStage)
)

type plugin struct{}

func (p *plugin) GeneratedResources(params plugins.Params, inClusters []*envoy_config_cluster_v3.Cluster, inEndpoints []*envoy_config_endpoint_v3.ClusterLoadAssignment, inRouteConfigurations []*envoy_config_route_v3.RouteConfiguration, inListeners []*envoy_config_listener_v3.Listener) ([]*envoy_config_cluster_v3.Cluster, []*envoy_config_endpoint_v3.ClusterLoadAssignment, []*envoy_config_route_v3.RouteConfiguration, []*envoy_config_listener_v3.Listener, error) {
	var generatedClusters []*envoy_config_cluster_v3.Cluster
	//for _, listener := range inListeners {
	//
	//}
	generatedClusters = append(generatedClusters, generateSelfCluster())
	return generatedClusters, nil, nil, nil, nil
}

// the initial route is updated to route to this generated cluster, which routes envoy back to itself (to the
// generated TCP listener, which forwards to the original destination)
//
// the purpose of doing this is to allow both the HTTP Connection Manager filter and TCP filter to run.
// the HTTP Connection Manager runs to allow route-level matching on HTTP parameters (such as request path),
// but then we forward the bytes as raw TCP to the HTTP Connect proxy (which can only be done on a TCP listener)
func generateSelfCluster() *envoy_config_cluster_v3.Cluster {
	cc := &envoy_extensions_clusters_dynamic_forward_proxy_v3.ClusterConfig{
		DnsCacheConfig:              getDnsCacheConfig(),
		AllowInsecureClusterOptions: false,
		AllowCoalescedConnections:   false,
	}
	marshalledConf, err := utils.MessageToAny(cc)
	if err != nil {
		// this should NEVER HAPPEN!
		panic(err)
	}
	return &envoy_config_cluster_v3.Cluster{
		Name:           "placeholder_gloo-system",
		ConnectTimeout: &duration.Duration{Seconds: 5},
		LbPolicy:       envoy_config_cluster_v3.Cluster_CLUSTER_PROVIDED,
		ClusterDiscoveryType: &envoy_config_cluster_v3.Cluster_ClusterType{
			ClusterType: &envoy_config_cluster_v3.Cluster_CustomClusterType{
				Name:        "envoy.clusters.dynamic_forward_proxy",
				TypedConfig: marshalledConf,
			},
		},
	}
}

func getDnsCacheConfig() *envoy_extensions_common_dynamic_forward_proxy_v3.DnsCacheConfig {
	return &envoy_extensions_common_dynamic_forward_proxy_v3.DnsCacheConfig{
		Name:            "dynamic_forward_proxy_cache_config",
		DnsLookupFamily: envoy_config_cluster_v3.Cluster_V4_ONLY,
		//DnsRefreshRate:         nil,
		//HostTtl:                nil,
		//MaxHosts:               nil,
		//DnsFailureRefreshRate:  nil,
		//DnsCacheCircuitBreaker: nil,
		//UseTcpForDnsLookups:    false,
		//DnsResolutionConfig:    nil,
		//TypedDnsResolverConfig: nil,
		//PreresolveHostnames:    nil,
		//DnsQueryTimeout:        nil,
		//KeyValueConfig:         nil,
	}
}

func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {

	//return []plugins.StagedHttpFilter{}, nil
	//listener.Options.

	dfp := &envoy_extensions_filters_http_dynamic_forward_proxy_v3.FilterConfig{
		DnsCacheConfig: getDnsCacheConfig(),
		//SaveUpstreamAddress: false,
	}

	c, err := plugins.NewStagedFilterWithConfig(FilterName, dfp, pluginStage)
	if err != nil {
		return []plugins.StagedHttpFilter{}, err
	}

	// put the filter in the chain, but the actual faults will be configured on the routes
	return []plugins.StagedHttpFilter{
		c,
	}, nil
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	dfpCfg := in.GetOptions().GetDynamicForwardProxy()
	if dfpCfg == nil {
		return nil
	}
	dfpRouteCfg := &envoy_extensions_filters_http_dynamic_forward_proxy_v3.PerRouteConfig{}
	switch d := dfpCfg.GetHostRewriteSpecifier().(type) {
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
