package trafficpolicy

import (
	"testing"

	grpcwebv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/grpc_web/v3"
	envoy_wellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestGRPCWebIREquals(t *testing.T) {
	enabled := &grpcWebIR{perRoute: EnableFilterPerRoute()}
	disabled := &grpcWebIR{perRoute: DisableFilterPerRouteOptional()}

	tests := []struct {
		name     string
		a, b     *grpcWebIR
		expected bool
	}{
		{name: "both nil", a: nil, b: nil, expected: true},
		{name: "nil vs enabled", a: nil, b: enabled, expected: false},
		{name: "enabled vs nil", a: enabled, b: nil, expected: false},
		{name: "enabled vs enabled", a: enabled, b: &grpcWebIR{perRoute: EnableFilterPerRoute()}, expected: true},
		{name: "enabled vs disabled", a: enabled, b: disabled, expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.a.Equals(tt.b), "a.Equals(b)")
			assert.Equal(t, tt.expected, tt.b.Equals(tt.a), "b.Equals(a)")
		})
	}
}

func TestConstructGRPCWeb(t *testing.T) {
	t.Run("unset", func(t *testing.T) {
		out := &trafficPolicySpecIr{}
		constructGRPCWeb(kgateway.TrafficPolicySpec{}, out)
		assert.Nil(t, out.grpcWeb, "no IR without grpcWeb")
	})
	t.Run("enable", func(t *testing.T) {
		out := &trafficPolicySpecIr{}
		constructGRPCWeb(kgateway.TrafficPolicySpec{GRPCWeb: &kgateway.GRPCWebPolicy{}}, out)
		require.NotNil(t, out.grpcWeb)
		assert.True(t, proto.Equal(EnableFilterPerRoute(), out.grpcWeb.perRoute), "empty grpcWeb enables the filter per route")
		assert.True(t, out.grpcWeb.enabled())
	})
	t.Run("disable", func(t *testing.T) {
		out := &trafficPolicySpecIr{}
		constructGRPCWeb(kgateway.TrafficPolicySpec{GRPCWeb: &kgateway.GRPCWebPolicy{Disable: &shared.PolicyDisable{}}}, out)
		require.NotNil(t, out.grpcWeb)
		assert.True(t, proto.Equal(DisableFilterPerRouteOptional(), out.grpcWeb.perRoute), "disable emits an optional per-route disable")
		assert.False(t, out.grpcWeb.enabled())
	})
}

func TestHandleGRPCWeb(t *testing.T) {
	const fcn = "listener~80"

	t.Run("enable adds the filter to the chain, disabled by default", func(t *testing.T) {
		p := &trafficPolicyPluginGwPass{}
		typed := &ir.TypedFilterConfigMap{}
		p.handleGRPCWeb(fcn, typed, &grpcWebIR{perRoute: EnableFilterPerRoute()})

		assert.True(t, proto.Equal(EnableFilterPerRoute(), typed.GetTypedConfig(envoy_wellknown.GRPCWeb)), "route enables the filter")

		staged := addGRPCWebFilterIfNeeded(nil, p, fcn)
		require.Len(t, staged, 1)
		assert.Equal(t, envoy_wellknown.GRPCWeb, staged[0].Filter.GetName())
		assert.True(t, staged[0].Filter.GetDisabled(), "chain filter is disabled by default")
		assert.Equal(t, grpcWebFilterStage, staged[0].Stage)
		assert.True(t, proto.Equal(utils.MustMessageToAny(&grpcwebv3.GrpcWeb{}), staged[0].Filter.GetTypedConfig()))
	})

	t.Run("disable does not add the filter to the chain", func(t *testing.T) {
		p := &trafficPolicyPluginGwPass{}
		typed := &ir.TypedFilterConfigMap{}
		p.handleGRPCWeb(fcn, typed, &grpcWebIR{perRoute: DisableFilterPerRouteOptional()})

		assert.True(t, proto.Equal(DisableFilterPerRouteOptional(), typed.GetTypedConfig(envoy_wellknown.GRPCWeb)), "route disables the filter")
		assert.Empty(t, addGRPCWebFilterIfNeeded(nil, p, fcn), "no chain filter for a disable-only policy")
	})

	t.Run("filter sits after CORS and compression, before WAF", func(t *testing.T) {
		assert.Positive(t, filters.FilterStageComparison(grpcWebFilterStage, filters.DuringStage(filters.CorsStage)))
		for i := range allCompressionLibraries {
			compressorStage := filters.RelativeToStage(filters.CorsStage, 1+i)
			assert.Positive(t, filters.FilterStageComparison(grpcWebFilterStage, compressorStage), "after compressor %d", i)
		}
		assert.Negative(t, filters.FilterStageComparison(grpcWebFilterStage, filters.BeforeStage(filters.WafStage)))
	})
}
