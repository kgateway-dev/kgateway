package trafficpolicy

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	cncfcorev3 "github.com/cncf/xds/go/xds/core/v3"
	cncfmatcherv3 "github.com/cncf/xds/go/xds/type/matcher/v3"
	cncftypev3 "github.com/cncf/xds/go/xds/type/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	rlqsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rate_limit_quota/v3"
	envoymatcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoytypev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/google/cel-go/cel"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	sharedv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmputils"
)

const (
	rateLimitQuotaFilterNamePrefix = "ratelimit/quota"
	rateLimitQuotaBucketActionName = "rate_limit_quota"

	defaultRateLimitQuotaReportingInterval = 5 * time.Second
)

// rateLimitQuotaIR keeps the policy translation close to Envoy's API. The
// route-selection predicate is added once the concrete Envoy routes are known.
type rateLimitQuotaIR struct {
	provider       *TrafficPolicyGatewayExtensionIR
	bucketSettings *rlqsv3.RateLimitQuotaBucketSettings
}

var _ PolicySubIR = &rateLimitQuotaIR{}

func (r *rateLimitQuotaIR) Equals(other PolicySubIR) bool {
	otherQuota, ok := other.(*rateLimitQuotaIR)
	if !ok {
		return false
	}
	if r == nil || otherQuota == nil {
		return r == nil && otherQuota == nil
	}
	if !proto.Equal(r.bucketSettings, otherQuota.bucketSettings) {
		return false
	}
	return cmputils.CompareWithNils(r.provider, otherQuota.provider, func(a, b *TrafficPolicyGatewayExtensionIR) bool {
		return a.Equals(*b)
	})
}

func (r *rateLimitQuotaIR) Validate() error {
	if r == nil {
		return nil
	}
	if r.bucketSettings != nil {
		if err := r.bucketSettings.ValidateAll(); err != nil {
			return err
		}
	}
	if r.provider != nil {
		return r.provider.Validate()
	}
	return nil
}

func constructRateLimitQuota(
	krtctx krt.HandlerContext,
	in *kgateway.TrafficPolicy,
	fetchGatewayExtension FetchGatewayExtensionFunc,
	out *trafficPolicySpecIr,
) error {
	if in.Spec.RateLimit == nil || in.Spec.RateLimit.Quota == nil {
		return nil
	}
	quota := in.Spec.RateLimit.Quota

	provider, err := fetchGatewayExtension(krtctx, quota.ExtensionRef, in.GetNamespace())
	if err != nil {
		return fmt.Errorf("rate limit quota: %w", err)
	}
	if provider.RateLimitQuota == nil {
		return pluginutils.ErrInvalidExtensionType(kgateway.GatewayExtensionTypeRateLimitQuota)
	}

	bucketSettings, err := buildRateLimitQuotaBucketSettings(quota)
	if err != nil {
		return fmt.Errorf("rate limit quota: %w", err)
	}
	out.rateLimitQuota = &rateLimitQuotaIR{
		provider:       provider,
		bucketSettings: bucketSettings,
	}
	return nil
}

