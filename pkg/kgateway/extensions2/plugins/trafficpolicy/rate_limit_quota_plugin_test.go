package trafficpolicy

import (
	"testing"
	"time"

	cncfmatcherv3 "github.com/cncf/xds/go/xds/type/matcher/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	rlqsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rate_limit_quota/v3"
	envoymatcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoytypev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func testRateLimitQuotaProvider(name string) *TrafficPolicyGatewayExtensionIR {
	return &TrafficPolicyGatewayExtensionIR{
		Name: name,
		RateLimitQuota: &rlqsv3.RateLimitQuotaFilterConfig{
			RlqsServer: &envoycorev3.GrpcService{
				TargetSpecifier: &envoycorev3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &envoycorev3.GrpcService_EnvoyGrpc{ClusterName: "rlqs_cluster"},
				},
			},
			Domain: "test-domain",
		},
	}
}

func TestBuildRateLimitQuotaBucketSettings(t *testing.T) {
	deny := kgateway.RateLimitQuotaFallbackDeny
	settings, err := buildRateLimitQuotaBucketSettings(&kgateway.RateLimitQuotaPolicy{
		Bucket: []kgateway.RateLimitQuotaBucketEntry{
			{Key: "service", Type: kgateway.RateLimitQuotaBucketEntryTypeGeneric, Value: new("payments")},
			{Key: "tenant", Type: kgateway.RateLimitQuotaBucketEntryTypeHeader, Header: new("x-tenant-id")},
		},
		ReportingInterval:    &metav1.Duration{Duration: 2 * time.Second},
		NoAssignmentBehavior: &deny,
		DenyStatus:           new(uint32(503)),
		ExpiredAssignmentBehavior: &kgateway.RateLimitQuotaExpiredAssignmentBehavior{
			Strategy: kgateway.RateLimitQuotaExpiredAssignmentStrategyReuseLast,
			Timeout:  metav1.Duration{Duration: 30 * time.Second},
		},
	})
	require.NoError(t, err)
	require.NoError(t, settings.ValidateAll())
	assert.Equal(t, 2*time.Second, settings.GetReportingInterval().AsDuration())
	assert.Equal(t, envoytypev3.StatusCode_ServiceUnavailable, settings.GetDenyResponseSettings().GetHttpStatus().GetCode())
	assert.Equal(t, envoytypev3.RateLimitStrategy_DENY_ALL,
		settings.GetNoAssignmentBehavior().GetFallbackRateLimit().GetBlanketRule())
	assert.Equal(t, 30*time.Second,
		settings.GetExpiredAssignmentBehavior().GetExpiredAssignmentBehaviorTimeout().AsDuration())
	assert.NotNil(t, settings.GetExpiredAssignmentBehavior().GetReuseLastAssignment())

	builders := settings.GetBucketIdBuilder().GetBucketIdBuilder()
	assert.Equal(t, "payments", builders["service"].GetStringValue())
	headerInput := &envoymatcherv3.HttpRequestHeaderMatchInput{}
	require.NoError(t, builders["tenant"].GetCustomValue().GetTypedConfig().UnmarshalTo(headerInput))
	assert.Equal(t, "x-tenant-id", headerInput.GetHeaderName())
}

func TestBuildRateLimitQuotaBucketSettingsDefaults(t *testing.T) {
	settings, err := buildRateLimitQuotaBucketSettings(&kgateway.RateLimitQuotaPolicy{
		Bucket: []kgateway.RateLimitQuotaBucketEntry{{
			Key: "service", Type: kgateway.RateLimitQuotaBucketEntryTypeGeneric, Value: new("api"),
		}},
	})
	require.NoError(t, err)
	assert.Equal(t, defaultRateLimitQuotaReportingInterval, settings.GetReportingInterval().AsDuration())
	assert.Nil(t, settings.NoAssignmentBehavior)
	assert.Nil(t, settings.ExpiredAssignmentBehavior)
	assert.Nil(t, settings.DenyResponseSettings)
}

