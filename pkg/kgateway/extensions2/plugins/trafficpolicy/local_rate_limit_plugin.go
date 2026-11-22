package trafficpolicy

import (
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/ratelimit/v3"
	localratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	localRatelimitFilterEnabledRuntimeKey  = "local_rate_limit_enabled"
	localRatelimitFilterEnforcedRuntimeKey = "local_rate_limit_enforced"
	localRatelimitFilterDisabledRuntimeKey = "local_rate_limit_disabled"
)

type localRateLimitIR struct {
	// config is ready to use wherever no single route is known (Gateway, listener,
	// virtual host, route backend): its stat prefix is already resolved, with the
	// route tokens of a template left empty.
	config *localratelimitv3.LocalRateLimit
	// statPrefixTemplate holds the stat prefix when it has template tokens, so
	// forRoute can resolve it for each route in ApplyForRoute.
	statPrefixTemplate string
}

var _ PolicySubIR = &localRateLimitIR{}

func (l *localRateLimitIR) Equals(other PolicySubIR) bool {
	otherLocalRateLimit, ok := other.(*localRateLimitIR)
	if !ok {
		return false
	}
	if l == nil && otherLocalRateLimit == nil {
		return true
	}
	if l == nil || otherLocalRateLimit == nil {
		return false
	}
	return l.statPrefixTemplate == otherLocalRateLimit.statPrefixTemplate &&
		proto.Equal(l.config, otherLocalRateLimit.config)
}

func (l *localRateLimitIR) Validate() error {
	if l == nil || l.config == nil {
		return nil
	}
	return l.config.ValidateAll()
}

// constructLocalRateLimit constructs the local rate limit policy IR from the policy specification.
func constructLocalRateLimit(in *kgateway.TrafficPolicy, out *trafficPolicySpecIr) {
	if in.Spec.RateLimit == nil || in.Spec.RateLimit.Local == nil {
		return
	}
	local := in.Spec.RateLimit.Local
	localRateLimit := toLocalRateLimitFilterConfig(local)
	out.localRateLimit = &localRateLimitIR{
		config: localRateLimit,
	}

	// The stat prefix is worked out once, here. The config gets the prefix with the
	// route tokens empty, which is what every level without a single route uses. A
	// template is also kept so ApplyForRoute can resolve it per route.
	// The CRD requires tokenBucket with statPrefix, so this is never the disabled config.
	if sp := local.StatPrefix; sp != nil {
		localRateLimit.StatPrefix = localRateLimitStatPrefixOrDefault(resolveStatPrefixTemplate(*sp, "", "", ""))
		if statPrefixTemplateVar.MatchString(*sp) {
			out.localRateLimit.statPrefixTemplate = *sp
		}
	}
}

func toLocalRateLimitFilterConfig(t *kgateway.LocalRateLimitPolicy) *localratelimitv3.LocalRateLimit {
	if t == nil {
		return nil
	}

	// If the local rate limit policy is empty, we add a LocalRateLimit configuration that disables
	// any other applied local rate limit policy (if any) for the target.
	if *t == (kgateway.LocalRateLimitPolicy{}) {
		return createDisabledRateLimit()
	}

	tokenBucket := &typev3.TokenBucket{}
	if t.TokenBucket != nil {
		tokenBucket.FillInterval = durationpb.New(t.TokenBucket.FillInterval.Duration)
		tokenBucket.MaxTokens = uint32(t.TokenBucket.MaxTokens) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
		if t.TokenBucket.TokensPerFill != nil {
			tokenBucket.TokensPerFill = wrapperspb.UInt32(uint32(*t.TokenBucket.TokensPerFill)) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
		}
	}

	filterEnabled := uint32(100)
	if t.PercentEnabled != nil {
		filterEnabled = uint32(*t.PercentEnabled) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}

	filterEnforced := uint32(100)
	if t.PercentEnforced != nil {
		filterEnforced = uint32(*t.PercentEnforced) // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
	}

	var lrl *localratelimitv3.LocalRateLimit = &localratelimitv3.LocalRateLimit{
		StatPrefix:  localRateLimitStatPrefix,
		TokenBucket: tokenBucket,
		// By default filter is enabled for 0% of the requests.
		FilterEnabled: &envoycorev3.RuntimeFractionalPercent{
			RuntimeKey: localRatelimitFilterEnabledRuntimeKey,
			DefaultValue: &typev3.FractionalPercent{
				Numerator:   filterEnabled,
				Denominator: typev3.FractionalPercent_HUNDRED,
			},
		},
		// By default filter is enforced for 0% of the requests (out of the enabled fraction).
		FilterEnforced: &envoycorev3.RuntimeFractionalPercent{
			RuntimeKey: localRatelimitFilterEnforcedRuntimeKey,
			DefaultValue: &typev3.FractionalPercent{
				Numerator:   filterEnforced,
				Denominator: typev3.FractionalPercent_HUNDRED,
			},
		},
	}

	if ptr.Deref(t.ShareAcrossGateway, false) {
		// Divides the token bucket evenly across the members of the proxy's local cluster,
		// which kgateway populates with the Gateway's own replicas.
		lrl.LocalClusterRateLimit = &ratelimitv3.LocalClusterRateLimit{}
	}

	return lrl
}