func buildRateLimitQuotaBucketSettings(quota *kgateway.RateLimitQuotaPolicy) (*rlqsv3.RateLimitQuotaBucketSettings, error) {
	reportingInterval := defaultRateLimitQuotaReportingInterval
	if quota.ReportingInterval != nil {
		reportingInterval = quota.ReportingInterval.Duration
	}

	bucketID := make(map[string]*rlqsv3.RateLimitQuotaBucketSettings_BucketIdBuilder_ValueBuilder, len(quota.Bucket))
	for _, entry := range quota.Bucket {
		switch entry.Type {
		case kgateway.RateLimitQuotaBucketEntryTypeGeneric:
			if entry.Value == nil {
				return nil, fmt.Errorf("bucket entry %q of type Generic requires value", entry.Key)
			}
			bucketID[entry.Key] = &rlqsv3.RateLimitQuotaBucketSettings_BucketIdBuilder_ValueBuilder{
				ValueSpecifier: &rlqsv3.RateLimitQuotaBucketSettings_BucketIdBuilder_ValueBuilder_StringValue{
					StringValue: *entry.Value,
				},
			}
		case kgateway.RateLimitQuotaBucketEntryTypeHeader:
			if entry.Header == nil {
				return nil, fmt.Errorf("bucket entry %q of type Header requires header", entry.Key)
			}
			bucketID[entry.Key] = &rlqsv3.RateLimitQuotaBucketSettings_BucketIdBuilder_ValueBuilder{
				ValueSpecifier: &rlqsv3.RateLimitQuotaBucketSettings_BucketIdBuilder_ValueBuilder_CustomValue{
					CustomValue: &envoycorev3.TypedExtensionConfig{
						Name:        "request-header",
						TypedConfig: utils.MustMessageToAny(&envoymatcherv3.HttpRequestHeaderMatchInput{HeaderName: *entry.Header}),
					},
				},
			}
		default:
			return nil, fmt.Errorf("unsupported bucket entry type %q", entry.Type)
		}
	}

	settings := &rlqsv3.RateLimitQuotaBucketSettings{
		BucketIdBuilder:   &rlqsv3.RateLimitQuotaBucketSettings_BucketIdBuilder{BucketIdBuilder: bucketID},
		ReportingInterval: durationpb.New(reportingInterval),
	}
	if quota.DenyStatus != nil {
		settings.DenyResponseSettings = &rlqsv3.RateLimitQuotaBucketSettings_DenyResponseSettings{
			HttpStatus: &envoytypev3.HttpStatus{Code: envoytypev3.StatusCode(*quota.DenyStatus)}, //nolint:gosec // CRD validation bounds the value to [400,599].
		}
	}
	if quota.NoAssignmentBehavior != nil {
		settings.NoAssignmentBehavior = noAssignmentBehavior(*quota.NoAssignmentBehavior)
	}
	if quota.ExpiredAssignmentBehavior != nil {
		settings.ExpiredAssignmentBehavior = expiredAssignmentBehavior(quota.ExpiredAssignmentBehavior)
	}
	return settings, nil
}

func blanketRule(strategy kgateway.RateLimitQuotaFallback) envoytypev3.RateLimitStrategy_BlanketRule {
	if strategy == kgateway.RateLimitQuotaFallbackDeny {
		return envoytypev3.RateLimitStrategy_DENY_ALL
	}
	return envoytypev3.RateLimitStrategy_ALLOW_ALL
}

func fallbackStrategy(strategy kgateway.RateLimitQuotaFallback) *envoytypev3.RateLimitStrategy {
	return &envoytypev3.RateLimitStrategy{
		Strategy: &envoytypev3.RateLimitStrategy_BlanketRule_{BlanketRule: blanketRule(strategy)},
	}
}

func noAssignmentBehavior(strategy kgateway.RateLimitQuotaFallback) *rlqsv3.RateLimitQuotaBucketSettings_NoAssignmentBehavior {
	return &rlqsv3.RateLimitQuotaBucketSettings_NoAssignmentBehavior{
		NoAssignmentBehavior: &rlqsv3.RateLimitQuotaBucketSettings_NoAssignmentBehavior_FallbackRateLimit{
			FallbackRateLimit: fallbackStrategy(strategy),
		},
	}
}

func expiredAssignmentBehavior(in *kgateway.RateLimitQuotaExpiredAssignmentBehavior) *rlqsv3.RateLimitQuotaBucketSettings_ExpiredAssignmentBehavior {
	out := &rlqsv3.RateLimitQuotaBucketSettings_ExpiredAssignmentBehavior{
		ExpiredAssignmentBehaviorTimeout: durationpb.New(in.Timeout.Duration),
	}
	switch in.Strategy {
	case kgateway.RateLimitQuotaExpiredAssignmentStrategyReuseLast:
		out.ExpiredAssignmentBehavior = &rlqsv3.RateLimitQuotaBucketSettings_ExpiredAssignmentBehavior_ReuseLastAssignment_{
			ReuseLastAssignment: &rlqsv3.RateLimitQuotaBucketSettings_ExpiredAssignmentBehavior_ReuseLastAssignment{},
		}
	case kgateway.RateLimitQuotaExpiredAssignmentStrategyDeny:
		out.ExpiredAssignmentBehavior = &rlqsv3.RateLimitQuotaBucketSettings_ExpiredAssignmentBehavior_FallbackRateLimit{
			FallbackRateLimit: fallbackStrategy(kgateway.RateLimitQuotaFallbackDeny),
		}
	default:
		out.ExpiredAssignmentBehavior = &rlqsv3.RateLimitQuotaBucketSettings_ExpiredAssignmentBehavior_FallbackRateLimit{
			FallbackRateLimit: fallbackStrategy(kgateway.RateLimitQuotaFallbackAllow),
		}
	}
	return out
}

