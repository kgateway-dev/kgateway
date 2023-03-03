package loadbalancer

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/lbhash"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var (
	_ plugins.Plugin         = new(plugin)
	_ plugins.RoutePlugin    = new(plugin)
	_ plugins.UpstreamPlugin = new(plugin)
)

const (
	ExtensionName = "load_balancer"
)

var (
	InvalidRouteTypeError = func(e error) error {
		return eris.Wrapf(e, "cannot use lbhash plugin on non-Route_Route route actions")
	}
)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Init(_ plugins.InitParams) {
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	lbPlugin := in.GetOptions().GetLbHash()
	if lbPlugin == nil {
		return nil
	}
	if err := utils.EnsureRouteAction(out); err != nil {
		return InvalidRouteTypeError(err)
	}
	outRa := out.GetRoute()
	outRa.HashPolicy = getHashPoliciesFromSpec(lbPlugin.GetHashPolicies())
	return nil
}

func getHashPoliciesFromSpec(spec []*lbhash.HashPolicy) []*envoy_config_route_v3.RouteAction_HashPolicy {
	var policies []*envoy_config_route_v3.RouteAction_HashPolicy
	for _, s := range spec {
		policy := &envoy_config_route_v3.RouteAction_HashPolicy{
			Terminal: s.GetTerminal(),
		}
		switch keyType := s.GetKeyType().(type) {
		case *lbhash.HashPolicy_Header:
			policy.PolicySpecifier = &envoy_config_route_v3.RouteAction_HashPolicy_Header_{
				Header: &envoy_config_route_v3.RouteAction_HashPolicy_Header{
					HeaderName: keyType.Header,
				},
			}
		case *lbhash.HashPolicy_Cookie:
			policy.PolicySpecifier = &envoy_config_route_v3.RouteAction_HashPolicy_Cookie_{
				Cookie: &envoy_config_route_v3.RouteAction_HashPolicy_Cookie{
					Name: keyType.Cookie.GetName(),
					Ttl:  keyType.Cookie.GetTtl(),
					Path: keyType.Cookie.GetPath(),
				},
			}
		case *lbhash.HashPolicy_SourceIp:
			policy.PolicySpecifier = &envoy_config_route_v3.RouteAction_HashPolicy_ConnectionProperties_{
				ConnectionProperties: &envoy_config_route_v3.RouteAction_HashPolicy_ConnectionProperties{
					SourceIp: keyType.SourceIp,
				},
			}
		}
		policies = append(policies, policy)
	}
	return policies
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {

	cfg := in.GetLoadBalancerConfig()
	if cfg == nil {
		return nil
	}

	if cfg.GetHealthyPanicThreshold() != nil || cfg.GetUpdateMergeWindow() != nil || cfg.GetLocalityConfig() != nil {
		out.CommonLbConfig = &envoy_config_cluster_v3.Cluster_CommonLbConfig{}
		if cfg.GetHealthyPanicThreshold() != nil {
			out.GetCommonLbConfig().HealthyPanicThreshold = &envoy_type_v3.Percent{
				Value: cfg.GetHealthyPanicThreshold().GetValue(),
			}
		}
		if cfg.GetUpdateMergeWindow() != nil {
			out.GetCommonLbConfig().UpdateMergeWindow = cfg.GetUpdateMergeWindow()
		}
		if cfg.GetLocalityConfig() != nil {
			switch cfg.GetLocalityConfig().(type) {
			case *v1.LoadBalancerConfig_LocalityWeightedLbConfig:
				out.GetCommonLbConfig().LocalityConfigSpecifier = &envoy_config_cluster_v3.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
					LocalityWeightedLbConfig: &envoy_config_cluster_v3.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
				}
			}
		}
	}

	if cfg.GetType() != nil {
		switch lbtype := cfg.GetType().(type) {
		case *v1.LoadBalancerConfig_RoundRobin_:
			configureRoundRobinLb(out, lbtype)
		case *v1.LoadBalancerConfig_LeastRequest_:
			configureLeastRequestLb(out, lbtype)
		case *v1.LoadBalancerConfig_Random_:
			out.LbPolicy = envoy_config_cluster_v3.Cluster_RANDOM
		case *v1.LoadBalancerConfig_RingHash_:
			out.LbPolicy = envoy_config_cluster_v3.Cluster_RING_HASH
			setRingHashLbConfig(out, lbtype.RingHash.GetRingHashConfig())
		case *v1.LoadBalancerConfig_Maglev_:
			out.LbPolicy = envoy_config_cluster_v3.Cluster_MAGLEV
		}
	}

	return nil
}

