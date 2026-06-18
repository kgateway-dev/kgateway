package backendconfigpolicy

import (
	"fmt"
	"strconv"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoydnsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/clusters/dns/v3"
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

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

const (
	cookieAttributeSecure   = "Secure"
	cookieAttributeHttpOnly = "HttpOnly"
	cookieAttributeSameSite = "SameSite"
	cookieValueTrue         = "true"
	dnsClusterExtensionName = "envoy.clusters.dns"

	defaultZoneAwareForceLocalZoneMinSize uint32  = 1
	defaultZoneAwareMinClusterSize        uint64  = 6
	defaultZoneAwareRoutingEnabledPercent float64 = 100
)

type LoadBalancerConfigIR struct {
	commonLbConfig        *envoyclusterv3.Cluster_CommonLbConfig
	loadBalancingPolicy   *envoyclusterv3.LoadBalancingPolicy
	useHostnameForHashing bool
	hasZoneAware          bool
	zoneAwareForce        *ZoneAwareForceIR
}

// ZoneAwareForceIR stores configuration for forced zone-local routing.
type ZoneAwareForceIR struct {
	minEndpointsInZoneThreshold uint32
}

func translateLoadBalancerConfig(config *kgateway.LoadBalancer, policyName, policyNamespace string) (*LoadBalancerConfigIR, error) {
	out := &LoadBalancerConfigIR{
		commonLbConfig: &envoyclusterv3.Cluster_CommonLbConfig{},
	}
	if config == nil {
		return out, nil
	}

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

	if config.ZoneAware != nil && config.ZoneAware.PreferLocal != nil {
		out.hasZoneAware = true
		out.zoneAwareForce = zoneAwareForceIR(config.ZoneAware)
	}

	var err error
	switch {
	case config.LeastRequest != nil:
		out.loadBalancingPolicy, err = buildLeastRequestPolicy(config, policyName, policyNamespace)
	case config.RoundRobin != nil:
		out.loadBalancingPolicy, err = buildRoundRobinPolicy(config, policyName, policyNamespace)
	case config.RingHash != nil:
		out.loadBalancingPolicy, out.useHostnameForHashing, err = buildRingHashPolicy(config)
	case config.Maglev != nil:
		out.loadBalancingPolicy, out.useHostnameForHashing, err = buildMaglevPolicy(config)
	case config.Random != nil:
		out.loadBalancingPolicy, err = buildRandomPolicy(config)
	}

	if err != nil {
		return nil, err
	}

	return out, nil
}

func zoneAwareForceIR(zoneAware *kgateway.ZoneAwareLoadBalancer) *ZoneAwareForceIR {
	if zoneAware == nil || zoneAware.PreferLocal == nil || zoneAware.PreferLocal.Force == nil {
		return nil
	}

	threshold := defaultZoneAwareForceLocalZoneMinSize
	if zoneAware.PreferLocal.Force.MinEndpointsInZoneThreshold != nil {
		threshold = *zoneAware.PreferLocal.Force.MinEndpointsInZoneThreshold
	}
	return &ZoneAwareForceIR{minEndpointsInZoneThreshold: threshold}
}

func buildTypedLocalityLbConfig(config *kgateway.LoadBalancer) *envoycommonv3.LocalityLbConfig {
	if config == nil {
		return nil
	}
	if zoneAware := buildTypedZoneAwareLbConfig(config.ZoneAware); zoneAware != nil {
		return &envoycommonv3.LocalityLbConfig{
			LocalityConfigSpecifier: zoneAware,
		}
	}
	// Default to locality-weighted LB. A cluster with a typed load_balancing_policy
	// ignores common_lb_config.locality_config_specifier, and a typed policy without
	// locality_lb_config falls back to Envoy's implicit zone-aware defaults
	// (routing_enabled 100%, min_cluster_size 6) once the proxy fleet spans multiple
	// zones. Locality-weighted keeps traffic evenly distributed unless zoneAware is
	// explicitly configured. This also covers config.LocalityType, which maps to the
	// same locality-weighted config.
	return &envoycommonv3.LocalityLbConfig{
		LocalityConfigSpecifier: &envoycommonv3.LocalityLbConfig_LocalityWeightedLbConfig_{
			LocalityWeightedLbConfig: &envoycommonv3.LocalityLbConfig_LocalityWeightedLbConfig{},
		},
	}
}

func buildTypedZoneAwareLbConfig(zoneAware *kgateway.ZoneAwareLoadBalancer) *envoycommonv3.LocalityLbConfig_ZoneAwareLbConfig_ {
	if zoneAware == nil || zoneAware.PreferLocal == nil {
		return nil
	}

	preferLocal := zoneAware.PreferLocal
	minClusterSize := defaultZoneAwareMinClusterSize
	if preferLocal.MinEndpointsThreshold != nil {
		minClusterSize = *preferLocal.MinEndpointsThreshold
	}
	routingEnabled := defaultZoneAwareRoutingEnabledPercent
	if preferLocal.RoutingEnabled != nil {
		routingEnabled = float64(*preferLocal.RoutingEnabled)
	}

	zoneAwareConfig := &envoycommonv3.LocalityLbConfig_ZoneAwareLbConfig{
		RoutingEnabled: &typev3.Percent{Value: routingEnabled},
		MinClusterSize: &wrapperspb.UInt64Value{Value: minClusterSize},
	}
	if force := zoneAwareForceIR(zoneAware); force != nil {
		zoneAwareConfig.ForceLocalZone = &envoycommonv3.LocalityLbConfig_ZoneAwareLbConfig_ForceLocalZone{
			MinSize: &wrapperspb.UInt32Value{Value: force.minEndpointsInZoneThreshold},
		}
	}
	return &envoycommonv3.LocalityLbConfig_ZoneAwareLbConfig_{
		ZoneAwareLbConfig: zoneAwareConfig,
	}
}

func buildLeastRequestPolicy(config *kgateway.LoadBalancer, policyName, policyNamespace string) (*envoyclusterv3.LoadBalancingPolicy, error) {
	leastRequest := &envoyleastrequestv3.LeastRequest{
		ChoiceCount: &wrapperspb.UInt32Value{
			Value: uint32(config.LeastRequest.ChoiceCount), // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
		},
		SlowStartConfig: toSlowStartConfig(config.LeastRequest.SlowStart, policyName, policyNamespace),
	}
	if localityLbConfig := buildTypedLocalityLbConfig(config); localityLbConfig != nil {
		leastRequest.LocalityLbConfig = localityLbConfig
	}
	leastRequestAny, err := utils.MessageToAny(leastRequest)
	if err != nil {
		return nil, err
	}
	return &envoyclusterv3.LoadBalancingPolicy{
		Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
			TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
				Name:        "envoy.load_balancing_policies.least_request",
				TypedConfig: leastRequestAny,
			},
		}},
	}, nil
}