func buildRateLimitQuotaFilter(grpcService *envoycorev3.GrpcService, provider *kgateway.RateLimitQuotaProvider) *rlqsv3.RateLimitQuotaFilterConfig {
	return &rlqsv3.RateLimitQuotaFilterConfig{
		RlqsServer: grpcService,
		Domain:     provider.Domain,
	}
}

func getRateLimitQuotaFilterName(name string) string {
	if name == "" {
		return rateLimitQuotaFilterNamePrefix
	}
	return fmt.Sprintf("%s/%s", rateLimitQuotaFilterNamePrefix, name)
}

// rateLimitQuotaPolicyScope identifies the concrete Envoy routes covered by a
// policy attachment and used to build the listener-level bucket matcher.
type rateLimitQuotaPolicyScope struct {
	routeNames []string
}

func rateLimitQuotaRouteScope(route *envoyroutev3.Route) rateLimitQuotaPolicyScope {
	return rateLimitQuotaPolicyScope{routeNames: []string{route.GetName()}}
}

func rateLimitQuotaVhostScope(vhost *envoyroutev3.VirtualHost) rateLimitQuotaPolicyScope {
	names := make([]string, 0, len(vhost.GetRoutes()))
	for _, route := range vhost.GetRoutes() {
		names = append(names, route.GetName())
	}
	return rateLimitQuotaPolicyScope{routeNames: names}
}

func rateLimitQuotaRouteConfigScope(routeConfig *envoyroutev3.RouteConfiguration) rateLimitQuotaPolicyScope {
	var names []string
	for _, vhost := range routeConfig.GetVirtualHosts() {
		for _, route := range vhost.GetRoutes() {
			names = append(names, route.GetName())
		}
	}
	return rateLimitQuotaPolicyScope{routeNames: names}
}

type rateLimitQuotaBuckets struct {
	matchers []*cncfmatcherv3.Matcher_MatcherList_FieldMatcher
	claimed  map[string]struct{}
}

func (b *rateLimitQuotaBuckets) add(routeName string, settings *rlqsv3.RateLimitQuotaBucketSettings) error {
	if routeName == "" {
		return errors.New("cannot scope rate limit quota policy to an unnamed Envoy route")
	}
	if b.claimed == nil {
		b.claimed = make(map[string]struct{})
	}
	if _, ok := b.claimed[routeName]; ok {
		return nil
	}
	predicate, err := rateLimitQuotaRouteNamePredicate(routeName)
	if err != nil {
		return err
	}
	b.claimed[routeName] = struct{}{}
	b.matchers = append(b.matchers, &cncfmatcherv3.Matcher_MatcherList_FieldMatcher{
		Predicate: predicate,
		OnMatch: &cncfmatcherv3.Matcher_OnMatch{
			OnMatch: &cncfmatcherv3.Matcher_OnMatch_Action{
				Action: &cncfcorev3.TypedExtensionConfig{
					Name:        rateLimitQuotaBucketActionName,
					TypedConfig: utils.MustMessageToAny(settings),
				},
			},
		},
	})
	return nil
}

