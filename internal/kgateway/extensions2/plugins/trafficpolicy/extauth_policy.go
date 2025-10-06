package trafficpolicy

import (
	"fmt"
	"slices"

	envoy_ext_authz_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	envoy_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/util/sets"

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
	// perProviderConfig is a list of ExtAuth providers that may be the result of a
	// merge between policy IRs attached to the same resource or just a single IR
	// when representing a singular policy before a merge
	perProviderConfig   []*perProviderExtAuthConfig
	disableAllProviders bool
	// providerNames is used to track duplicates during policy merging,
	// and has no relevance to the policy config, so it can be excluded from Equals
	// +noKrtEquals
	providerNames sets.Set[string]
}

type perProviderExtAuthConfig struct {
	provider       *TrafficPolicyGatewayExtensionIR
	perRouteConfig *envoy_ext_authz_v3.ExtAuthzPerRoute
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
	if !slices.EqualFunc(e.perProviderConfig, otherExtAuth.perProviderConfig, func(a, b *perProviderExtAuthConfig) bool {
		// compare perRouteConfig
		return proto.Equal(a.perRouteConfig, b.perRouteConfig) &&
			// compare provider config
			cmputils.CompareWithNils(a.provider, b.provider, func(a, b *TrafficPolicyGatewayExtensionIR) bool {
				return a.Equals(*b)
			})
	}) {
		return false
	}

	return true
}

func (e *extAuthIR) Validate() error {
	if e == nil {
		return nil
	}
	for _, p := range e.perProviderConfig {
		if p.perRouteConfig != nil {
			if err := p.perRouteConfig.ValidateAll(); err != nil {
				return err
			}
		}
		if p.provider != nil {
			return p.provider.Validate()
		}
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

	// kubebuilder validation ensures the extensionRef is not nil, since disable is nil
	provider, err := fetchGatewayExtension(krtctx, *spec.ExtensionRef, in.GetNamespace())
	if err != nil {
		return fmt.Errorf("extauth: %w", err)
	}
	if provider.ExtType != v1alpha1.GatewayExtensionTypeExtAuth || provider.ExtAuth == nil {
		return pluginutils.ErrInvalidExtensionType(v1alpha1.GatewayExtensionTypeExtAuth, provider.ExtType)
	}

	out.extAuth = &extAuthIR{
		perProviderConfig: []*perProviderExtAuthConfig{
			{
				provider:       provider,
				perRouteConfig: buildExtAuthPerRouteFilterConfig(spec),
			},
		},
		providerNames: sets.New(providerName(provider)),
	}
	return nil
}

func buildExtAuthPerRouteFilterConfig(
	spec *v1alpha1.ExtAuthPolicy,
) *envoy_ext_authz_v3.ExtAuthzPerRoute {
	checkSettings := &envoy_ext_authz_v3.CheckSettings{}

	if spec.WithRequestBody != nil {
		checkSettings.WithRequestBody = &envoy_ext_authz_v3.BufferSettings{
			MaxRequestBytes:     uint32(spec.WithRequestBody.MaxRequestBytes), // nolint:gosec // G115: kubebuilder validation ensures safe for uint32
			AllowPartialMessage: spec.WithRequestBody.AllowPartialMessage,
			PackAsBytes:         spec.WithRequestBody.PackAsBytes,
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

func (p *trafficPolicyPluginGwPass) handleExtAuth(filterChain string, pCtxTypedFilterConfig *ir.TypedFilterConfigMap, in *extAuthIR) {
	if in == nil {
		return
	}

	// Add the global disable all filter if all providers are disabled
	if in.disableAllProviders {
		pCtxTypedFilterConfig.AddTypedConfig(ExtAuthGlobalDisableFilterName, EnableFilterPerRoute)
		return
	}

	for _, cfg := range in.perProviderConfig {
		providerName := providerName(cfg.provider)
		p.extAuthPerProvider.Add(filterChain, providerName, cfg.provider)

		// Filter is not disabled, set the PerRouteConfig
		if cfg.perRouteConfig != nil {
			pCtxTypedFilterConfig.AddTypedConfig(extAuthFilterName(providerName), cfg.perRouteConfig)
		} else {
			// if you are on a route and not trying to disable it then we need to override the top level disable on the filter chain
			pCtxTypedFilterConfig.AddTypedConfig(extAuthFilterName(providerName), EnableFilterPerRoute)
		}
	}
}
