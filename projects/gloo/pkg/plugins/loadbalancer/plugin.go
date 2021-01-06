package loadbalancer

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_type_v3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/lbhash"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var _ plugins.Plugin = new(Plugin)
var _ plugins.RoutePlugin = new(Plugin)
var _ plugins.UpstreamPlugin = new(Plugin)

type Plugin struct{}

var (
	InvalidRouteTypeError = func(e error) error {
		return eris.Wrapf(e, "cannot use lbhash plugin on non-Route_Route route actions")
	}
)

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	lbPlugin := in.Options.GetLbHash()
	if lbPlugin == nil {
		return nil
	}
	if err := utils.EnsureRouteAction(out); err != nil {
		return InvalidRouteTypeError(err)
	}
	outRa := out.GetRoute()
	outRa.HashPolicy = getHashPoliciesFromSpec(lbPlugin.HashPolicies)
	return nil
}

func getHashPoliciesFromSpec(spec []*lbhash.HashPolicy) []*envoy_config_route_v3.RouteAction_HashPolicy {
	var policies []*envoy_config_route_v3.RouteAction_HashPolicy
	for _, s := range spec {
		policy := &envoy_config_route_v3.RouteAction_HashPolicy{
			Terminal: s.Terminal,
		}
		switch keyType := s.KeyType.(type) {
		case *lbhash.HashPolicy_Header:
			policy.PolicySpecifier = &envoy_config_route_v3.RouteAction_HashPolicy_Header_{
				Header: &envoy_config_route_v3.RouteAction_HashPolicy_Header{
					HeaderName: keyType.Header,
				},
			}
		case *lbhash.HashPolicy_Cookie:
			policy.PolicySpecifier = &envoy_config_route_v3.RouteAction_HashPolicy_Cookie_{
				Cookie: &envoy_config_route_v3.RouteAction_HashPolicy_Cookie{
					Name: keyType.Cookie.Name,
					Ttl:  keyType.Cookie.Ttl,
					Path: keyType.Cookie.Path,
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

func (p *Plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {

	cfg := in.GetLoadBalancerConfig()
	if cfg == nil {
		return nil
	}

	if cfg.HealthyPanicThreshold != nil || cfg.UpdateMergeWindow != nil || cfg.LocalityConfig != nil {
		out.CommonLbConfig = &envoy_config_cluster_v3.Cluster_CommonLbConfig{}
		if cfg.HealthyPanicThreshold != nil {
			out.CommonLbConfig.HealthyPanicThreshold = &envoy_type_v3.Percent{
				Value: cfg.HealthyPanicThreshold.Value,
			}
		}
		if cfg.UpdateMergeWindow != nil {
			out.CommonLbConfig.UpdateMergeWindow = cfg.UpdateMergeWindow
		}
		if cfg.LocalityConfig != nil {
			switch cfg.LocalityConfig.(type) {
			case *v1.LoadBalancerConfig_LocalityWeightedLbConfig:
				out.CommonLbConfig.LocalityConfigSpecifier = &envoy_config_cluster_v3.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
					LocalityWeightedLbConfig: &envoy_config_cluster_v3.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
				}
			}
		}
	}

	if cfg.Type != nil {
		switch lbtype := cfg.Type.(type) {
		case *v1.LoadBalancerConfig_RoundRobin_:
			out.LbPolicy = envoy_config_cluster_v3.Cluster_ROUND_ROBIN
		case *v1.LoadBalancerConfig_LeastRequest_:
			out.LbPolicy = envoy_config_cluster_v3.Cluster_LEAST_REQUEST
			if lbtype.LeastRequest.ChoiceCount != 0 {
				out.LbConfig = &envoy_config_cluster_v3.Cluster_LeastRequestLbConfig_{
					LeastRequestLbConfig: &envoy_config_cluster_v3.Cluster_LeastRequestLbConfig{
						ChoiceCount: &wrappers.UInt32Value{
							Value: lbtype.LeastRequest.ChoiceCount,
						},
					},
				}
			}
		case *v1.LoadBalancerConfig_Random_:
			out.LbPolicy = envoy_config_cluster_v3.Cluster_RANDOM
		case *v1.LoadBalancerConfig_RingHash_:
			out.LbPolicy = envoy_config_cluster_v3.Cluster_RING_HASH
			setRingHashLbConfig(out, lbtype.RingHash.RingHashConfig)
		case *v1.LoadBalancerConfig_Maglev_:
			out.LbPolicy = envoy_config_cluster_v3.Cluster_MAGLEV
		}
	}

	return nil
}

func setRingHashLbConfig(out *envoy_config_cluster_v3.Cluster, userConfig *v1.LoadBalancerConfig_RingHashConfig) {
	cfg := &envoy_config_cluster_v3.Cluster_RingHashLbConfig_{
		RingHashLbConfig: &envoy_config_cluster_v3.Cluster_RingHashLbConfig{},
	}
	if userConfig != nil {
		if userConfig.MinimumRingSize != 0 {
			cfg.RingHashLbConfig.MinimumRingSize = &wrappers.UInt64Value{
				Value: userConfig.MinimumRingSize,
			}
		}
		if userConfig.MaximumRingSize != 0 {
			cfg.RingHashLbConfig.MaximumRingSize = &wrappers.UInt64Value{
				Value: userConfig.MaximumRingSize,
			}
		}
	}
	out.LbConfig = cfg
}
