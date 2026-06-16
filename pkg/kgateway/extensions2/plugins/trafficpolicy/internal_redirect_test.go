package trafficpolicy

import (
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
)

func TestInternalRedirectIREquals(t *testing.T) {
	tests := []struct {
		name string
		a, b *kgateway.InternalRedirect
		want bool
	}{
		{
			name: "both nil are equal",
			want: true,
		},
		{
			name: "same values are equal",
			a: &kgateway.InternalRedirect{
				RedirectResponseCodes:    []kgateway.InternalRedirectResponseCode{303},
				AllowCrossSchemeRedirect: new(true),
				MaxRedirects:             new(uint32(2)),
			},
			b: &kgateway.InternalRedirect{
				RedirectResponseCodes:    []kgateway.InternalRedirectResponseCode{303},
				AllowCrossSchemeRedirect: new(true),
				MaxRedirects:             new(uint32(2)),
			},
			want: true,
		},
		{
			name: "different response codes",
			a: &kgateway.InternalRedirect{
				RedirectResponseCodes: []kgateway.InternalRedirectResponseCode{302},
			},
			b: &kgateway.InternalRedirect{
				RedirectResponseCodes: []kgateway.InternalRedirectResponseCode{303},
			},
			want: false,
		},
		{
			name: "different cross scheme",
			a: &kgateway.InternalRedirect{
				AllowCrossSchemeRedirect: new(true),
			},
			b: &kgateway.InternalRedirect{
				AllowCrossSchemeRedirect: new(false),
			},
			want: false,
		},
		{
			name: "different max redirects",
			a: &kgateway.InternalRedirect{
				MaxRedirects: new(uint32(1)),
			},
			b: &kgateway.InternalRedirect{
				MaxRedirects: new(uint32(3)),
			},
			want: false,
		},
		{
			name: "nil vs non-nil",
			a:    nil,
			b: &kgateway.InternalRedirect{
				RedirectResponseCodes: []kgateway.InternalRedirectResponseCode{302},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)

			aOut := &trafficPolicySpecIr{}
			constructInternalRedirect(kgateway.TrafficPolicySpec{
				InternalRedirect: tt.a,
			}, aOut)

			bOut := &trafficPolicySpecIr{}
			constructInternalRedirect(kgateway.TrafficPolicySpec{
				InternalRedirect: tt.b,
			}, bOut)

			a.Equal(tt.want, aOut.internalRedirect.Equals(bOut.internalRedirect))
			// Symmetry
			a.Equal(tt.want, bOut.internalRedirect.Equals(aOut.internalRedirect), "Equals should be symmetric")
		})
	}
}

func TestConstructInternalRedirect(t *testing.T) {
	t.Run("nil spec produces nil IR", func(t *testing.T) {
		out := &trafficPolicySpecIr{}
		constructInternalRedirect(kgateway.TrafficPolicySpec{}, out)
		assert.Nil(t, out.internalRedirect)
	})

	t.Run("defaults", func(t *testing.T) {
		out := &trafficPolicySpecIr{}
		constructInternalRedirect(kgateway.TrafficPolicySpec{
			InternalRedirect: &kgateway.InternalRedirect{},
		}, out)
		require.NotNil(t, out.internalRedirect)
		policy := out.internalRedirect.policy
		assert.Empty(t, policy.RedirectResponseCodes)
		assert.False(t, policy.AllowCrossSchemeRedirect)
		assert.Empty(t, policy.ResponseHeadersToCopy)
		assert.Nil(t, policy.MaxInternalRedirects)
	})

	t.Run("full config", func(t *testing.T) {
		out := &trafficPolicySpecIr{}
		constructInternalRedirect(kgateway.TrafficPolicySpec{
			InternalRedirect: &kgateway.InternalRedirect{
				RedirectResponseCodes:    []kgateway.InternalRedirectResponseCode{301, 303, 307},
				AllowCrossSchemeRedirect: new(true),
				ResponseHeadersToCopy:    []gwv1.HTTPHeaderName{"Authorization", "X-Request-Id"},
				MaxRedirects:             new(uint32(5)),
			},
		}, out)
		require.NotNil(t, out.internalRedirect)
		policy := out.internalRedirect.policy
		assert.Equal(t, []uint32{301, 303, 307}, policy.RedirectResponseCodes)
		assert.True(t, policy.AllowCrossSchemeRedirect)
		assert.Equal(t, []string{"Authorization", "X-Request-Id"}, policy.ResponseHeadersToCopy)
		assert.Equal(t, wrapperspb.UInt32(5), policy.MaxInternalRedirects)
	})
}

func TestInternalRedirectApply(t *testing.T) {
	t.Run("sets policy on route action", func(t *testing.T) {
		ir := &trafficPolicySpecIr{}
		constructInternalRedirect(kgateway.TrafficPolicySpec{
			InternalRedirect: &kgateway.InternalRedirect{
				RedirectResponseCodes: []kgateway.InternalRedirectResponseCode{303},
			},
		}, ir)

		route := &envoyroutev3.Route{
			Action: &envoyroutev3.Route_Route{
				Route: &envoyroutev3.RouteAction{},
			},
		}

		pass := &trafficPolicyPluginGwPass{}
		pass.handlePerRoutePolicies(*ir, route)

		require.NotNil(t, route.GetRoute().InternalRedirectPolicy)
		assert.Equal(t, []uint32{303}, route.GetRoute().InternalRedirectPolicy.RedirectResponseCodes)
	})

	t.Run("does not overwrite existing policy", func(t *testing.T) {
		ir := &trafficPolicySpecIr{}
		constructInternalRedirect(kgateway.TrafficPolicySpec{
			InternalRedirect: &kgateway.InternalRedirect{
				RedirectResponseCodes: []kgateway.InternalRedirectResponseCode{301},
			},
		}, ir)

		existing := &envoyroutev3.InternalRedirectPolicy{
			RedirectResponseCodes: []uint32{303},
		}
		route := &envoyroutev3.Route{
			Action: &envoyroutev3.Route_Route{
				Route: &envoyroutev3.RouteAction{
					InternalRedirectPolicy: existing,
				},
			},
		}

		pass := &trafficPolicyPluginGwPass{}
		pass.handlePerRoutePolicies(*ir, route)

		assert.Same(t, existing, route.GetRoute().InternalRedirectPolicy,
			"should preserve the existing policy, not overwrite it")
	})

	t.Run("skipped on redirect action", func(t *testing.T) {
		ir := &trafficPolicySpecIr{}
		constructInternalRedirect(kgateway.TrafficPolicySpec{
			InternalRedirect: &kgateway.InternalRedirect{
				RedirectResponseCodes: []kgateway.InternalRedirectResponseCode{303},
			},
		}, ir)

		route := &envoyroutev3.Route{
			Action: &envoyroutev3.Route_Redirect{
				Redirect: &envoyroutev3.RedirectAction{},
			},
		}

		pass := &trafficPolicyPluginGwPass{}
		pass.handlePerRoutePolicies(*ir, route)
		assert.Nil(t, route.GetRoute())
	})
}