func buildRoundRobinPolicy(config *kgateway.LoadBalancer, policyName, policyNamespace string) (*envoyclusterv3.LoadBalancingPolicy, error) {
	roundRobin := &envoyroundrobinv3.RoundRobin{
		SlowStartConfig: toSlowStartConfig(config.RoundRobin.SlowStart, policyName, policyNamespace),
	}
	if localityLbConfig := buildTypedLocalityLbConfig(config); localityLbConfig != nil {
		roundRobin.LocalityLbConfig = localityLbConfig
	}
	roundRobinAny, err := utils.MessageToAny(roundRobin)
	if err != nil {
		return nil, err
	}
	return &envoyclusterv3.LoadBalancingPolicy{
		Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
			TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
				Name:        "envoy.load_balancing_policies.round_robin",
				TypedConfig: roundRobinAny,
			},
		}},
	}, nil
}

func buildRingHashPolicy(config *kgateway.LoadBalancer) (*envoyclusterv3.LoadBalancingPolicy, bool, error) {
	ringHash := &envoyringhashv3.RingHash{}
	useHostnameForHashing := false

	if config.RingHash.MinimumRingSize != nil {
		ringHash.MinimumRingSize = &wrapperspb.UInt64Value{
			Value: uint64(*config.RingHash.MinimumRingSize), // nolint:gosec // G115: kubebuilder validation ensures safe for uint64
		}
	}
	if config.RingHash.MaximumRingSize != nil {
		ringHash.MaximumRingSize = &wrapperspb.UInt64Value{
			Value: uint64(*config.RingHash.MaximumRingSize), // nolint:gosec // G115: kubebuilder validation ensures safe for uint64
		}
	}
	if config.RingHash.UseHostnameForHashing != nil || len(config.RingHash.HashPolicies) > 0 {
		hashingLBConfig := &envoycommonv3.ConsistentHashingLbConfig{}
		if config.RingHash.UseHostnameForHashing != nil {
			useHostnameForHashing = *config.RingHash.UseHostnameForHashing
			hashingLBConfig.UseHostnameForHashing = *config.RingHash.UseHostnameForHashing
		}
		hashingLBConfig.HashPolicy = constructHashPolicy(config.RingHash.HashPolicies)
		ringHash.ConsistentHashingLbConfig = hashingLBConfig
	}

	if config.LocalityType != nil {
		ringHash.LocalityWeightedLbConfig = &envoycommonv3.LocalityLbConfig_LocalityWeightedLbConfig{}
	}
	ringHashAny, err := utils.MessageToAny(ringHash)
	if err != nil {
		return nil, false, err
	}
	return &envoyclusterv3.LoadBalancingPolicy{
		Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
			TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
				Name:        "envoy.load_balancing_policies.ring_hash",
				TypedConfig: ringHashAny,
			},
		}},
	}, useHostnameForHashing, nil
}

