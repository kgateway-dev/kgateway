package trafficpolicy

import (
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
)

func TestConstructRouteTracing_Nil(t *testing.T) {
	out := &trafficPolicySpecIr{}
	constructRouteTracing(kgateway.TrafficPolicySpec{}, out)
	assert.Nil(t, out.tracing, "tracing IR should be nil when spec.Tracing is nil")
}

func TestConstructRouteTracing_Disable(t *testing.T) {
	out := &trafficPolicySpecIr{}
	constructRouteTracing(kgateway.TrafficPolicySpec{
		Tracing: &kgateway.RouteTracing{
			Disable: &shared.PolicyDisable{},
		},
	}, out)

	require.NotNil(t, out.tracing)
	require.NotNil(t, out.tracing.tracing)

	tr := out.tracing.tracing
	assert.Equal(t, uint32(0), tr.GetClientSampling().GetNumerator())
	assert.Equal(t, uint32(0), tr.GetRandomSampling().GetNumerator())
	assert.Equal(t, uint32(0), tr.GetOverallSampling().GetNumerator())
	assert.Equal(t, typev3.FractionalPercent_HUNDRED, tr.GetClientSampling().GetDenominator())
	assert.Equal(t, typev3.FractionalPercent_HUNDRED, tr.GetRandomSampling().GetDenominator())
	assert.Equal(t, typev3.FractionalPercent_HUNDRED, tr.GetOverallSampling().GetDenominator())
	assert.Empty(t, tr.GetCustomTags())
}

func TestConstructRouteTracing_SamplingRates(t *testing.T) {
	out := &trafficPolicySpecIr{}
	constructRouteTracing(kgateway.TrafficPolicySpec{
		Tracing: &kgateway.RouteTracing{
			ClientSampling:  ptr.To[int32](50),
			RandomSampling:  ptr.To[int32](10),
			OverallSampling: ptr.To[int32](100),
		},
	}, out)

	require.NotNil(t, out.tracing)
	tr := out.tracing.tracing

	assert.Equal(t, uint32(50), tr.GetClientSampling().GetNumerator())
	assert.Equal(t, typev3.FractionalPercent_HUNDRED, tr.GetClientSampling().GetDenominator())

	assert.Equal(t, uint32(10), tr.GetRandomSampling().GetNumerator())
	assert.Equal(t, typev3.FractionalPercent_HUNDRED, tr.GetRandomSampling().GetDenominator())

	assert.Equal(t, uint32(100), tr.GetOverallSampling().GetNumerator())
	assert.Equal(t, typev3.FractionalPercent_HUNDRED, tr.GetOverallSampling().GetDenominator())
}

func TestConstructRouteTracing_PartialSampling(t *testing.T) {
	out := &trafficPolicySpecIr{}
	constructRouteTracing(kgateway.TrafficPolicySpec{
		Tracing: &kgateway.RouteTracing{
			RandomSampling: ptr.To[int32](5),
		},
	}, out)

	require.NotNil(t, out.tracing)
	tr := out.tracing.tracing

	assert.Nil(t, tr.ClientSampling, "unset sampling should be nil")
	assert.Equal(t, uint32(5), tr.GetRandomSampling().GetNumerator())
	assert.Nil(t, tr.OverallSampling, "unset sampling should be nil")
}

func TestConstructRouteTracing_Attributes(t *testing.T) {
	out := &trafficPolicySpecIr{}
	constructRouteTracing(kgateway.TrafficPolicySpec{
		Tracing: &kgateway.RouteTracing{
			Attributes: []kgateway.CustomAttribute{
				{
					Name: "route.id",
					Literal: &kgateway.CustomAttributeLiteral{
						Value: "my-route",
					},
				},
				{
					Name: "request.id",
					RequestHeader: &kgateway.CustomAttributeHeader{
						Name:         "x-request-id",
						DefaultValue: new("Request"),
					},
				},
				{
					Name: "env.region",
					Environment: &kgateway.CustomAttributeEnvironment{
						Name: "AWS_REGION",
					},
				},
			},
		},
	}, out)

	require.NotNil(t, out.tracing)
	tr := out.tracing.tracing

	require.Len(t, tr.GetCustomTags(), 3)

	// Literal tag
	assert.Equal(t, "route.id", tr.GetCustomTags()[0].GetTag())
	assert.Equal(t, "my-route", tr.GetCustomTags()[0].GetLiteral().GetValue())

	// Request header tag
	assert.Equal(t, "request.id", tr.GetCustomTags()[1].GetTag())
	assert.Equal(t, "x-request-id", tr.GetCustomTags()[1].GetRequestHeader().GetName())
	assert.Equal(t, "Request", tr.GetCustomTags()[1].GetRequestHeader().GetDefaultValue())

	// Environment tag
	assert.Equal(t, "env.region", tr.GetCustomTags()[2].GetTag())
	assert.Equal(t, "AWS_REGION", tr.GetCustomTags()[2].GetEnvironment().GetName())
}

