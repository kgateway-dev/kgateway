package backendconfigpolicy

import (
	"fmt"
	"strconv"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoycommonv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/load_balancing_policies/common/v3"
	envoyleastrequestv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/load_balancing_policies/least_request/v3"
	envoymaglevv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/load_balancing_policies/maglev/v3"
	envoyrandomv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/load_balancing_policies/random/v3"
	envoyringhashv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/load_balancing_policies/ring_hash/v3"
	envoyroundrobinv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/load_balancing_policies/round_robin/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
)

type LoadBalancerConfigIR struct {
	commonLbConfig        *envoyclusterv3.Cluster_CommonLbConfig
	loadBalancingPolicy   *envoyclusterv3.LoadBalancingPolicy
	useHostnameForHashing bool
}

func translateLoadBalancerConfig(config *v1alpha1.LoadBalancer, policyName, policyNamespace string) (*LoadBalancerConfigIR, error) {
	out := &LoadBalancerConfigIR{}

	out.commonLbConfig = &envoyclusterv3.Cluster_CommonLbConfig{}

	if config.HealthyPanicThreshold != nil {
		out.commonLbConfig.HealthyPanicThreshold = &typev3.Percent{
			Value: float64(*config.HealthyPanicThreshold),
		}
	}

	if config.UpdateMergeWindow != nil {
		out.commonLbConfig.UpdateMergeWindow = durationpb.New(config.UpdateMergeWindow.Duration)
	}

	if config.CloseConnectionsOnHostSetChange != nil {
		out.commonLbConfig.CloseConnectionsOnHostSetChange = *config.CloseConnectionsOnHostSetChange
	}

	if config.LeastRequest != nil {
		leastRequest := &envoyleastrequestv3.LeastRequest{
			ChoiceCount: &wrapperspb.UInt32Value{
				Value: config.LeastRequest.ChoiceCount,
			},
			SlowStartConfig: toSlowStartConfig(config.LeastRequest.SlowStart, policyName, policyNamespace),
		}
		if config.LocalityType != nil {
			leastRequest.LocalityLbConfig = &envoycommonv3.LocalityLbConfig{
				LocalityConfigSpecifier: &envoycommonv3.LocalityLbConfig_LocalityWeightedLbConfig_{
					LocalityWeightedLbConfig: &envoycommonv3.LocalityLbConfig_LocalityWeightedLbConfig{},
				},
			}
		}
		leastRequestAny, err := utils.MessageToAny(leastRequest)
		if err != nil {
			return nil, err
		}
		out.loadBalancingPolicy = &envoyclusterv3.LoadBalancingPolicy{
			Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
				TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
					Name:        "envoy.load_balancing_policies.least_request",
					TypedConfig: leastRequestAny,
				},
			}},
		}
	} else if config.RoundRobin != nil {
		roundRobin := &envoyroundrobinv3.RoundRobin{
			SlowStartConfig: toSlowStartConfig(config.RoundRobin.SlowStart, policyName, policyNamespace),
		}
		if config.LocalityType != nil {
			roundRobin.LocalityLbConfig = &envoycommonv3.LocalityLbConfig{
				LocalityConfigSpecifier: &envoycommonv3.LocalityLbConfig_LocalityWeightedLbConfig_{
					LocalityWeightedLbConfig: &envoycommonv3.LocalityLbConfig_LocalityWeightedLbConfig{},
				},
			}
		}
		roundRobinAny, err := utils.MessageToAny(roundRobin)
		if err != nil {
			return nil, err
		}
		out.loadBalancingPolicy = &envoyclusterv3.LoadBalancingPolicy{
			Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
				TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
					Name:        "envoy.load_balancing_policies.round_robin",
					TypedConfig: roundRobinAny,
				},
			}},
		}
	} else if config.RingHash != nil {
		ringHash := &envoyringhashv3.RingHash{}
		if config.RingHash.MinimumRingSize != nil {
			ringHash.MinimumRingSize = &wrapperspb.UInt64Value{
				Value: *config.RingHash.MinimumRingSize,
			}
		}
		if config.RingHash.MaximumRingSize != nil {
			ringHash.MaximumRingSize = &wrapperspb.UInt64Value{
				Value: *config.RingHash.MaximumRingSize,
			}
		}
		if config.RingHash.UseHostnameForHashing != nil {
			out.useHostnameForHashing = *config.RingHash.UseHostnameForHashing
			ringHash.ConsistentHashingLbConfig = &envoycommonv3.ConsistentHashingLbConfig{
				UseHostnameForHashing: *config.RingHash.UseHostnameForHashing,
			}
		}
		if config.LocalityType != nil {
			ringHash.LocalityWeightedLbConfig = &envoycommonv3.LocalityLbConfig_LocalityWeightedLbConfig{}
		}
		ringHashAny, err := utils.MessageToAny(ringHash)
		if err != nil {
			return nil, err
		}
		out.loadBalancingPolicy = &envoyclusterv3.LoadBalancingPolicy{
			Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
				TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
					Name:        "envoy.load_balancing_policies.ring_hash",
					TypedConfig: ringHashAny,
				},
			}},
		}
	} else if config.Maglev != nil {
		maglev := &envoymaglevv3.Maglev{}
		if config.Maglev.UseHostnameForHashing != nil {
			out.useHostnameForHashing = *config.Maglev.UseHostnameForHashing
			maglev.ConsistentHashingLbConfig = &envoycommonv3.ConsistentHashingLbConfig{
				UseHostnameForHashing: *config.Maglev.UseHostnameForHashing,
			}
		}
		if config.LocalityType != nil {
			maglev.LocalityWeightedLbConfig = &envoycommonv3.LocalityLbConfig_LocalityWeightedLbConfig{}
		}
		maglevAny, err := utils.MessageToAny(maglev)
		if err != nil {
			return nil, err
		}
		out.loadBalancingPolicy = &envoyclusterv3.LoadBalancingPolicy{
			Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
				TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
					Name:        "envoy.load_balancing_policies.maglev",
					TypedConfig: maglevAny,
				},
			}},
		}
	} else if config.Random != nil {
		random := &envoyrandomv3.Random{}
		if config.LocalityType != nil {
			random.LocalityLbConfig = &envoycommonv3.LocalityLbConfig{
				LocalityConfigSpecifier: &envoycommonv3.LocalityLbConfig_LocalityWeightedLbConfig_{
					LocalityWeightedLbConfig: &envoycommonv3.LocalityLbConfig_LocalityWeightedLbConfig{},
				},
			}
		}
		randomAny, err := utils.MessageToAny(random)
		if err != nil {
			return nil, err
		}
		out.loadBalancingPolicy = &envoyclusterv3.LoadBalancingPolicy{
			Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
				TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
					Name:        "envoy.load_balancing_policies.random",
					TypedConfig: randomAny,
				},
			}},
		}
	}

	return out, nil
}

