package trafficpolicy

import (
	"fmt"

	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmputils"
)

const (
	ExtAuthGlobalDisableFilterName              = "global_disable/ext_auth"
	ExtAuthGlobalDisableFilterMetadataNamespace = "dev.kgateway.disable_ext_auth"
	globalFilterDisableMetadataKey              = "disable"
	extauthFilterNamePrefix                     = "ext_auth"
)

var ExtAuthzEnabledMetadataMatcher = &envoy_matcher_v3.MetadataMatcher{
	Filter: ExtAuthGlobalDisableFilterMetadataNamespace,
	Invert: true,
	Path: []*envoy_matcher_v3.MetadataMatcher_PathSegment{
		{
			Segment: &envoy_matcher_v3.MetadataMatcher_PathSegment_Key{
				Key: globalFilterDisableMetadataKey,
			},
		},
	},
	Value: &envoy_matcher_v3.ValueMatcher{
		MatchPattern: &envoy_matcher_v3.ValueMatcher_BoolMatch{
			BoolMatch: true,
		},
	},
}

type extAuthIR struct {
	provider            *TrafficPolicyGatewayExtensionIR
	disableAllProviders bool
	perRoute            *envoy_ext_authz_v3.ExtAuthzPerRoute
}

var _ PolicySubIR = &extAuthIR{}

// Equals compares two ExtAuthIR instances for equality
func (e *extAuthIR) Equals(other PolicySubIR) bool {
	otherExtAuth, ok := other.(*extAuthIR)
	if !ok {
		return false
	}
	if e == nil || otherExtAuth == nil {
		return e == nil && otherExtAuth == nil
	}
	if e.disableAllProviders != otherExtAuth.disableAllProviders {
		return false
	}
	if !proto.Equal(e.perRoute, otherExtAuth.perRoute) {
		return false
	}
	// Compare providers
	if !cmputils.CompareWithNils(e.provider, otherExtAuth.provider, func(a, b *TrafficPolicyGatewayExtensionIR) bool {
		return a.Equals(*b)
	}) {
		return false
	}
	return true
}

func (e *extAuthIR) Validate() error {
	if e == nil {
		return nil
	}
	if e.perRoute != nil {
		if err := e.perRoute.ValidateAll(); err != nil {
			return err
		}
	}
	if e.provider != nil {
		return e.provider.Validate()
	}
	return nil
}

// constructExtAuth constructs the external authentication policy IR from the policy specification.
func constructExtAuth(
	krtctx krt.HandlerContext,
	in *v1alpha1.TrafficPolicy,
	fetchGatewayExtension FetchGatewayExtensionFunc,
	out *trafficPolicySpecIr,
) error {
	spec := in.Spec.ExtAuth
	if spec == nil {
		return nil
	}

	if spec.Disable != nil {
		out.extAuth = &extAuthIR{
			disableAllProviders: true,
		}
		return nil
	}

	perRouteCfg := buildExtAuthPerRouteFilterConfig(spec)

	// kubebuilder validation ensures the extensionRef is not nil, since disable is nil
	provider, err := fetchGatewayExtension(krtctx, *spec.ExtensionRef, in.GetNamespace())
	if err != nil {
		return fmt.Errorf("extauthz: %w", err)
	}
	if provider.ExtType != v1alpha1.GatewayExtensionTypeExtAuth || provider.ExtAuth == nil {
		return pluginutils.ErrInvalidExtensionType(v1alpha1.GatewayExtensionTypeExtAuth, provider.ExtType)
	}

	out.extAuth = &extAuthIR{
		provider: provider,
		perRoute: perRouteCfg,
	}
	return nil
}

func buildExtAuthPerRouteFilterConfig(
	spec *v1alpha1.ExtAuthPolicy,
) *envoy_ext_authz_v3.ExtAuthzPerRoute {
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

	// Add the global disable all filter if all providers are disabled
	if extAuth.disableAllProviders {
		pCtxTypedFilterConfig.AddTypedConfig(ExtAuthGlobalDisableFilterName, EnableFilterPerRoute)
		return
	}

	providerName := extAuth.provider.ResourceName()
	p.extAuthPerProvider.Add(fcn, providerName, extAuth.provider)

	// Filter is not disabled, set the PerRouteConfig
	if extAuth.perRoute != nil {
		pCtxTypedFilterConfig.AddTypedConfig(extAuthFilterName(providerName), extAuth.perRoute)
	} else {
		// if you are on a route and not trying to disable it then we need to override the top level disable on the filter chain
		pCtxTypedFilterConfig.AddTypedConfig(extAuthFilterName(providerName), EnableFilterPerRoute)
	}
}
