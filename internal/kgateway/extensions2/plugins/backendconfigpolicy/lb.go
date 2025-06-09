package backendconfigpolicy

import (
	"fmt"
	"strconv"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func applyLoadBalancerConfig(config *v1alpha1.LoadBalancerConfig, out *clusterv3.Cluster) {
	if config.HealthyPanicThreshold != nil || config.UpdateMergeWindow != nil ||
		config.LocalityConfigType != nil || config.CloseConnectionsOnHostSetChange != nil {
		out.CommonLbConfig = &clusterv3.Cluster_CommonLbConfig{}
		if config.HealthyPanicThreshold != nil {
			out.GetCommonLbConfig().HealthyPanicThreshold = &typev3.Percent{
				Value: float64(*config.HealthyPanicThreshold),
			}
		}
		if config.UpdateMergeWindow != nil {
			out.GetCommonLbConfig().UpdateMergeWindow = durationpb.New(config.UpdateMergeWindow.Duration)
		}
		if config.LocalityConfigType != nil {
			switch *config.LocalityConfigType {
			case v1alpha1.LocalityConfigTypeWeightedLb:
				out.GetCommonLbConfig().LocalityConfigSpecifier = &clusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
					LocalityWeightedLbConfig: &clusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
				}
			}
		}
		if config.CloseConnectionsOnHostSetChange != nil {
			out.GetCommonLbConfig().CloseConnectionsOnHostSetChange = *config.CloseConnectionsOnHostSetChange
		}
	}

	if config.Type != nil {
		switch *config.Type {
		case v1alpha1.LoadBalancerTypeLeastRequest:
			configureLeastRequestLb(out, config.LeastRequest)
		case v1alpha1.LoadBalancerTypeRoundRobin:
			configureRoundRobinLb(out, config.RoundRobin)
		case v1alpha1.LoadBalancerTypeRingHash:
			setRingHashLbConfig(out, config.RingHash)
		case v1alpha1.LoadBalancerTypeMaglev:
			out.LbPolicy = clusterv3.Cluster_MAGLEV
		case v1alpha1.LoadBalancerTypeRandom:
			out.LbPolicy = clusterv3.Cluster_RANDOM
		}
	}

	if config.UseHostnameForHashing != nil {
		if out.GetCommonLbConfig() == nil {
			out.CommonLbConfig = &clusterv3.Cluster_CommonLbConfig{}
		}
		out.GetCommonLbConfig().ConsistentHashingLbConfig = &clusterv3.Cluster_CommonLbConfig_ConsistentHashingLbConfig{
			UseHostnameForHashing: *config.UseHostnameForHashing,
		}
	}
}

func configureRoundRobinLb(out *clusterv3.Cluster, cfg *v1alpha1.LoadBalancerRoundRobinConfig) {
	out.LbPolicy = clusterv3.Cluster_ROUND_ROBIN

	if cfg == nil {
		return
	}
	slowStartConfig := toSlowStartConfig(out, cfg.SlowStartConfig)
	if slowStartConfig != nil {
		out.LbConfig = &clusterv3.Cluster_RoundRobinLbConfig_{
			RoundRobinLbConfig: &clusterv3.Cluster_RoundRobinLbConfig{
				SlowStartConfig: slowStartConfig,
			},
		}
	}
}

func configureLeastRequestLb(out *clusterv3.Cluster, cfg *v1alpha1.LoadBalancerLeastRequestConfig) {
	out.LbPolicy = clusterv3.Cluster_LEAST_REQUEST

	if cfg == nil {
		return
	}

	var choiceCount *wrapperspb.UInt32Value
	if cfg.ChoiceCount != nil {
		choiceCount = &wrapperspb.UInt32Value{
			Value: *cfg.ChoiceCount,
		}
	}
	slowStartConfig := toSlowStartConfig(out, cfg.SlowStartConfig)
	if choiceCount != nil || slowStartConfig != nil {
		out.LbConfig = &clusterv3.Cluster_LeastRequestLbConfig_{
			LeastRequestLbConfig: &clusterv3.Cluster_LeastRequestLbConfig{
				ChoiceCount:     choiceCount,
				SlowStartConfig: slowStartConfig,
			},
		}
	}
}

func toSlowStartConfig(clusterInfo *clusterv3.Cluster, cfg *v1alpha1.SlowStartConfig) *clusterv3.Cluster_SlowStartConfig {
	if cfg == nil {
		return nil
	}
	out := clusterv3.Cluster_SlowStartConfig{
		SlowStartWindow: durationpb.New(cfg.Window.Duration),
	}
	if cfg.Aggression != "" {
		aggressionValue, err := strconv.ParseFloat(cfg.Aggression, 64)
		if err != nil {
			// This should ideally not happen due to CRD validation
			logger.Error("failed to parse aggression value", "error", err)
			return nil
		}
		runtimeKeyPrefix := "upstream"

		if clusterInfo.GetName() != "" {
			runtimeKeyPrefix = fmt.Sprintf("%s.%s", runtimeKeyPrefix, clusterInfo.GetName())
		}

		out.Aggression = &corev3.RuntimeDouble{
			DefaultValue: aggressionValue,
			RuntimeKey:   fmt.Sprintf("%s.slowStart.aggression", runtimeKeyPrefix),
		}
	}
	if cfg.MinWeightPercent != nil {
		out.MinWeightPercent = &typev3.Percent{
			Value: float64(*cfg.MinWeightPercent),
		}
	}
	return &out
}

