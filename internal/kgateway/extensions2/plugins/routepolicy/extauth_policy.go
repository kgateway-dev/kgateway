package routepolicy

import (
	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoytransformation "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	transformationpb "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
)

type extAuthIR struct {
	filter       *envoy_ext_authz_v3.ExtAuthz
	providerName string
	enablement   v1alpha1.ExtAuthEnabled
}

// extAuthForSpec translates the ExtAuthz spec into the Envoy configuration
func extAuthForSpec(
	gatewayExtensions *krtcollections.GatewayExtensionIndex,
	krtctx krt.HandlerContext,
	routepolicy *v1alpha1.RoutePolicy,
	out *routeSpecIr) {
	getter := (func(name, namespace string) (*ir.GatewayExtension, error) {
		return pluginutils.GetGatewayExtension(gatewayExtensions, krtctx, name, namespace)
	})

	extAuthForSpecWithExtensionFunction(getter, routepolicy, out)
}

func extAuthForSpecWithExtensionFunction(
	gExtensionGetter func(name, namespace string) (*ir.GatewayExtension, error),
	routepolicy *v1alpha1.RoutePolicy,
	out *routeSpecIr) {
	routeSpec := &routepolicy.Spec

	if routeSpec == nil || routeSpec.ExtAuth == nil {
		return
	}
	spec := routeSpec.ExtAuth
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
		// service, err := commoncol.BackendIndex.GetBackendFromRef(krtctx, parentSrc, log.GrpcService.BackendRef.BackendObjectReference)
		gExt, err := gExtensionGetter(spec.ExtensionRef.Name, routepolicy.GetNamespace())
		if err != nil {
			out.errors = append(out.errors, err)
			return
		}
		if gExt.Type != v1alpha1.GatewayExtensionTypeExtAuth {
			out.errors = append(out.errors, pluginutils.ErrInvalidExtensionType(v1alpha1.GatewayExtensionTypeExtAuth, gExt.Type))
			return
		}

		extAuth.Services = &envoy_ext_authz_v3.ExtAuthz_GrpcService{
			GrpcService: &envoy_core_v3.GrpcService{
				TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
						ClusterName: pluginutils.BackendToEnvoyCluster(gExt.ExtAuth.BackendRef),
					},
				},
			},
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
