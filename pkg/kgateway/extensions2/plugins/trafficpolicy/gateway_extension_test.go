package trafficpolicy

import (
	"testing"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	apifake "github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	k8splugin "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/plugins/kubernetes"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

func TestBuildStringListMatcher(t *testing.T) {
	tests := []struct {
		name      string
		headers   []string
		wantNil   bool
		wantExact []string
	}{
		{
			name:    "nil input returns nil",
			headers: nil,
			wantNil: true,
		},
		{
			name:    "empty slice returns nil",
			headers: []string{},
			wantNil: true,
		},
		{
			name:      "single header",
			headers:   []string{"location"},
			wantExact: []string{"location"},
		},
		{
			name:      "multiple headers produce exact matchers in order",
			headers:   []string{"location", "set-cookie", "www-authenticate"},
			wantExact: []string{"location", "set-cookie", "www-authenticate"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildStringListMatcher(tt.headers)
			if tt.wantNil {
				assert.Nil(t, result)
				return
			}
			require.NotNil(t, result)
			require.Len(t, result.Patterns, len(tt.wantExact))
			for i, want := range tt.wantExact {
				assert.Equal(t, want, result.Patterns[i].GetExact())
			}
		})
	}
}

func TestBuildRateLimitFilterShadowMode(t *testing.T) {
	int32Ptr := func(v int32) *int32 { return &v }

	tests := []struct {
		name              string
		percentEnabled    *int32
		percentEnforced   *int32
		wantEnabledUnset  bool
		wantEnabledNum    uint32
		wantEnforcedUnset bool
		wantEnforcedNum   uint32
	}{
		{
			name:              "unset leaves both nil so Envoy falls back to its own default runtime keys",
			percentEnabled:    nil,
			percentEnforced:   nil,
			wantEnabledUnset:  true,
			wantEnforcedUnset: true,
		},
		{
			name:            "shadow mode: fully enabled, not enforced",
			percentEnabled:  int32Ptr(100),
			percentEnforced: int32Ptr(0),
			wantEnabledNum:  100,
			wantEnforcedNum: 0,
		},
		{
			name:            "partial shadow: fully enabled, partially enforced",
			percentEnabled:  int32Ptr(100),
			percentEnforced: int32Ptr(25),
			wantEnabledNum:  100,
			wantEnforcedNum: 25,
		},
		{
			name:            "filter disabled entirely",
			percentEnabled:  int32Ptr(0),
			percentEnforced: int32Ptr(0),
			wantEnabledNum:  0,
			wantEnforcedNum: 0,
		},
		{
			name:              "only enabled set, enforced left to Envoy's default",
			percentEnabled:    int32Ptr(100),
			percentEnforced:   nil,
			wantEnabledNum:    100,
			wantEnforcedUnset: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &kgateway.RateLimitProvider{
				Domain:          "api-gateway",
				FailOpen:        true,
				Timeout:         metav1.Duration{Duration: 100 * time.Millisecond},
				PercentEnabled:  tt.percentEnabled,
				PercentEnforced: tt.percentEnforced,
			}

			got := buildRateLimitFilter("default/full-ratelimit", &envoycorev3.GrpcService{}, provider)

			if tt.wantEnabledUnset {
				assert.Nil(t, got.FilterEnabled,
					"nil PercentEnabled must leave FilterEnabled unset so Envoy consults its own global runtime key default")
			} else {
				require.NotNil(t, got.FilterEnabled)
				assert.Equal(t, "ratelimit.default.full-ratelimit.filter_enabled", got.FilterEnabled.RuntimeKey)
				assert.Equal(t, tt.wantEnabledNum, got.FilterEnabled.DefaultValue.Numerator)
				assert.Equal(t, typev3.FractionalPercent_HUNDRED, got.FilterEnabled.DefaultValue.Denominator)
			}

			if tt.wantEnforcedUnset {
				assert.Nil(t, got.FilterEnforced,
					"nil PercentEnforced must leave FilterEnforced unset so Envoy consults its own global runtime key default")
			} else {
				require.NotNil(t, got.FilterEnforced)
				assert.Equal(t, "ratelimit.default.full-ratelimit.filter_enforced", got.FilterEnforced.RuntimeKey)
				assert.Equal(t, tt.wantEnforcedNum, got.FilterEnforced.DefaultValue.Numerator)
				assert.Equal(t, typev3.FractionalPercent_HUNDRED, got.FilterEnforced.DefaultValue.Denominator)
			}
		})
	}
}