func buildMaglevPolicy(config *kgateway.LoadBalancer) (*envoyclusterv3.LoadBalancingPolicy, bool, error) {
	maglev := &envoymaglevv3.Maglev{}
	useHostnameForHashing := false

	if config.Maglev.UseHostnameForHashing != nil || len(config.Maglev.HashPolicies) > 0 {
		hashingLBConfig := &envoycommonv3.ConsistentHashingLbConfig{}
		if config.Maglev.UseHostnameForHashing != nil {
			useHostnameForHashing = *config.Maglev.UseHostnameForHashing
			hashingLBConfig.UseHostnameForHashing = *config.Maglev.UseHostnameForHashing
		}
		hashingLBConfig.HashPolicy = constructHashPolicy(config.Maglev.HashPolicies)
		maglev.ConsistentHashingLbConfig = hashingLBConfig
	}
	if config.LocalityType != nil {
		maglev.LocalityWeightedLbConfig = &envoycommonv3.LocalityLbConfig_LocalityWeightedLbConfig{}
	}
	maglevAny, err := utils.MessageToAny(maglev)
	if err != nil {
		return nil, false, err
	}
	return &envoyclusterv3.LoadBalancingPolicy{
		Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
			TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
				Name:        "envoy.load_balancing_policies.maglev",
				TypedConfig: maglevAny,
			},
		}},
	}, useHostnameForHashing, nil
}

func buildRandomPolicy(config *kgateway.LoadBalancer) (*envoyclusterv3.LoadBalancingPolicy, error) {
	random := &envoyrandomv3.Random{}
	if localityLbConfig := buildTypedLocalityLbConfig(config); localityLbConfig != nil {
		random.LocalityLbConfig = localityLbConfig
	}
	randomAny, err := utils.MessageToAny(random)
	if err != nil {
		return nil, err
	}
	return &envoyclusterv3.LoadBalancingPolicy{
		Policies: []*envoyclusterv3.LoadBalancingPolicy_Policy{{
			TypedExtensionConfig: &envoycorev3.TypedExtensionConfig{
				Name:        "envoy.load_balancing_policies.random",
				TypedConfig: randomAny,
			},
		}},
	}, nil
}

func applyLoadBalancerConfig(config *LoadBalancerConfigIR, out *envoyclusterv3.Cluster) {
	if config == nil {
		return
	}

	if config.useHostnameForHashing && !supportsHostnameForHashing(out) {
		logger.Error("useHostnameForHashing is only supported for strict DNS clusters. Ignoring useHostnameForHashing.",
			"cluster", out.GetName())
		if config.loadBalancingPolicy != nil && len(config.loadBalancingPolicy.Policies) > 0 {
			typedCfg := config.loadBalancingPolicy.Policies[0].GetTypedExtensionConfig()
			disableUseHostnameForHashingIfPresent(typedCfg)
		}
	}

	out.CommonLbConfig = config.commonLbConfig
	out.LoadBalancingPolicy = config.loadBalancingPolicy
}