// createDisabledRateLimit returns a LocalRateLimit configuration that disables rate limiting.
// This is used when an empty policy is provided to override any existing rate limit configuration.
func createDisabledRateLimit() *localratelimitv3.LocalRateLimit {
	return &localratelimitv3.LocalRateLimit{
		StatPrefix: localRateLimitStatPrefix,
		// Config per route requires a token bucket, so we create a minimal one
		TokenBucket: &typev3.TokenBucket{
			MaxTokens:    1,
			FillInterval: durationpb.New(1),
		},
		// Set filter enabled to 0% to effectively disable rate limiting
		FilterEnabled: &envoycorev3.RuntimeFractionalPercent{
			RuntimeKey:   localRatelimitFilterDisabledRuntimeKey,
			DefaultValue: &typev3.FractionalPercent{},
		},
	}
}

// localRateLimitStatPrefixOrDefault returns prefix, or the default local rate limit stat
// prefix when prefix is empty.
func localRateLimitStatPrefixOrDefault(prefix string) string {
	if prefix == "" {
		return localRateLimitStatPrefix
	}
	return prefix
}

// forRoute returns the IR to use for one route. If the stat prefix is a template, it
// returns a copy with the prefix resolved for that route. The receiver is shared by all
// routes the policy applies to, so it is never changed.
func (l *localRateLimitIR) forRoute(routeName, routeNamespace, ruleName string) *localRateLimitIR {
	if l == nil || l.config == nil || l.statPrefixTemplate == "" {
		return l
	}
	config := proto.Clone(l.config).(*localratelimitv3.LocalRateLimit)
	config.StatPrefix = localRateLimitStatPrefixOrDefault(
		resolveStatPrefixTemplate(l.statPrefixTemplate, routeName, routeNamespace, ruleName))
	return &localRateLimitIR{config: config}
}

func (p *trafficPolicyPluginGwPass) handleLocalRateLimit(fcn string, typedFilterConfig *ir.TypedFilterConfigMap, localRateLimit *localRateLimitIR) {
	if localRateLimit == nil {
		return
	}
	// The config is used as built: ApplyForRoute has resolved the stat prefix for its
	// route, and at the other levels the IR already holds the prefix with the route
	// tokens empty.
	typedFilterConfig.AddTypedConfig(localRateLimitFilterNamePrefix, localRateLimit.config)

	// Add a filter to the chain. When having a rate limit for a route we need to also have a
	// globally disabled rate limit filter in the chain otherwise it will be ignored.
	// If there is also rate limit for the listener, it will not override this one.
	if p.localRateLimitInChain == nil {
		p.localRateLimitInChain = make(map[string]*localratelimitv3.LocalRateLimit)
	}
	if _, ok := p.localRateLimitInChain[fcn]; !ok {
		p.localRateLimitInChain[fcn] = &localratelimitv3.LocalRateLimit{
			StatPrefix: localRateLimitStatPrefix,
		}
	}
}
