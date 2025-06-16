package backendconfigpolicy

import (
	"fmt"
	"strconv"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

type LoadBalancerConfigIR struct {
	commonLbConfig       *clusterv3.Cluster_CommonLbConfig
	lbPolicy             clusterv3.Cluster_LbPolicy
	roundRobinLbConfig   *clusterv3.Cluster_RoundRobinLbConfig
	leastRequestLbConfig *clusterv3.Cluster_LeastRequestLbConfig
	ringHashLbConfig     *clusterv3.Cluster_RingHashLbConfig
	slowStartConfigIR    *slowStartConfigIR
}

type slowStartConfigIR struct {
	slowStartConfig *clusterv3.Cluster_SlowStartConfig
	aggression      string
}

func translateLoadBalancerConfig(config *v1alpha1.LoadBalancer) *LoadBalancerConfigIR {
	out := &LoadBalancerConfigIR{}

	out.commonLbConfig = &clusterv3.Cluster_CommonLbConfig{}

	out.commonLbConfig.ConsistentHashingLbConfig = &clusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{
		UseHostnameForHashing: config.UseHostnameForHashing,
	}

	if config.HealthyPanicThreshold != nil {
		out.commonLbConfig.HealthyPanicThreshold = &typev3.Percent{
			Value: float64(*config.HealthyPanicThreshold),
		}
	}

	if config.UpdateMergeWindow != nil {
		out.commonLbConfig.UpdateMergeWindow = durationpb.New(config.UpdateMergeWindow.Duration)
	}

	if config.LocalityType != nil {
		switch *config.LocalityType {
		case v1alpha1.LocalityConfigTypeWeightedLb:
			out.commonLbConfig.LocalityConfigSpecifier = &clusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
				LocalityWeightedLbConfig: &clusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
			}
		}
	}

	if config.CloseConnectionsOnHostSetChange != nil {
		out.commonLbConfig.CloseConnectionsOnHostSetChange = *config.CloseConnectionsOnHostSetChange
	}

	if config.LeastRequest != nil {
		out.lbPolicy = clusterv3.Cluster_LEAST_REQUEST
		out.leastRequestLbConfig = &clusterv3.Cluster_LeastRequestLbConfig{}
		out.leastRequestLbConfig.ChoiceCount = &wrapperspb.UInt32Value{
			Value: config.LeastRequest.ChoiceCount,
		}
		out.slowStartConfigIR = toSlowStartConfigIR(config.LeastRequest.SlowStart)
	} else if config.RoundRobin != nil {
		out.lbPolicy = clusterv3.Cluster_ROUND_ROBIN
		out.slowStartConfigIR = toSlowStartConfigIR(config.RoundRobin.SlowStart)
	} else if config.RingHash != nil {
		out.lbPolicy = clusterv3.Cluster_RING_HASH
		out.ringHashLbConfig = &clusterv3.Cluster_RingHashLbConfig{}
		if config.RingHash.MinimumRingSize != nil {
			out.ringHashLbConfig.MinimumRingSize = &wrapperspb.UInt64Value{
				Value: *config.RingHash.MinimumRingSize,
			}
		}
		if config.RingHash.MaximumRingSize != nil {
			out.ringHashLbConfig.MaximumRingSize = &wrapperspb.UInt64Value{
				Value: *config.RingHash.MaximumRingSize,
			}
		}
	} else if config.Maglev != nil {
		out.lbPolicy = clusterv3.Cluster_MAGLEV
	} else if config.Random != nil {
		out.lbPolicy = clusterv3.Cluster_RANDOM
	}

	return out
}

func applyLoadBalancerConfig(config *LoadBalancerConfigIR, out *clusterv3.Cluster) {
	if config == nil {
		return
	}

	out.CommonLbConfig = config.commonLbConfig
	out.LbPolicy = config.lbPolicy
	switch config.lbPolicy {
	case clusterv3.Cluster_ROUND_ROBIN:
		configureRoundRobinLb(out, config)
	case clusterv3.Cluster_LEAST_REQUEST:
		configureLeastRequestLb(out, config)
	case clusterv3.Cluster_RING_HASH:
		out.LbConfig = &clusterv3.Cluster_RingHashLbConfig_{
			RingHashLbConfig: config.ringHashLbConfig,
		}
	}
}