func supportsHostnameForHashing(cluster *envoyclusterv3.Cluster) bool {
	switch cdt := cluster.GetClusterDiscoveryType().(type) {
	case *envoyclusterv3.Cluster_Type:
		return cdt.Type == envoyclusterv3.Cluster_STRICT_DNS
	case *envoyclusterv3.Cluster_ClusterType:
		if cdt.ClusterType.GetName() != dnsClusterExtensionName || cdt.ClusterType.GetTypedConfig() == nil {
			return false
		}
		msg, err := utils.AnyToMessage(cdt.ClusterType.GetTypedConfig())
		if err != nil {
			logger.Error("failed to unpack dns cluster config", "cluster", cluster.GetName(), "error", err)
			return false
		}
		dnsCluster, ok := msg.(*envoydnsv3.DnsCluster)
		if !ok {
			return false
		}
		return !dnsCluster.GetAllAddressesInSingleEndpoint()
	default:
		return false
	}
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

func toSlowStartConfig(cfg *kgateway.SlowStart, name, namespace string) *envoycommonv3.SlowStartConfig {
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
	if a.hasZoneAware != b.hasZoneAware {
		return false
	}
	if !proto.Equal(a.loadBalancingPolicy, b.loadBalancingPolicy) {
		return false
	}
	if !a.zoneAwareForce.Equals(b.zoneAwareForce) {
		return false
	}

	return true
}

func (a *ZoneAwareForceIR) Equals(b *ZoneAwareForceIR) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.minEndpointsInZoneThreshold == b.minEndpointsInZoneThreshold
}

// constructHashPolicy constructs the hash policies from the policy specification.
func constructHashPolicy(hashPolicies []kgateway.HashPolicy) []*envoyroutev3.RouteAction_HashPolicy {
	if len(hashPolicies) == 0 {
		return nil
	}
	policies := make([]*envoyroutev3.RouteAction_HashPolicy, 0, len(hashPolicies))
	for _, hashPolicy := range hashPolicies {
		policy := &envoyroutev3.RouteAction_HashPolicy{}
		if hashPolicy.Terminal != nil {
			policy.Terminal = *hashPolicy.Terminal
		}
		switch {
		case hashPolicy.Header != nil:
			policy.PolicySpecifier = &envoyroutev3.RouteAction_HashPolicy_Header_{
				Header: &envoyroutev3.RouteAction_HashPolicy_Header{
					HeaderName: hashPolicy.Header.Name,
				},
			}
		case hashPolicy.Cookie != nil:
			policy.PolicySpecifier = &envoyroutev3.RouteAction_HashPolicy_Cookie_{
				Cookie: &envoyroutev3.RouteAction_HashPolicy_Cookie{
					Name: hashPolicy.Cookie.Name,
				},
			}
			if hashPolicy.Cookie.TTL != nil {
				policy.GetCookie().Ttl = durationpb.New(hashPolicy.Cookie.TTL.Duration)
			}
			if hashPolicy.Cookie.Path != nil {
				policy.GetCookie().Path = *hashPolicy.Cookie.Path
			}

			attributes := make([]*envoyroutev3.RouteAction_HashPolicy_CookieAttribute, 0, 3)
			if hashPolicy.Cookie.Secure != nil && *hashPolicy.Cookie.Secure {
				attributes = append(attributes, &envoyroutev3.RouteAction_HashPolicy_CookieAttribute{
					Name:  cookieAttributeSecure,
					Value: cookieValueTrue,
				})
			}
			if hashPolicy.Cookie.HttpOnly != nil && *hashPolicy.Cookie.HttpOnly {
				attributes = append(attributes, &envoyroutev3.RouteAction_HashPolicy_CookieAttribute{
					Name:  cookieAttributeHttpOnly,
					Value: cookieValueTrue,
				})
			}
			if hashPolicy.Cookie.SameSite != nil {
				attributes = append(attributes, &envoyroutev3.RouteAction_HashPolicy_CookieAttribute{
					Name:  cookieAttributeSameSite,
					Value: *hashPolicy.Cookie.SameSite,
				})
			}
			if len(attributes) > 0 {
				policy.GetCookie().Attributes = attributes
			}
		case hashPolicy.SourceIP != nil:
			policy.PolicySpecifier = &envoyroutev3.RouteAction_HashPolicy_ConnectionProperties_{
				ConnectionProperties: &envoyroutev3.RouteAction_HashPolicy_ConnectionProperties{
					SourceIp: true,
				},
			}
		}
		policies = append(policies, policy)
	}
	return policies
}