func (p *trafficPolicyPluginGwPass) handleRateLimitQuota(fcn string, quota *rateLimitQuotaIR, scope rateLimitQuotaPolicyScope) {
	if quota == nil || quota.provider == nil || quota.bucketSettings == nil {
		return
	}
	if len(scope.routeNames) == 0 {
		logger.Warn("skipping rate limit quota policy without route scope")
		return
	}

	providerName := quota.provider.ResourceName()
	p.rateLimitQuotaPerProvider.Add(fcn, providerName, quota.provider, filters.DuringStage(filters.RateLimitStage))
	if p.rateLimitQuotaBuckets == nil {
		p.rateLimitQuotaBuckets = make(map[string]map[string]*rateLimitQuotaBuckets)
	}
	if p.rateLimitQuotaBuckets[fcn] == nil {
		p.rateLimitQuotaBuckets[fcn] = make(map[string]*rateLimitQuotaBuckets)
	}
	buckets := p.rateLimitQuotaBuckets[fcn][providerName]
	if buckets == nil {
		buckets = &rateLimitQuotaBuckets{claimed: make(map[string]struct{})}
		p.rateLimitQuotaBuckets[fcn][providerName] = buckets
	}
	for _, routeName := range scope.routeNames {
		if err := buckets.add(routeName, quota.bucketSettings); err != nil {
			logger.Error("failed to build rate limit quota route matcher", "route", routeName, "error", err)
		}
	}
}

var rateLimitQuotaCELEnv = sync.OnceValues(func() (*cel.Env, error) { return cel.NewEnv() })

func rateLimitQuotaRouteNamePredicate(routeName string) (*cncfmatcherv3.Matcher_MatcherList_Predicate, error) {
	env, err := rateLimitQuotaCELEnv()
	if err != nil {
		return nil, fmt.Errorf("create CEL environment: %w", err)
	}
	parsed, err := parseCELExpression(env, sharedv1alpha1.CELExpression(
		"xds.route_name == "+strconv.Quote(routeName)))
	if err != nil {
		return nil, fmt.Errorf("parse route-name expression: %w", err)
	}
	parsed.SourceInfo = nil

	return &cncfmatcherv3.Matcher_MatcherList_Predicate{
		MatchType: &cncfmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_{
			SinglePredicate: &cncfmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate{
				Input: &cncfcorev3.TypedExtensionConfig{
					Name:        "envoy.matching.inputs.cel_data_input",
					TypedConfig: utils.MustMessageToAny(&cncfmatcherv3.HttpAttributesCelMatchInput{}),
				},
				Matcher: &cncfmatcherv3.Matcher_MatcherList_Predicate_SinglePredicate_CustomMatch{
					CustomMatch: &cncfcorev3.TypedExtensionConfig{
						Name: "envoy.matching.matchers.cel_matcher",
						TypedConfig: utils.MustMessageToAny(&cncfmatcherv3.CelMatcher{
							ExprMatch: &cncftypev3.CelExpression{CelExprParsed: parsed},
						}),
					},
				},
			},
		},
	}, nil
}

func (p *trafficPolicyPluginGwPass) rateLimitQuotaHTTPFilters(fcn string) []filters.StagedHttpFilter {
	var out []filters.StagedHttpFilter
	emitted := make(map[string]struct{})
	for _, provider := range p.rateLimitQuotaPerProvider.Providers[fcn] {
		if _, ok := emitted[provider.Name]; ok || provider.Extension.RateLimitQuota == nil {
			continue
		}
		buckets := p.rateLimitQuotaBuckets[fcn][provider.Name]
		if buckets == nil || len(buckets.matchers) == 0 {
			continue
		}
		emitted[provider.Name] = struct{}{}

		config := proto.Clone(provider.Extension.RateLimitQuota).(*rlqsv3.RateLimitQuotaFilterConfig)
		config.BucketMatchers = &cncfmatcherv3.Matcher{
			MatcherType: &cncfmatcherv3.Matcher_MatcherList_{
				MatcherList: &cncfmatcherv3.Matcher_MatcherList{Matchers: buckets.matchers},
			},
		}
		out = append(out, filters.MustNewStagedFilter(
			getRateLimitQuotaFilterName(provider.Name),
			config,
			provider.FilterStage,
		))
	}
	return out
}
