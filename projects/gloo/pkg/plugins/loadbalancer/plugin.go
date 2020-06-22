package loadbalancer

import (
	envoycluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/gogoutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/lbhash"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var _ plugins.Plugin = new(Plugin)
var _ plugins.RoutePlugin = new(Plugin)

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

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoyroute.Route) error {
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

func getHashPoliciesFromSpec(spec []*lbhash.HashPolicy) []*envoyroute.RouteAction_HashPolicy {
	var policies []*envoyroute.RouteAction_HashPolicy
	for _, s := range spec {
		policy := &envoyroute.RouteAction_HashPolicy{
			Terminal: s.Terminal,
		}
		switch keyType := s.KeyType.(type) {
		case *lbhash.HashPolicy_Header:
			policy.PolicySpecifier = &envoyroute.RouteAction_HashPolicy_Header_{
				Header: &envoyroute.RouteAction_HashPolicy_Header{
					HeaderName: keyType.Header,
				},
			}
		case *lbhash.HashPolicy_Cookie:
			policy.PolicySpecifier = &envoyroute.RouteAction_HashPolicy_Cookie_{
				Cookie: &envoyroute.RouteAction_HashPolicy_Cookie{
					Name: keyType.Cookie.Name,
					Ttl:  gogoutils.DurationStdToProto(keyType.Cookie.Ttl),
					Path: keyType.Cookie.Path,
				},
			}
		case *lbhash.HashPolicy_SourceIp:
			policy.PolicySpecifier = &envoyroute.RouteAction_HashPolicy_ConnectionProperties_{
				ConnectionProperties: &envoyroute.RouteAction_HashPolicy_ConnectionProperties{
					SourceIp: keyType.SourceIp,
				},
			}
		}
		policies = append(policies, policy)
	}
	return policies
}

func (p *Plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoycluster.Cluster) error {

	cfg := in.GetLoadBalancerConfig()
	if cfg == nil {
		return nil
	}

	if cfg.HealthyPanicThreshold != nil || cfg.UpdateMergeWindow != nil {
		out.CommonLbConfig = &envoycluster.Cluster_CommonLbConfig{}
		if cfg.HealthyPanicThreshold != nil {
			out.CommonLbConfig.HealthyPanicThreshold = &envoytype.Percent{
				Value: cfg.HealthyPanicThreshold.Value,
			}
		}
		if cfg.UpdateMergeWindow != nil {
			out.CommonLbConfig.UpdateMergeWindow = gogoutils.DurationStdToProto(cfg.UpdateMergeWindow)
		}
	}

	if cfg.Type != nil {
		switch lbtype := cfg.Type.(type) {
		case *v1.LoadBalancerConfig_RoundRobin_:
			out.LbPolicy = envoycluster.Cluster_ROUND_ROBIN
		case *v1.LoadBalancerConfig_LeastRequest_:
			out.LbPolicy = envoycluster.Cluster_LEAST_REQUEST
			if lbtype.LeastRequest.ChoiceCount != 0 {
				out.LbConfig = &envoycluster.Cluster_LeastRequestLbConfig_{
					LeastRequestLbConfig: &envoycluster.Cluster_LeastRequestLbConfig{
						ChoiceCount: &wrappers.UInt32Value{
							Value: lbtype.LeastRequest.ChoiceCount,
						},
					},
				}
			}
		case *v1.LoadBalancerConfig_Random_:
			out.LbPolicy = envoycluster.Cluster_RANDOM
		case *v1.LoadBalancerConfig_RingHash_:
			out.LbPolicy = envoycluster.Cluster_RING_HASH
			setRingHashLbConfig(out, lbtype.RingHash.RingHashConfig)
		case *v1.LoadBalancerConfig_Maglev_:
			out.LbPolicy = envoycluster.Cluster_MAGLEV
		}
	}

	return nil
}

func setRingHashLbConfig(out *envoycluster.Cluster, userConfig *v1.LoadBalancerConfig_RingHashConfig) {
	cfg := &envoycluster.Cluster_RingHashLbConfig_{
		RingHashLbConfig: &envoycluster.Cluster_RingHashLbConfig{},
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