func TestConstructRouteTracing_SamplingWithAttributes(t *testing.T) {
	out := &trafficPolicySpecIr{}
	constructRouteTracing(kgateway.TrafficPolicySpec{
		Tracing: &kgateway.RouteTracing{
			RandomSampling: ptr.To[int32](25),
			Attributes: []kgateway.CustomAttribute{
				{
					Name: "tag1",
					Literal: &kgateway.CustomAttributeLiteral{
						Value: "value1",
					},
				},
			},
		},
	}, out)

	require.NotNil(t, out.tracing)
	tr := out.tracing.tracing

	assert.Equal(t, uint32(25), tr.GetRandomSampling().GetNumerator())
	require.Len(t, tr.GetCustomTags(), 1)
	assert.Equal(t, "tag1", tr.GetCustomTags()[0].GetTag())
}

func TestRouteTracingIR_Equals(t *testing.T) {
	tests := []struct {
		name     string
		a        *routeTracingIR
		b        PolicySubIR
		expected bool
	}{
		{
			name:     "both nil",
			a:        nil,
			b:        (*routeTracingIR)(nil),
			expected: true,
		},
		{
			name: "a nil, b not nil",
			a:    nil,
			b: &routeTracingIR{
				tracing: &envoyroutev3.Tracing{},
			},
			expected: false,
		},
		{
			name: "a not nil, b nil",
			a: &routeTracingIR{
				tracing: &envoyroutev3.Tracing{},
			},
			b:        (*routeTracingIR)(nil),
			expected: false,
		},
		{
			name: "equal sampling",
			a: &routeTracingIR{
				tracing: &envoyroutev3.Tracing{
					RandomSampling: &typev3.FractionalPercent{
						Numerator:   10,
						Denominator: typev3.FractionalPercent_HUNDRED,
					},
				},
			},
			b: &routeTracingIR{
				tracing: &envoyroutev3.Tracing{
					RandomSampling: &typev3.FractionalPercent{
						Numerator:   10,
						Denominator: typev3.FractionalPercent_HUNDRED,
					},
				},
			},
			expected: true,
		},
		{
			name: "different sampling",
			a: &routeTracingIR{
				tracing: &envoyroutev3.Tracing{
					RandomSampling: &typev3.FractionalPercent{
						Numerator:   10,
						Denominator: typev3.FractionalPercent_HUNDRED,
					},
				},
			},
			b: &routeTracingIR{
				tracing: &envoyroutev3.Tracing{
					RandomSampling: &typev3.FractionalPercent{
						Numerator:   50,
						Denominator: typev3.FractionalPercent_HUNDRED,
					},
				},
			},
			expected: false,
		},
		{
			name:     "wrong type",
			a:        &routeTracingIR{},
			b:        &csrfIR{},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.a.Equals(tc.b))
		})
	}
}

func TestRouteTracingIR_Validate(t *testing.T) {
	t.Run("nil IR", func(t *testing.T) {
		var ir *routeTracingIR
		assert.NoError(t, ir.Validate())
	})

	t.Run("nil tracing", func(t *testing.T) {
		ir := &routeTracingIR{}
		assert.NoError(t, ir.Validate())
	})

	t.Run("valid tracing", func(t *testing.T) {
		ir := &routeTracingIR{
			tracing: &envoyroutev3.Tracing{
				RandomSampling: &typev3.FractionalPercent{
					Numerator:   50,
					Denominator: typev3.FractionalPercent_HUNDRED,
				},
			},
		}
		assert.NoError(t, ir.Validate())
	})
}

func TestHandleRouteTracing(t *testing.T) {
	t.Run("nil tracing does not set Route.Tracing", func(t *testing.T) {
		p := &trafficPolicyPluginGwPass{}
		route := &envoyroutev3.Route{}
		spec := trafficPolicySpecIr{}

		p.handleRouteTracing(spec, route)
		assert.Nil(t, route.Tracing)
	})

	t.Run("tracing sets Route.Tracing", func(t *testing.T) {
		p := &trafficPolicyPluginGwPass{}
		route := &envoyroutev3.Route{}
		spec := trafficPolicySpecIr{
			tracing: &routeTracingIR{
				tracing: &envoyroutev3.Tracing{
					RandomSampling: &typev3.FractionalPercent{
						Numerator:   10,
						Denominator: typev3.FractionalPercent_HUNDRED,
					},
				},
			},
		}

		p.handleRouteTracing(spec, route)
		require.NotNil(t, route.Tracing)
		assert.Equal(t, uint32(10), route.Tracing.GetRandomSampling().GetNumerator())
	})
}