func setRingHashLbConfig(out *clusterv3.Cluster, userConfig *v1alpha1.LoadBalancerRingHashConfig) {
	out.LbPolicy = clusterv3.Cluster_RING_HASH
	cfg := &clusterv3.Cluster_RingHashLbConfig_{
		RingHashLbConfig: &clusterv3.Cluster_RingHashLbConfig{},
	}
	if userConfig != nil {
		if userConfig.MinimumRingSize != nil {
			cfg.RingHashLbConfig.MinimumRingSize = &wrapperspb.UInt64Value{
				Value: *userConfig.MinimumRingSize,
			}
		}
		if userConfig.MaximumRingSize != nil {
			cfg.RingHashLbConfig.MaximumRingSize = &wrapperspb.UInt64Value{
				Value: *userConfig.MaximumRingSize,
			}
		}
	}
	out.LbConfig = cfg
}

func equalsLoadBalancerConfig(a, b *v1alpha1.LoadBalancerConfig) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	if (a.HealthyPanicThreshold == nil) != (b.HealthyPanicThreshold == nil) {
		return false
	}
	if a.HealthyPanicThreshold != nil && *a.HealthyPanicThreshold != *b.HealthyPanicThreshold {
		return false
	}

	if (a.UpdateMergeWindow == nil) != (b.UpdateMergeWindow == nil) {
		return false
	}
	if a.UpdateMergeWindow != nil && *a.UpdateMergeWindow != *b.UpdateMergeWindow {
		return false
	}

	if (a.Type == nil) != (b.Type == nil) {
		return false
	}
	if a.Type != nil && *a.Type != *b.Type {
		return false
	}

	if !equalsLeastRequest(a.LeastRequest, b.LeastRequest) {
		return false
	}

	if !equalsRoundRobin(a.RoundRobin, b.RoundRobin) {
		return false
	}

	if !equalsRingHash(a.RingHash, b.RingHash) {
		return false
	}

	if (a.LocalityConfigType == nil) != (b.LocalityConfigType == nil) {
		return false
	}
	if a.LocalityConfigType != nil && *a.LocalityConfigType != *b.LocalityConfigType {
		return false
	}

	if (a.UseHostnameForHashing == nil) != (b.UseHostnameForHashing == nil) {
		return false
	}
	if a.UseHostnameForHashing != nil && *a.UseHostnameForHashing != *b.UseHostnameForHashing {
		return false
	}

	if (a.CloseConnectionsOnHostSetChange == nil) != (b.CloseConnectionsOnHostSetChange == nil) {
		return false
	}
	if a.CloseConnectionsOnHostSetChange != nil && *a.CloseConnectionsOnHostSetChange != *b.CloseConnectionsOnHostSetChange {
		return false
	}

	return true
}

func equalsLeastRequest(a, b *v1alpha1.LoadBalancerLeastRequestConfig) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	if (a.ChoiceCount == nil) != (b.ChoiceCount == nil) {
		return false
	}
	if a.ChoiceCount != nil && *a.ChoiceCount != *b.ChoiceCount {
		return false
	}
	return equalsSlowStart(a.SlowStartConfig, b.SlowStartConfig)
}

func equalsRoundRobin(a, b *v1alpha1.LoadBalancerRoundRobinConfig) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return equalsSlowStart(a.SlowStartConfig, b.SlowStartConfig)
}

func equalsRingHash(a, b *v1alpha1.LoadBalancerRingHashConfig) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	if (a.MinimumRingSize == nil) != (b.MinimumRingSize == nil) {
		return false
	}
	if a.MinimumRingSize != nil && *a.MinimumRingSize != *b.MinimumRingSize {
		return false
	}

	if (a.MaximumRingSize == nil) != (b.MaximumRingSize == nil) {
		return false
	}
	if a.MaximumRingSize != nil && *a.MaximumRingSize != *b.MaximumRingSize {
		return false
	}

	return true
}

func equalsSlowStart(a, b *v1alpha1.SlowStartConfig) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	if (a.Window == nil) != (b.Window == nil) {
		return false
	}
	if a.Window != nil && *a.Window != *b.Window {
		return false
	}

	if a.Aggression != b.Aggression {
		return false
	}

	if (a.MinWeightPercent == nil) != (b.MinWeightPercent == nil) {
		return false
	}
	if a.MinWeightPercent != nil && *a.MinWeightPercent != *b.MinWeightPercent {
		return false
	}

	return true
}