func TestBuildRateLimitQuotaBucketSettingsRejectsMalformedEntries(t *testing.T) {
	tests := []struct {
		name  string
		entry kgateway.RateLimitQuotaBucketEntry
		want  string
	}{
		{
			name:  "generic without value",
			entry: kgateway.RateLimitQuotaBucketEntry{Key: "service", Type: kgateway.RateLimitQuotaBucketEntryTypeGeneric},
			want:  "requires value",
		},
		{
			name:  "header without name",
			entry: kgateway.RateLimitQuotaBucketEntry{Key: "tenant", Type: kgateway.RateLimitQuotaBucketEntryTypeHeader},
			want:  "requires header",
		},
		{
			name:  "unknown type",
			entry: kgateway.RateLimitQuotaBucketEntry{Key: "bad", Type: "Metadata"},
			want:  "unsupported bucket entry type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildRateLimitQuotaBucketSettings(&kgateway.RateLimitQuotaPolicy{
				Bucket: []kgateway.RateLimitQuotaBucketEntry{tt.entry},
			})
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestExpiredAssignmentFallbackStrategies(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   kgateway.RateLimitQuotaExpiredAssignmentStrategy
		want envoytypev3.RateLimitStrategy_BlanketRule
	}{
		{name: "allow", in: kgateway.RateLimitQuotaExpiredAssignmentStrategyAllow, want: envoytypev3.RateLimitStrategy_ALLOW_ALL},
		{name: "deny", in: kgateway.RateLimitQuotaExpiredAssignmentStrategyDeny, want: envoytypev3.RateLimitStrategy_DENY_ALL},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out := expiredAssignmentBehavior(&kgateway.RateLimitQuotaExpiredAssignmentBehavior{
				Strategy: tt.in,
				Timeout:  metav1.Duration{Duration: time.Minute},
			})
			assert.Equal(t, tt.want, out.GetFallbackRateLimit().GetBlanketRule())
		})
	}
}

func TestRateLimitQuotaRouteNamePredicate(t *testing.T) {
	predicate, err := rateLimitQuotaRouteNamePredicate("route-a")
	require.NoError(t, err)
	require.NoError(t, predicate.ValidateAll())
	matcher := &cncfmatcherv3.CelMatcher{}
	require.NoError(t, predicate.GetSinglePredicate().GetCustomMatch().GetTypedConfig().UnmarshalTo(matcher))
	call := matcher.GetExprMatch().GetCelExprParsed().GetExpr().GetCallExpr()
	require.Len(t, call.GetArgs(), 2)
	assert.Equal(t, "_==_", call.GetFunction())
	assert.Equal(t, "xds", call.GetArgs()[0].GetSelectExpr().GetOperand().GetIdentExpr().GetName())
	assert.Equal(t, "route_name", call.GetArgs()[0].GetSelectExpr().GetField())
	assert.Equal(t, "route-a", call.GetArgs()[1].GetConstExpr().GetStringValue())
}

func TestRateLimitQuotaBucketsFirstPolicyClaimsRoute(t *testing.T) {
	first, err := buildRateLimitQuotaBucketSettings(&kgateway.RateLimitQuotaPolicy{Bucket: []kgateway.RateLimitQuotaBucketEntry{{
		Key: "policy", Type: kgateway.RateLimitQuotaBucketEntryTypeGeneric, Value: new("route"),
	}}})
	require.NoError(t, err)
	second, err := buildRateLimitQuotaBucketSettings(&kgateway.RateLimitQuotaPolicy{Bucket: []kgateway.RateLimitQuotaBucketEntry{{
		Key: "policy", Type: kgateway.RateLimitQuotaBucketEntryTypeGeneric, Value: new("gateway"),
	}}})
	require.NoError(t, err)

	buckets := &rateLimitQuotaBuckets{}
	require.NoError(t, buckets.add("route-a", first))
	require.NoError(t, buckets.add("route-a", second))
	require.NoError(t, buckets.add("route-b", second))
	require.Len(t, buckets.matchers, 2)

	got := &rlqsv3.RateLimitQuotaBucketSettings{}
	require.NoError(t, buckets.matchers[0].GetOnMatch().GetAction().GetTypedConfig().UnmarshalTo(got))
	assert.Equal(t, "route", got.GetBucketIdBuilder().GetBucketIdBuilder()["policy"].GetStringValue())
}

