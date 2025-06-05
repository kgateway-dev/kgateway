package trafficpolicy

import (
	"fmt"

	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	ratev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

type TrafficPolicyGatewayExtensionIR struct {
	Name      string
	ExtType   v1alpha1.GatewayExtensionType
	ExtAuth   *envoy_ext_authz_v3.ExtAuthz
	ExtProc   *envoy_ext_proc_v3.ExternalProcessor
	RateLimit *ratev3.RateLimit
	Err       error
}

// ResourceName returns the unique name for this extension.
func (e TrafficPolicyGatewayExtensionIR) ResourceName() string {
	return e.Name
}

func (e TrafficPolicyGatewayExtensionIR) Equals(other TrafficPolicyGatewayExtensionIR) bool {
	if e.ExtType != other.ExtType {
		return false
	}

	if !proto.Equal(e.ExtAuth, other.ExtAuth) {
		return false
	}
	if !proto.Equal(e.ExtProc, other.ExtProc) {
		return false
	}
	if !proto.Equal(e.RateLimit, other.RateLimit) {
		return false
	}

	// Compare providers
	if e.Err == nil && other.Err == nil {
		return true
	}
	if e.Err == nil || other.Err == nil {
		return false
	}

	return e.Err.Error() == other.Err.Error()
}

func TranslateGatewayExtensionBuilder(commoncol *common.CommonCollections) func(krtctx krt.HandlerContext, gExt ir.GatewayExtension) *TrafficPolicyGatewayExtensionIR {
	return func(krtctx krt.HandlerContext, gExt ir.GatewayExtension) *TrafficPolicyGatewayExtensionIR {
		p := &TrafficPolicyGatewayExtensionIR{
			Name:    krt.Named{Name: gExt.Name, Namespace: gExt.Namespace}.ResourceName(),
			ExtType: gExt.Type,
		}

		switch gExt.Type {
		case v1alpha1.GatewayExtensionTypeExtAuth:
			envoyGrpcService, err := ResolveExtGrpcService(krtctx, commoncol.BackendIndex, false, gExt.ObjectSource, gExt.ExtAuth.GrpcService)
			if err != nil {
				// TODO: should this be a warning, and set cluster to blackhole?
				p.Err = fmt.Errorf("failed to resolve ExtAuth backend: %w", err)
				return p
			}

			p.ExtAuth = &envoy_ext_authz_v3.ExtAuthz{
				Services: &envoy_ext_authz_v3.ExtAuthz_GrpcService{
					GrpcService: envoyGrpcService,
				},
				FilterEnabledMetadata: ExtAuthzEnabledMetadataMatcher,
			}

		case v1alpha1.GatewayExtensionTypeExtProc:
			envoyGrpcService, err := ResolveExtGrpcService(krtctx, commoncol.BackendIndex, false, gExt.ObjectSource, gExt.ExtProc.GrpcService)
			if err != nil {
				p.Err = fmt.Errorf("failed to resolve ExtProc backend: %w", err)
				return p
			}

			p.ExtProc = &envoy_ext_proc_v3.ExternalProcessor{
				GrpcService: envoyGrpcService,
			}

		case v1alpha1.GatewayExtensionTypeRateLimit:
			if gExt.RateLimit == nil {
				p.Err = fmt.Errorf("rate limit extension missing configuration")
				return p
			}

			grpcService, err := ResolveExtGrpcService(krtctx, commoncol.BackendIndex, false, gExt.ObjectSource, gExt.RateLimit.GrpcService)
			if err != nil {
				p.Err = fmt.Errorf("ratelimit: %w", err)
				return p
			}

			// Use the specialized function for rate limit service resolution
			rateLimitConfig := resolveRateLimitService(grpcService, gExt.RateLimit)

			p.RateLimit = rateLimitConfig
		}
		return p
	}
}
