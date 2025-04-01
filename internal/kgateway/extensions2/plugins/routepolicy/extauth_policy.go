package routepolicy

import (
	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoytransformation "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	transformationpb "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

type extAuthIR struct {
	filter       *envoy_ext_authz_v3.ExtAuthz
	providerName string
	enablement   v1alpha1.ExtAuthEnabled
	fromListener bool
}

// extAuthForSpec translates the ExtAuthz spec into the Envoy configuration
func extAuthForSpec(
	commoncol *common.CommonCollections,
	krtctx krt.HandlerContext,
	trafficpolicy *v1alpha1.TrafficPolicy,
	out *trafficPolicySpecIr,
) {
	getter := (func(name, namespace string) (*ir.GatewayExtension, *ir.BackendObjectIR, error) {
		gExt, err := pluginutils.GetGatewayExtension(commoncol.GatewayExtensions, krtctx, name, namespace)
		if err != nil {
			return nil, nil, err
		}
		if gExt.Type != v1alpha1.GatewayExtensionTypeExtAuth {
			return nil, nil, pluginutils.ErrInvalidExtensionType(v1alpha1.GatewayExtensionTypeExtAuth, gExt.Type)
		}
		var backend *ir.BackendObjectIR
		if gExt.ExtAuth.GrpcService != nil {
			if gExt.ExtAuth.GrpcService.BackendRef == nil {
				return nil, nil, nil
			}
			backendRef := gExt.ExtAuth.GrpcService.BackendRef.BackendObjectReference
			backend, err = commoncol.BackendIndex.GetBackendFromRef(krtctx, gExt.ObjectSource, backendRef)
			if err != nil {
				return nil, nil, err
			}
		}
		return gExt, backend, nil
	})

	extAuthForSpecWithExtensionFunction(getter, trafficpolicy, out)
}

func extAuthForSpecWithExtensionFunction(
	gExtensionGetter func(name, namespace string) (*ir.GatewayExtension, *ir.BackendObjectIR, error),
	trafficpolicy *v1alpha1.TrafficPolicy,
	out *trafficPolicySpecIr,
) {
	policySpec := &trafficpolicy.Spec

	if policySpec == nil || policySpec.ExtAuth == nil {
		return
	}
	spec := policySpec.ExtAuth
	// Create the ExtAuthz configuration
	extAuth := &envoy_ext_authz_v3.ExtAuthz{}
	if spec.FailureModeAllow != nil {
		extAuth.FailureModeAllow = *spec.FailureModeAllow
	}
	if spec.ClearRouteCache != nil {
		extAuth.ClearRouteCache = *spec.ClearRouteCache
	}
	if spec.IncludePeerCertificate != nil {
		extAuth.IncludePeerCertificate = *spec.IncludePeerCertificate
	}
	if spec.IncludeTLSSession != nil {
		extAuth.IncludeTlsSession = *spec.IncludeTLSSession
	}

	// Configure metadata context namespaces if specified
	if len(spec.MetadataContextNamespaces) > 0 {
		extAuth.MetadataContextNamespaces = spec.MetadataContextNamespaces
	}

	// Configure request body buffering if specified
	if spec.WithRequestBody != nil {
		extAuth.WithRequestBody = &envoy_ext_authz_v3.BufferSettings{
			MaxRequestBytes: spec.WithRequestBody.MaxRequestBytes,
		}
		if spec.WithRequestBody.AllowPartialMessage != nil {
			extAuth.GetWithRequestBody().AllowPartialMessage = *spec.WithRequestBody.AllowPartialMessage
		}
		if spec.WithRequestBody.PackAsBytes != nil {
			extAuth.GetWithRequestBody().PackAsBytes = *spec.WithRequestBody.PackAsBytes
		}
	}

	if spec.ExtensionRef != nil {
		_, backend, err := gExtensionGetter(spec.ExtensionRef.Name, trafficpolicy.GetNamespace())
		if err != nil {
			out.errors = append(out.errors, err)
			return
		}
		if backend != nil {
			extAuth.Services = &envoy_ext_authz_v3.ExtAuthz_GrpcService{
				GrpcService: &envoy_core_v3.GrpcService{
					TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
						EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
							ClusterName: backend.ClusterName(),
						},
					},
				},
			}
		}
	}

	nameOrPlaceholder := ""
	if spec.ExtensionRef != nil {
		nameOrPlaceholder = string(spec.ExtensionRef.Name)
	}

	out.extAuth = &extAuthIR{
		filter:       extAuth,
		providerName: nameOrPlaceholder,
		enablement:   spec.Enablement,
	}
}

// extAuthEnablementPerRoute returns a transformation that sets the ext auth filter key to false
// this then fires on the metadata match that all top level configuration shall have.
func extAuthEnablementPerRoute() proto.Message {
	return &transformationpb.RouteTransformations{
		RequestTransformation: &envoytransformation.Transformation{
			TransformationType: &envoytransformation.Transformation_TransformationTemplate{
				TransformationTemplate: &envoytransformation.TransformationTemplate{
					DynamicMetadataValues: []*envoytransformation.TransformationTemplate_DynamicMetadataValue{
						{
							Key:   extAuthGlobalDisableFilterKey,
							Value: &envoytransformation.InjaTemplate{Text: "false"},
						},
					},
				},
			},
		},
	}
}