func TestRateLimitQuotaHTTPFiltersGroupsByProviderWithoutMutation(t *testing.T) {
	provider := testRateLimitQuotaProvider("default/rlqs")
	original := proto.Clone(provider.RateLimitQuota)
	settings, err := buildRateLimitQuotaBucketSettings(&kgateway.RateLimitQuotaPolicy{Bucket: []kgateway.RateLimitQuotaBucketEntry{{
		Key: "service", Type: kgateway.RateLimitQuotaBucketEntryTypeGeneric, Value: new("api"),
	}}})
	require.NoError(t, err)

	pass := &trafficPolicyPluginGwPass{}
	quota := &rateLimitQuotaIR{provider: provider, bucketSettings: settings}
	pass.handleRateLimitQuota("http", quota, rateLimitQuotaPolicyScope{routeNames: []string{"route-a"}})
	pass.handleRateLimitQuota("http", quota, rateLimitQuotaPolicyScope{routeNames: []string{"route-b"}})

	filters := pass.rateLimitQuotaHTTPFilters("http")
	require.Len(t, filters, 1)
	assert.Equal(t, "ratelimit/quota/default/rlqs", filters[0].Filter.GetName())

	config := &rlqsv3.RateLimitQuotaFilterConfig{}
	require.NoError(t, filters[0].Filter.GetTypedConfig().UnmarshalTo(config))
	require.NoError(t, config.ValidateAll())
	assert.Equal(t, "test-domain", config.GetDomain())
	assert.Len(t, config.GetBucketMatchers().GetMatcherList().GetMatchers(), 2)
	assert.True(t, proto.Equal(original, provider.RateLimitQuota), "assembling the listener filter must not mutate shared provider IR")
}

func TestRateLimitQuotaBackendScopeDoesNotBroadenPolicy(t *testing.T) {
	pass := &trafficPolicyPluginGwPass{}
	pass.handleRateLimitQuota("http", &rateLimitQuotaIR{
		provider:       testRateLimitQuotaProvider("default/rlqs"),
		bucketSettings: &rlqsv3.RateLimitQuotaBucketSettings{},
	}, rateLimitQuotaPolicyScope{})
	assert.Empty(t, pass.rateLimitQuotaPerProvider.Providers)
	assert.Empty(t, pass.rateLimitQuotaBuckets)
}

func TestRateLimitQuotaIREquals(t *testing.T) {
	provider := testRateLimitQuotaProvider("default/rlqs")
	settings := &rlqsv3.RateLimitQuotaBucketSettings{}
	base := &rateLimitQuotaIR{provider: provider, bucketSettings: settings}
	assert.True(t, base.Equals(&rateLimitQuotaIR{
		provider: provider, bucketSettings: proto.Clone(settings).(*rlqsv3.RateLimitQuotaBucketSettings),
	}))
	assert.False(t, base.Equals(&rateLimitQuotaIR{
		provider: testRateLimitQuotaProvider("default/other"), bucketSettings: settings,
	}))
	assert.False(t, base.Equals(&rateLimitQuotaIR{
		provider: provider,
		bucketSettings: &rlqsv3.RateLimitQuotaBucketSettings{
			ReportingInterval: durationpb.New(time.Second),
		},
	}))
	assert.False(t, base.Equals(nil))
}
