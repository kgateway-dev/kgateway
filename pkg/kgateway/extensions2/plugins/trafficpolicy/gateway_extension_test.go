package trafficpolicy

import (
	"slices"
	"testing"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoymatchingv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/matching/v3"
	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
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

func TestBuildExtAuthMetadataContextNamespaces(t *testing.T) {
	empty := []string{}
	configuredNamespaces := []string{"dev.example.a", "dev.example.b"}
	withJWTDuplicate := []string{JWTAuthnMetadataNamespace, "dev.example.a"}
	withUserDuplicate := []string{"dev.example.a", "dev.example.a"}

	tests := []struct {
		name       string
		configured *[]string
		want       []string
	}{
		{
			name: "nil config preserves legacy behavior",
			want: nil,
		},
		{
			name:       "empty config only includes the required JWT namespace",
			configured: &empty,
			want:       []string{JWTAuthnMetadataNamespace},
		},
		{
			name:       "user namespaces are kept in order before the required namespace",
			configured: &configuredNamespaces,
			want:       []string{"dev.example.a", "dev.example.b", JWTAuthnMetadataNamespace},
		},
		{
			name:       "required namespace is not duplicated",
			configured: &withJWTDuplicate,
			want:       []string{JWTAuthnMetadataNamespace, "dev.example.a"},
		},
		{
			name:       "duplicate user namespaces are collapsed",
			configured: &withUserDuplicate,
			want:       []string{"dev.example.a", JWTAuthnMetadataNamespace},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var original []string
			if tt.configured != nil {
				original = slices.Clone(*tt.configured)
			}
			got := buildExtAuthMetadataContextNamespaces(tt.configured)
			assert.Equal(t, tt.want, got)
			if tt.configured != nil {
				assert.Equal(t, original, *tt.configured, "input slice must not be mutated")
			}
		})
	}
}

func TestExtAuthFilterStage(t *testing.T) {
	tests := []struct {
		name    string
		extAuth *envoy_ext_authz_v3.ExtAuthz
		want    filters.HTTPFilterStage
	}{
		{
			name: "grpc without metadata keeps historical AuthN placement",
			extAuth: &envoy_ext_authz_v3.ExtAuthz{
				Services: &envoy_ext_authz_v3.ExtAuthz_GrpcService{
					GrpcService: &envoycorev3.GrpcService{},
				},
			},
			want: filters.DuringStage(filters.AuthNStage),
		},
		{
			name: "grpc with metadata runs after AuthN",
			extAuth: &envoy_ext_authz_v3.ExtAuthz{
				Services: &envoy_ext_authz_v3.ExtAuthz_GrpcService{
					GrpcService: &envoycorev3.GrpcService{},
				},
				MetadataContextNamespaces: []string{JWTAuthnMetadataNamespace},
			},
			want: filters.AfterStage(filters.AuthNStage),
		},
		{
			name: "route cache clearing keeps legacy placement as defense in depth",
			extAuth: &envoy_ext_authz_v3.ExtAuthz{
				Services: &envoy_ext_authz_v3.ExtAuthz_GrpcService{
					GrpcService: &envoycorev3.GrpcService{},
				},
				MetadataContextNamespaces: []string{JWTAuthnMetadataNamespace},
				ClearRouteCache:           true,
			},
			want: filters.DuringStage(filters.AuthNStage),
		},
		{
			name: "http keeps its original AuthN placement",
			extAuth: &envoy_ext_authz_v3.ExtAuthz{
				Services: &envoy_ext_authz_v3.ExtAuthz_HttpService{
					HttpService: &envoy_ext_authz_v3.HttpService{},
				},
			},
			want: filters.DuringStage(filters.AuthNStage),
		},
		{
			name:    "missing service keeps the original AuthN placement",
			extAuth: &envoy_ext_authz_v3.ExtAuthz{},
			want:    filters.DuringStage(filters.AuthNStage),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extAuthFilterStage(tt.extAuth))
		})
	}
}

func TestGrpcExtAuthWithMetadataIsOrderedAfterJWT(t *testing.T) {
	grpcExtAuth := &envoy_ext_authz_v3.ExtAuthz{
		Services: &envoy_ext_authz_v3.ExtAuthz_GrpcService{
			GrpcService: &envoycorev3.GrpcService{},
		},
		MetadataContextNamespaces: []string{JWTAuthnMetadataNamespace},
	}
	extAuth := filters.MustNewStagedFilter(
		extAuthFilterName("ext-auth"),
		grpcExtAuth,
		extAuthFilterStage(grpcExtAuth),
	)
	jwt := filters.MustNewStagedFilter(
		jwtFilterName("jwt-ext"),
		&envoymatchingv3.ExtensionWithMatcher{},
		filters.DuringStage(filters.AuthNStage),
	)
	list := filters.StagedHttpFilterList{jwt, extAuth}
	list.Sort()

	require.Len(t, list, 2)
	assert.Equal(t, jwtFilterName("jwt-ext"), list[0].Filter.GetName())
	assert.Equal(t, extAuthFilterName("ext-auth"), list[1].Filter.GetName())
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