func applyLoadBalancerConfig(config *LoadBalancerConfigIR, out *envoyclusterv3.Cluster) {
	if config == nil {
		return
	}

	if config.useHostnameForHashing && out.GetType() != envoyclusterv3.Cluster_STRICT_DNS {
		logger.Error("useHostnameForHashing is only supported for STRICT_DNS clusters. Ignoring useHostnameForHashing.", "cluster", out.GetName())
		if config.loadBalancingPolicy != nil && len(config.loadBalancingPolicy.Policies) > 0 {
			typedCfg := config.loadBalancingPolicy.Policies[0].GetTypedExtensionConfig()
			disableUseHostnameForHashingIfPresent(typedCfg)
		}
	}

	out.CommonLbConfig = config.commonLbConfig
	out.LoadBalancingPolicy = config.loadBalancingPolicy
}

// disableUseHostnameForHashingIfPresent ensures that if a load balancing policy
// contains a ConsistentHashingLbConfig with UseHostnameForHashing set, it is
// disabled and the typed config is re-packed. This is used when the cluster
// type does not support hostname hashing.
func disableUseHostnameForHashingIfPresent(typedCfg *envoycorev3.TypedExtensionConfig) {
	if typedCfg == nil || typedCfg.TypedConfig == nil {
		return
	}
	msg, err := utils.AnyToMessage(typedCfg.TypedConfig)
	if err != nil {
		logger.Error("failed to unpack typed extension config", "error", err)
		return
	}
	switch m := msg.(type) {
	case *envoyringhashv3.RingHash:
		if m.ConsistentHashingLbConfig != nil && m.ConsistentHashingLbConfig.UseHostnameForHashing {
			m.ConsistentHashingLbConfig.UseHostnameForHashing = false
			if anyMsg, err := utils.MessageToAny(m); err == nil {
				typedCfg.TypedConfig = anyMsg
			} else {
				logger.Error("failed to re-pack RingHash after mutating ConsistentHashingLbConfig", "error", err)
			}
		}
	case *envoymaglevv3.Maglev:
		if m.ConsistentHashingLbConfig != nil && m.ConsistentHashingLbConfig.UseHostnameForHashing {
			m.ConsistentHashingLbConfig.UseHostnameForHashing = false
			if anyMsg, err := utils.MessageToAny(m); err == nil {
				typedCfg.TypedConfig = anyMsg
			} else {
				logger.Error("failed to re-pack Maglev after mutating ConsistentHashingLbConfig", "error", err)
			}
		}
	}
}

func toSlowStartConfig(cfg *v1alpha1.SlowStart, name, namespace string) *envoycommonv3.SlowStartConfig {
	if cfg == nil {
		return nil
	}
	out := &envoycommonv3.SlowStartConfig{}
	if cfg.Window != nil {
		out.SlowStartWindow = durationpb.New(cfg.Window.Duration)
	}
	if cfg.MinWeightPercent != nil {
		out.MinWeightPercent = &typev3.Percent{
			Value: float64(*cfg.MinWeightPercent),
		}
	}
	if cfg.Aggression != nil {
		aggressionValue, err := strconv.ParseFloat(*cfg.Aggression, 64)
		if err != nil {
			// This should ideally not happen due to CRD validation
			logger.Error("error parsing slowStartConfig.aggression", "error", err, "policy", name, "namespace", namespace)
			return nil
		}
		// Envoy requires runtime key for RuntimeDouble types,
		// so use a policy-specific runtime key.
		// See https://github.com/kgateway-dev/kgateway/pull/9031
		runtimeKeyPrefix := fmt.Sprintf("%s.%s", name, namespace)

		out.Aggression = &envoycorev3.RuntimeDouble{
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

	if a.useHostnameForHashing != b.useHostnameForHashing {
		return false
	}
	if !proto.Equal(a.loadBalancingPolicy, b.loadBalancingPolicy) {
		return false
	}

	return true
}
