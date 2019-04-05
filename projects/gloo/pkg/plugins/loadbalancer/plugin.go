package loadbalancer

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type"
	types "github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

type Plugin struct{}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) ProcessRoute(params plugins.Params, in *v1.Route, out *envoyroute.Route) error {
	// TODO(yuval-k): add ring hash config
	return nil
}

func (p *Plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoyapi.Cluster) error {

	cfg := in.GetUpstreamSpec().GetLoadBalancerConfig()
	if cfg == nil {
		return nil
	}

	if cfg.HealthyPanicThreshold != nil || cfg.UpdateMergeWindow != nil {
		out.CommonLbConfig = &envoyapi.Cluster_CommonLbConfig{}
		if cfg.HealthyPanicThreshold != nil {
			out.CommonLbConfig.HealthyPanicThreshold = &envoytype.Percent{
				Value: cfg.HealthyPanicThreshold.Value,
			}
		}
		if cfg.UpdateMergeWindow != nil {
			out.CommonLbConfig.UpdateMergeWindow = types.DurationProto(*cfg.UpdateMergeWindow)
		}
	}

	if cfg.Type != nil {
		switch lbtype := cfg.Type.(type) {
		case (*v1.LoadBalancerConfig_RoundRobin_):
			out.LbPolicy = envoyapi.Cluster_ROUND_ROBIN
		case (*v1.LoadBalancerConfig_LeastRequest_):
			out.LbPolicy = envoyapi.Cluster_LEAST_REQUEST
			if lbtype.LeastRequest.ChoiceCount != 0 {
				out.LbConfig = &envoyapi.Cluster_LeastRequestLbConfig_{
					LeastRequestLbConfig: &envoyapi.Cluster_LeastRequestLbConfig{
						ChoiceCount: &types.UInt32Value{
							Value: lbtype.LeastRequest.ChoiceCount,
						},
					},
				}
			}
		case (*v1.LoadBalancerConfig_Random_):
			out.LbPolicy = envoyapi.Cluster_RANDOM
		}
	}

	return nil
}
