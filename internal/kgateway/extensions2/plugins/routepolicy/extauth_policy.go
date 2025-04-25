package routepolicy

import (
	"fmt"

	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoytransformation "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	transformationpb "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
)

type extAuthIR struct {
	provider        *trafficPolicyGatewayExtensionIR
	enablement      v1alpha1.ExtAuthEnabled
	extauthPerRoute *envoy_ext_authz_v3.ExtAuthzPerRoute
}

// Equals compares two extAuthIR instances for equality
func (e *extAuthIR) Equals(other *extAuthIR) bool {
	if e == nil && other == nil {
		return true
	}
	if e == nil || other == nil {
		return false
	}

	// Compare enablement
	if e.enablement != other.enablement {
		return false
	}

	if !proto.Equal(e.extauthPerRoute, other.extauthPerRoute) {
		return false
	}

	// Compare providers
	if e.provider == nil && other.provider == nil {
		return true
	}
	if e.provider == nil || other.provider == nil {
		return false
	}

	return e.provider.Equals(*other.provider)
}

// extAuthForSpec translates the ExtAuthz spec into the Envoy configuration
func extAuthForSpec(
	commoncol *common.CommonCollections,
	krtctx krt.HandlerContext,
	trafficpolicy *v1alpha1.TrafficPolicy,
	gatewayExtensions krt.Collection[trafficPolicyGatewayExtensionIR],
	out *trafficPolicySpecIr,
) {
	policySpec := &trafficpolicy.Spec

	if policySpec == nil || policySpec.ExtAuth == nil {
		return
	}
	spec := policySpec.ExtAuth
	var provider *trafficPolicyGatewayExtensionIR

	if spec.ExtensionRef != nil {
		gwExtName := types.NamespacedName{Name: spec.ExtensionRef.Name, Namespace: trafficpolicy.GetNamespace()}
		gatewayExtension := krt.FetchOne(krtctx, gatewayExtensions, krt.FilterObjectName(gwExtName))
		if gatewayExtension == nil {
			out.errors = append(out.errors, fmt.Errorf("extauth extension not found"))
			return
		}
		if gatewayExtension.err != nil {
			out.errors = append(out.errors, gatewayExtension.err)
			return
		}
		if gatewayExtension.extAuth == nil {
			out.errors = append(out.errors, pluginutils.ErrInvalidExtensionType(v1alpha1.GatewayExtensionTypeExtAuth, gatewayExtension.extType))
			return
		}
		provider = gatewayExtension
	}

	out.extAuth = &extAuthIR{
		provider:        provider,
		enablement:      spec.Enablement,
		extauthPerRoute: translatePerFilterConfig(spec),
	}
}

func translatePerFilterConfig(spec *v1alpha1.ExtAuthPolicy) *envoy_ext_authz_v3.ExtAuthzPerRoute {
	checkSettings := &envoy_ext_authz_v3.CheckSettings{}

	// Create the ExtAuthz configuration
	// Configure request body buffering if specified
	if spec.WithRequestBody != nil {
		checkSettings.WithRequestBody = &envoy_ext_authz_v3.BufferSettings{
			MaxRequestBytes: spec.WithRequestBody.MaxRequestBytes,
		}
		if spec.WithRequestBody.AllowPartialMessage != nil {
			checkSettings.GetWithRequestBody().AllowPartialMessage = *spec.WithRequestBody.AllowPartialMessage
		}
		if spec.WithRequestBody.PackAsBytes != nil {
			checkSettings.GetWithRequestBody().PackAsBytes = *spec.WithRequestBody.PackAsBytes
		}
	}
	checkSettings.ContextExtensions = spec.ContextExtensions

	if proto.Size(checkSettings) > 0 {
		return &envoy_ext_authz_v3.ExtAuthzPerRoute{
			Override: &envoy_ext_authz_v3.ExtAuthzPerRoute_CheckSettings{
				CheckSettings: checkSettings,
			},
		}
	}
	return nil
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