func configureRoundRobinLb(out *clusterv3.Cluster, cfg *LoadBalancerConfigIR) {
	if cfg == nil {
		return
	}
	slowStartConfig := toSlowStartConfig(cfg.slowStartConfigIR, out.GetName())
	if slowStartConfig != nil {
		out.LbConfig = &clusterv3.Cluster_RoundRobinLbConfig_{
			RoundRobinLbConfig: &clusterv3.Cluster_RoundRobinLbConfig{
				SlowStartConfig: slowStartConfig,
			},
		}
	}
}

func configureLeastRequestLb(out *clusterv3.Cluster, cfg *LoadBalancerConfigIR) {
	out.LbPolicy = clusterv3.Cluster_LEAST_REQUEST

	if cfg == nil {
		return
	}

	var choiceCount *wrapperspb.UInt32Value
	if cfg.leastRequestLbConfig != nil && cfg.leastRequestLbConfig.GetChoiceCount() != nil {
		choiceCount = cfg.leastRequestLbConfig.GetChoiceCount()
	}

	slowStartConfig := toSlowStartConfig(cfg.slowStartConfigIR, out.GetName())
	if choiceCount != nil || slowStartConfig != nil {
		out.LbConfig = &clusterv3.Cluster_LeastRequestLbConfig_{
			LeastRequestLbConfig: &clusterv3.Cluster_LeastRequestLbConfig{
				ChoiceCount:     choiceCount,
				SlowStartConfig: slowStartConfig,
			},
		}
	}
}

func toSlowStartConfigIR(cfg *v1alpha1.SlowStart) *slowStartConfigIR {
	if cfg == nil {
		return nil
	}
	slowStart := clusterv3.Cluster_SlowStartConfig{
		SlowStartWindow: durationpb.New(cfg.Window.Duration),
	}
	if cfg.MinWeightPercent != nil {
		slowStart.MinWeightPercent = &typev3.Percent{
			Value: float64(*cfg.MinWeightPercent),
		}
	}
	return &slowStartConfigIR{
		slowStartConfig: &slowStart,
		aggression:      cfg.Aggression,
	}
}

func toSlowStartConfig(ir *slowStartConfigIR, clusterName string) *clusterv3.Cluster_SlowStartConfig {
	if ir == nil {
		return nil
	}
	out := ir.slowStartConfig
	if ir.aggression != "" {
		aggressionValue, err := strconv.ParseFloat(ir.aggression, 64)
		if err != nil {
			// This should ideally not happen due to CRD validation
			logger.Error("error parsing slowStartConfig.aggression", "error", err, "cluster", clusterName)
			return nil
		}
		// Envoy requires runtime key for RuntimeDouble types,
		// so use a cluster-specific runtime key.
		// See https://github.com/kgateway-dev/kgateway/pull/9031
		runtimeKeyPrefix := "upstream"
		if clusterName != "" {
			runtimeKeyPrefix = fmt.Sprintf("%s.%s", runtimeKeyPrefix, clusterName)
		}

		out.Aggression = &corev3.RuntimeDouble{
			DefaultValue: aggressionValue,
			RuntimeKey:   fmt.Sprintf("%s.slowStart.aggression", runtimeKeyPrefix),
		}
	}
	return out
}

func (a *LoadBalancerConfigIR) Equals(b *LoadBalancerConfigIR) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if !proto.Equal(a.commonLbConfig, b.commonLbConfig) {
		return false
	}
	if a.lbPolicy != b.lbPolicy {
		return false
	}
	if !proto.Equal(a.roundRobinLbConfig, b.roundRobinLbConfig) {
		return false
	}
	if !proto.Equal(a.leastRequestLbConfig, b.leastRequestLbConfig) {
		return false
	}
	if !proto.Equal(a.ringHashLbConfig, b.ringHashLbConfig) {
		return false
	}
	if (a.slowStartConfigIR == nil) != (b.slowStartConfigIR == nil) {
		return false
	}
	if a.slowStartConfigIR != nil && b.slowStartConfigIR != nil {
		if !proto.Equal(a.slowStartConfigIR.slowStartConfig, b.slowStartConfigIR.slowStartConfig) {
			return false
		}
		if a.slowStartConfigIR.aggression != b.slowStartConfigIR.aggression {
			return false
		}
	}
	return true
}