func TestBuildRateLimitFilterRuntimeKeysAreScopedByProviderNotDomain(t *testing.T) {
	int32Ptr := func(v int32) *int32 { return &v }

	// Two distinct GatewayExtensions may legitimately share the same Domain (kgateway does not
	// enforce Domain uniqueness). The runtime key must still be per-provider, otherwise an
	// operator overriding one provider's key at runtime would silently affect the other.
	providerA := &kgateway.RateLimitProvider{
		Domain:          "api-gateway",
		Timeout:         metav1.Duration{Duration: 100 * time.Millisecond},
		PercentEnforced: int32Ptr(25),
	}
	providerB := &kgateway.RateLimitProvider{
		Domain:          "api-gateway",
		Timeout:         metav1.Duration{Duration: 100 * time.Millisecond},
		PercentEnforced: int32Ptr(75),
	}

	filterA := buildRateLimitFilter("default/provider-a", &envoycorev3.GrpcService{}, providerA)
	filterB := buildRateLimitFilter("default/provider-b", &envoycorev3.GrpcService{}, providerB)

	assert.NotEqual(t, filterA.FilterEnforced.RuntimeKey, filterB.FilterEnforced.RuntimeKey,
		"same-domain providers must still get distinct runtime keys so one provider's override can't clobber another's")
}

// TestGatewayExtensionExtAuthEmitsFilterStateStats is the regression test for
// https://github.com/kgateway-dev/kgateway/issues/14790. Envoy only populates
// ExtAuthzLoggingInfo (e.g. latency_us for access logs) when emit_filter_state_stats
// is set, so gatewayExtensionBuilder must set it on both the gRPC and HTTP paths.
// It drives the real KRT collection, failing if the flag is dropped from either branch,
// and pins the filter name that doubles as the %FILTER_STATE() key.
func TestGatewayExtensionExtAuthEmitsFilterStateStats(t *testing.T) {
	ctx := t.Context()

	backendRef := gwv1.BackendRef{
		BackendObjectReference: gwv1.BackendObjectReference{
			Kind: new(gwv1.Kind("Service")),
			Name: "ext-authz",
			Port: new(gwv1.PortNumber(9000)),
		},
	}
	grpcExt := &kgateway.GatewayExtension{
		ObjectMeta: metav1.ObjectMeta{Name: "grpc-extauth", Namespace: "default"},
		Spec: kgateway.GatewayExtensionSpec{
			Type: new(kgateway.GatewayExtensionTypeExtAuth),
			ExtAuth: &kgateway.ExtAuthProvider{
				GrpcService:   &kgateway.ExtGrpcService{BackendRef: backendRef},
				StatusOnError: 403,
			},
		},
	}
	httpExt := &kgateway.GatewayExtension{
		ObjectMeta: metav1.ObjectMeta{Name: "http-extauth", Namespace: "default"},
		Spec: kgateway.GatewayExtensionSpec{
			Type: new(kgateway.GatewayExtensionTypeExtAuth),
			ExtAuth: &kgateway.ExtAuthProvider{
				HttpService:   &kgateway.ExtHttpService{BackendRef: backendRef},
				StatusOnError: 403,
			},
		},
	}

	fakeClient := apifake.NewClient(t, grpcExt, httpExt,
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "ext-authz", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Port: 9000, TargetPort: intstr.FromInt32(9000)}},
			},
		},
	)

	settings := apisettings.Settings{}
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)
	commoncol, err := collections.NewCommonCollections(
		ctx, krtopts, fakeClient, wellknown.DefaultGatewayControllerName, settings,
	)
	require.NoError(t, err)
	// The kubernetes plugin contributes Service-backed Backends, which the
	// extauth backendRefs above resolve against.
	commoncol.InitPlugins(ctx, k8splugin.NewPlugin(ctx, commoncol), settings)

	// The OIDC discoverer is unused on the extauth path; it is never started.
	discoverer := newOIDCProviderConfigDiscoverer(func() []string { return nil })
	extensions := krt.NewCollection(commoncol.GatewayExtensions, gatewayExtensionBuilder(ctx, commoncol, discoverer))

	fakeClient.RunAndWait(ctx.Done())
	require.Eventually(t, func() bool {
		return commoncol.HasSynced() && commoncol.BackendIndex.HasSynced() && extensions.HasSynced()
	}, 10*time.Second, 50*time.Millisecond, "collections should sync")

	for _, name := range []string{"grpc-extauth", "http-extauth"} {
		key := krt.Named{Name: name, Namespace: "default"}.ResourceName()
		var got *TrafficPolicyGatewayExtensionIR
		// EventuallyWithT reports the last translation error rather than only a timeout,
		// so a consistent failure shows its cause.
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			got = extensions.GetKey(key)
			require.NotNil(c, got, "extension %q should be translated", key)
			require.NoError(c, got.Err, "extension %q should translate without error", key)
		}, 10*time.Second, 50*time.Millisecond, "extension %q should translate without error", key)
		require.NotNil(t, got.ExtAuth, "extension %q should produce an ExtAuthz filter", key)
		require.True(t, got.ExtAuth.GetEmitFilterStateStats(),
			"extension %q must set emit_filter_state_stats so ext-auth latency can be access-logged", key)
		require.Equal(t, "ext_auth/"+key, extAuthFilterName(providerName(got)),
			"filter name pins the %FILTER_STATE() key for extension %q", key)
	}
}
