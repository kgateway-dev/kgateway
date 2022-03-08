package dynamic_forward_proxy

import (
	"fmt"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_extensions_clusters_dynamic_forward_proxy_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dynamic_forward_proxy/v3"
	envoy_extensions_common_dynamic_forward_proxy_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/dynamic_forward_proxy/v3"
	envoy_extensions_filters_http_dynamic_forward_proxy_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_forward_proxy/v3"
	"github.com/golang/protobuf/ptypes/duration"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/dynamic_forward_proxy"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/hashutils"
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

type plugin struct {
	filterHashMap map[uint64]*dynamic_forward_proxy.FilterConfig
}

func (p *plugin) GeneratedResources(params plugins.Params, inClusters []*envoy_config_cluster_v3.Cluster, inEndpoints []*envoy_config_endpoint_v3.ClusterLoadAssignment, inRouteConfigurations []*envoy_config_route_v3.RouteConfiguration, inListeners []*envoy_config_listener_v3.Listener) ([]*envoy_config_cluster_v3.Cluster, []*envoy_config_endpoint_v3.ClusterLoadAssignment, []*envoy_config_route_v3.RouteConfiguration, []*envoy_config_listener_v3.Listener, error) {
	var generatedClusters []*envoy_config_cluster_v3.Cluster
	for _, lCfg := range p.filterHashMap {
		generatedClusters = append(generatedClusters, generateCustomDynamicForwardProxyCluster(lCfg))
	}
	return generatedClusters, nil, nil, nil, nil
}

// envoy is silly and thus dynamic forward proxy DNS config must be identical across HTTP filter and cluster config,
// https://github.com/envoyproxy/envoy/blob/v1.21.1/source/extensions/filters/http/dynamic_forward_proxy/proxy_filter.cc#L129-L132
//
// to be nice, we hide this behavior from the user and generate a cluster for each DNS cache config as provided
// in our http listener options.
//
// as a result of this, the generated cluster is very simple (e.g., no TLS config). this is intentional as the provided
// use case did not require it, and I wanted to keep the number of dangerous user configurations to a minimum. we could
// add a new upstream type in the future for dynamic forwarding and make other cluster fields configurable, but this
// would require very careful validation with all other features and require the extra user step of providing an
// upstream that in most cases the user does not want to customize at all.
func generateCustomDynamicForwardProxyCluster(lCfg *dynamic_forward_proxy.FilterConfig) *envoy_config_cluster_v3.Cluster {
	cc := &envoy_extensions_clusters_dynamic_forward_proxy_v3.ClusterConfig{
		DnsCacheConfig: getDnsCacheConfig(lCfg),
		// AllowInsecureClusterOptions is not needed to be configurable unless we make a
		// new upstream type so the cluster's upstream_http_protocol_options is configurable
		AllowInsecureClusterOptions: false,
		AllowCoalescedConnections:   false, // not-implemented in envoy yet
	}
	marshalledConf, err := utils.MessageToAny(cc)
	if err != nil {
		// this should NEVER HAPPEN!
		panic(err)
	}
	return &envoy_config_cluster_v3.Cluster{
		Name:           GetGeneratedClusterName(lCfg), //TODO(kdorosh) non-nil
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

func GetGeneratedClusterName(dfpListenerConf *dynamic_forward_proxy.FilterConfig) string {
	// TODO(kdorosh) generate cluster per route for each listener..
	hash := hashutils.MustHash(dfpListenerConf)
	return fmt.Sprintf("placeholder_gloo-system:%v", hash)
}

func getDnsCacheConfig(dfpListenerConf *dynamic_forward_proxy.FilterConfig) *envoy_extensions_common_dynamic_forward_proxy_v3.DnsCacheConfig {
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
	cpDfp := listener.GetOptions().GetDynamicForwardProxy()
	if cpDfp == nil {
		// TODO(kdorosh) add default dns config
		//return []plugins.StagedHttpFilter{}, nil
	}

	dfp := &envoy_extensions_filters_http_dynamic_forward_proxy_v3.FilterConfig{
		DnsCacheConfig: getDnsCacheConfig(cpDfp),
		//SaveUpstreamAddress: false,
	}

	hash := hashutils.MustHash(cpDfp)
	p.filterHashMap[hash] = cpDfp

	c, err := plugins.NewStagedFilterWithConfig(FilterName, dfp, pluginStage)
	if err != nil {
		return []plugins.StagedHttpFilter{}, err
	}
	return []plugins.StagedHttpFilter{c}, nil
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	dfpCfg := in.GetRouteAction().GetDynamicForwardProxy()
	if dfpCfg == nil {
		return nil
	}
	dfpRouteCfg := &envoy_extensions_filters_http_dynamic_forward_proxy_v3.PerRouteConfig{}

	dfpRouteCfg.HostRewriteSpecifier = &envoy_extensions_filters_http_dynamic_forward_proxy_v3.PerRouteConfig_HostRewriteHeader{
		HostRewriteHeader: "x-rewrite-me",
	}

	//switch d := dfpCfg.GetHostRewriteSpecifier().(type) {
	//case *dynamic_forward_proxy.PerRouteConfig_HostRewrite:
	//	dfpRouteCfg.HostRewriteSpecifier = &envoy_extensions_filters_http_dynamic_forward_proxy_v3.PerRouteConfig_HostRewriteLiteral{
	//		HostRewriteLiteral: d.HostRewrite,
	//	}
	//case *dynamic_forward_proxy.PerRouteConfig_AutoHostRewriteHeader:
	//	dfpRouteCfg.HostRewriteSpecifier = &envoy_extensions_filters_http_dynamic_forward_proxy_v3.PerRouteConfig_HostRewriteHeader{
	//		HostRewriteHeader: d.AutoHostRewriteHeader,
	//	}
	//default:
	//	return eris.Errorf("unimplemented dynamic forward proxy route config type %T", d)
	//}
	return pluginutils.SetRoutePerFilterConfig(out, FilterName, dfpRouteCfg)
}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(_ plugins.InitParams) error {
	p.filterHashMap = map[uint64]*dynamic_forward_proxy.FilterConfig{}
	return nil
}