func configureRoundRobinLb(out *envoy_config_cluster_v3.Cluster, cfg *v1.LoadBalancerConfig_RoundRobin_) {
	out.LbPolicy = envoy_config_cluster_v3.Cluster_ROUND_ROBIN

	slowStartConfig := toSlowStartConfig(cfg.RoundRobin.GetSlowStartConfig())
	if slowStartConfig != nil {
		out.LbConfig = &envoy_config_cluster_v3.Cluster_RoundRobinLbConfig_{
			RoundRobinLbConfig: &envoy_config_cluster_v3.Cluster_RoundRobinLbConfig{
				SlowStartConfig: slowStartConfig,
			},
		}
	}
}

func configureLeastRequestLb(out *envoy_config_cluster_v3.Cluster, cfg *v1.LoadBalancerConfig_LeastRequest_) {
	out.LbPolicy = envoy_config_cluster_v3.Cluster_LEAST_REQUEST
	var choiceCount *wrappers.UInt32Value
	if cfg.LeastRequest.GetChoiceCount() != 0 {
		choiceCount = &wrappers.UInt32Value{
			Value: cfg.LeastRequest.GetChoiceCount(),
		}
	}

	slowStartConfig := toSlowStartConfig(cfg.LeastRequest.GetSlowStartConfig())
	if choiceCount != nil || slowStartConfig != nil {
		out.LbConfig = &envoy_config_cluster_v3.Cluster_LeastRequestLbConfig_{
			LeastRequestLbConfig: &envoy_config_cluster_v3.Cluster_LeastRequestLbConfig{
				ChoiceCount:     choiceCount,
				SlowStartConfig: slowStartConfig,
			},
		}
	}
}

func toSlowStartConfig(cfg *v1.LoadBalancerConfig_SlowStartConfig) *envoy_config_cluster_v3.Cluster_SlowStartConfig {
	if cfg == nil {
		return nil
	}
	out := envoy_config_cluster_v3.Cluster_SlowStartConfig{
		SlowStartWindow: cfg.GetSlowStartWindow(),
	}
	if cfg.GetAggression() != nil {
		out.Aggression = &envoy_config_core_v3.RuntimeDouble{
			DefaultValue: cfg.GetAggression().GetValue(),
		}
	}
	if cfg.GetMinWeightPercent() != nil {
		out.MinWeightPercent = &envoy_type_v3.Percent{
			Value: cfg.GetMinWeightPercent().GetValue(),
		}
	}
	return &out
}

func setRingHashLbConfig(out *envoy_config_cluster_v3.Cluster, userConfig *v1.LoadBalancerConfig_RingHashConfig) {
	cfg := &envoy_config_cluster_v3.Cluster_RingHashLbConfig_{
		RingHashLbConfig: &envoy_config_cluster_v3.Cluster_RingHashLbConfig{},
	}
	if userConfig != nil {
		if userConfig.GetMinimumRingSize() != 0 {
			cfg.RingHashLbConfig.MinimumRingSize = &wrappers.UInt64Value{
				Value: userConfig.GetMinimumRingSize(),
			}
		}
		if userConfig.GetMaximumRingSize() != 0 {
			cfg.RingHashLbConfig.MaximumRingSize = &wrappers.UInt64Value{
				Value: userConfig.GetMaximumRingSize(),
			}
		}
	}
	out.LbConfig = cfg
}
