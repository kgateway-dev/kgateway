package trafficpolicy

import (
	"fmt"

	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	set_metadata "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/set_metadata/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

var (
	setMetadataConfig = &set_metadata.Config{
		Metadata: []*set_metadata.Metadata{
			{
				MetadataNamespace: extAuthGlobalDisableFilterMetadataNamespace,
				Value: &structpb.Struct{Fields: map[string]*structpb.Value{
					extAuthGlobalDisableKey: structpb.NewBoolValue(true),
				}},
			},
		},
	}

	ExtAuthzEnabledMetadataMatcher = &envoy_matcher_v3.MetadataMatcher{
		Filter: extAuthGlobalDisableFilterMetadataNamespace,
		Invert: true,
		Path: []*envoy_matcher_v3.MetadataMatcher_PathSegment{
			{
				Segment: &envoy_matcher_v3.MetadataMatcher_PathSegment_Key{
					Key: extAuthGlobalDisableKey,
				},
			},
		},
		Value: &envoy_matcher_v3.ValueMatcher{
			MatchPattern: &envoy_matcher_v3.ValueMatcher_BoolMatch{
				BoolMatch: true,
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
	if (e.provider == nil) != (other.provider == nil) {
		return false
	}
	if e.provider != nil && !e.provider.Equals(*other.provider) {
		return false
	}
	return true
}

// extAuthForSpec translates the ExtAuthz spec into the Envoy configuration

func (b *TrafficPolicyBuilder) extAuthForSpec(
	krtctx krt.HandlerContext,
	trafficPolicy *v1alpha1.TrafficPolicy,
	out *trafficPolicySpecIr,
) error {
	policySpec := &trafficPolicy.Spec

	if policySpec.ExtAuth == nil {
		return nil
	}
	spec := policySpec.ExtAuth

	if spec.Enablement == v1alpha1.ExtAuthDisableAll {
		out.extAuth = &extAuthIR{
			provider:        nil,
			enablement:      v1alpha1.ExtAuthDisableAll,
			extauthPerRoute: translatePerFilterConfig(spec),
		}
		return nil
	}

	provider, err := b.FetchGatewayExtension(krtctx, spec.ExtensionRef, trafficPolicy.GetNamespace())
	if err != nil {
		return fmt.Errorf("extauthz: %w", err)
	}
	if provider.ExtType != v1alpha1.GatewayExtensionTypeExtAuth || provider.ExtAuth == nil {
		return pluginutils.ErrInvalidExtensionType(v1alpha1.GatewayExtensionTypeExtAuth, provider.ExtType)
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

func extAuthFilterName(name string) string {
	if name == "" {
		return extauthFilterNamePrefix
	}
	return fmt.Sprintf("%s/%s", extauthFilterNamePrefix, name)
}

func (p *trafficPolicyPluginGwPass) handleExtAuth(fcn string, pCtxTypedFilterConfig *ir.TypedFilterConfigMap, extAuth *extAuthIR) {
	if extAuth == nil {
		return
	}

	// Handle the enablement state
	if extAuth.enablement == v1alpha1.ExtAuthDisableAll {
		// Disable the filter under all providers via the metadata match
		// we have to use the metadata as we dont know what other configurations may have extauth
		pCtxTypedFilterConfig.AddTypedConfig(extAuthGlobalDisableFilterName, EnableFilterPerRoute)
	} else {
		providerName := extAuth.provider.ResourceName()
		if extAuth.extauthPerRoute != nil {
			pCtxTypedFilterConfig.AddTypedConfig(extAuthFilterName(providerName),
				extAuth.extauthPerRoute,
			)
		} else {
			// if you are on a route and not trying to disable it then we need to override the top level disable on the filter chain
			pCtxTypedFilterConfig.AddTypedConfig(extAuthFilterName(providerName), EnableFilterPerRoute)
		}
		p.extAuthPerProvider.Add(fcn, providerName, extAuth.provider)
	}
}
