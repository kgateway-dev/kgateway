package trafficpolicy

import (
	"fmt"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	set_metadata "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/set_metadata/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
)

var (
	// from envoy code:
	// If the field `config` is configured but is empty, we treat the filter is enabled
	// explicitly.
	// see: https://github.com/envoyproxy/envoy/blob/8ed93ef372f788456b708fc93a7e54e17a013aa7/source/common/router/config_impl.cc#L2552
	enableFilterPerRoute = &routev3.FilterConfig{Config: &anypb.Any{}}
	setMetadataConfig    = &set_metadata.Config{
		Metadata: []*set_metadata.Metadata{
			{
				MetadataNamespace: extAuthGlobalDisableFilterMetadataNamespace,
				Value: &structpb.Struct{Fields: map[string]*structpb.Value{
					extAuthGlobalDisableKey: structpb.NewBoolValue(true),
				}},
			},
		},
	}
)

type extAuthIR struct {
	provider        *TrafficPolicyGatewayExtensionIR
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
	gatewayExtensions krt.Collection[TrafficPolicyGatewayExtensionIR],
	out *trafficPolicySpecIr,
) error {
	policySpec := &trafficpolicy.Spec

	if policySpec.ExtAuth == nil {
		return nil
	}
	spec := policySpec.ExtAuth
	var provider *TrafficPolicyGatewayExtensionIR
	if spec.ExtensionRef != nil {
		gwExtName := types.NamespacedName{Name: spec.ExtensionRef.Name, Namespace: trafficpolicy.GetNamespace()}
		gatewayExtension := krt.FetchOne(krtctx, gatewayExtensions, krt.FilterObjectName(gwExtName))
		if gatewayExtension == nil {
			return fmt.Errorf("gateway extension %s not found", gwExtName)
		}
		if gatewayExtension.err != nil {
			return gatewayExtension.err
		}
		if gatewayExtension.extAuth == nil {
			return pluginutils.ErrInvalidExtensionType(v1alpha1.GatewayExtensionTypeExtAuth, gatewayExtension.extType)
		}
		provider = gatewayExtension
	}

	out.extAuth = &extAuthIR{
		provider:        provider,
		enablement:      spec.Enablement,
		extauthPerRoute: translatePerFilterConfig(spec),
	}
	return nil
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
