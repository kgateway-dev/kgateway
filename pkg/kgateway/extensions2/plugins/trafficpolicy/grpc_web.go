package trafficpolicy

import (
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	grpcwebv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/grpc_web/v3"
	envoy_wellknown "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/proto"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// grpcWebFilterStage places the gRPC-Web filter after CORS, so browser preflight requests are
// answered before it, and after the compressor and decompressor filters, which sit at
// CorsStage+1..+len(allCompressionLibraries). Being later in the chain than those filters means a
// request body is decompressed before it is converted to gRPC, and a response is converted to
// gRPC-Web before it is compressed. It stays ahead of WAF and authentication, so those filters
// see the converted gRPC request.
var grpcWebFilterStage = filters.RelativeToStage(filters.CorsStage, 1+len(allCompressionLibraries))

type grpcWebIR struct {
	// perRoute enables or disables the chain's gRPC-Web filter for the routes this policy covers.
	perRoute *envoyroutev3.FilterConfig
}

var _ PolicySubIR = &grpcWebIR{}

func (g *grpcWebIR) Equals(other PolicySubIR) bool {
	otherGrpcWeb, ok := other.(*grpcWebIR)
	if !ok {
		return false
	}
	if g == nil || otherGrpcWeb == nil {
		return g == nil && otherGrpcWeb == nil
	}
	return proto.Equal(g.perRoute, otherGrpcWeb.perRoute)
}

// Validate is a no-op: the IR holds no user-supplied values.
func (g *grpcWebIR) Validate() error { return nil }

// enabled reports whether this policy turns gRPC-Web on.
func (g *grpcWebIR) enabled() bool {
	return g != nil && g.perRoute != nil && !g.perRoute.GetDisabled()
}

// constructGRPCWeb constructs the gRPC-Web policy IR from the policy specification.
func constructGRPCWeb(spec kgateway.TrafficPolicySpec, out *trafficPolicySpecIr) {
	if spec.GRPCWeb == nil {
		return
	}
	perRoute := EnableFilterPerRoute()
	if spec.GRPCWeb.Disable != nil {
		// Optional, so Envoy accepts it on a chain that has no gRPC-Web filter because no
		// policy on it enables gRPC-Web.
		perRoute = DisableFilterPerRouteOptional()
	}
	out.grpcWeb = &grpcWebIR{perRoute: perRoute}
}

func (p *trafficPolicyPluginGwPass) handleGRPCWeb(fcn string, typedFilterConfig *ir.TypedFilterConfigMap, grpcWeb *grpcWebIR) {
	if typedFilterConfig == nil || grpcWeb == nil {
		return
	}
	typedFilterConfig.AddTypedConfig(envoy_wellknown.GRPCWeb, grpcWeb.perRoute)

	// The filter goes into the chain disabled and only routes whose policy enables it turn it on.
	// A policy that only disables gRPC-Web does not need the filter in the chain.
	if !grpcWeb.enabled() {
		return
	}
	if p.grpcWebInChain == nil {
		p.grpcWebInChain = make(map[string]bool)
	}
	p.grpcWebInChain[fcn] = true
}

// addGRPCWebFilterIfNeeded adds the chain's disabled-by-default gRPC-Web filter.
func addGRPCWebFilterIfNeeded(staged []filters.StagedHttpFilter, p *trafficPolicyPluginGwPass, fcn string) []filters.StagedHttpFilter {
	if !p.grpcWebInChain[fcn] {
		return staged
	}
	filter := filters.MustNewStagedFilter(envoy_wellknown.GRPCWeb, &grpcwebv3.GrpcWeb{}, grpcWebFilterStage)
	filter.Filter.Disabled = true
	return append(staged, filter)
}
