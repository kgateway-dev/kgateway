package trafficpolicy

import (
	"fmt"
	"slices"

	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmputils"
)

const (
	// extProcFilterPrefix is the prefix for the ExtProc filter name
	extProcFilterPrefix = "ext_proc/"

	// extProcGlobalDisableFilterName is the name of the filter for ExtProc that disables all ExtProc providers
	extProcGlobalDisableFilterName = "global_disable/ext_proc"

	// extProcGlobalDisableFilterMetadataNamespace is the metadata namespace for the global disable ExtProc filter
	extProcGlobalDisableFilterMetadataNamespace = "dev.kgateway.disable_ext_proc"
)

type extprocIR struct {
	perProviderConfig   []*perProviderExtProcConfig
	disableAllProviders bool
	// providerNames is used to track duplicates during policy merging,
	// and has no relevance to the policy config, so it can be excluded from Equals
	// +noKrtEquals
	providerNames sets.Set[string]
}

type perProviderExtProcConfig struct {
	provider       *TrafficPolicyGatewayExtensionIR
	perRouteConfig *envoy_ext_proc_v3.ExtProcPerRoute
}

var _ PolicySubIR = &extprocIR{}

func (e *extprocIR) Equals(other PolicySubIR) bool {
	otherExtProc, ok := other.(*extprocIR)
	if !ok {
		return false
	}
	if e == nil || otherExtProc == nil {
		return e == nil && otherExtProc == nil
	}
	if e.disableAllProviders != otherExtProc.disableAllProviders {
		return false
	}
	if !slices.EqualFunc(e.perProviderConfig, otherExtProc.perProviderConfig, func(a, b *perProviderExtProcConfig) bool {
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

func (e *extprocIR) Validate() error {
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
			if err := p.provider.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

// constructExtProc constructs the external processing policy IR from the policy specification.
func constructExtProc(
	krtctx krt.HandlerContext,
	in *v1alpha1.TrafficPolicy,
	fetchGatewayExtension FetchGatewayExtensionFunc,
	out *trafficPolicySpecIr,
) error {
	spec := in.Spec.ExtProc
	if spec == nil {
		return nil
	}

	if spec.Disable != nil {
		out.extProc = &extprocIR{
			disableAllProviders: true,
		}
		return nil
	}

	// kubebuilder validation ensures the extensionRef is not nil, since disable is nil
	gatewayExtension, err := fetchGatewayExtension(krtctx, *spec.ExtensionRef, in.GetNamespace())
	if err != nil {
		return fmt.Errorf("extproc: %w", err)
	}
	if gatewayExtension.ExtType != v1alpha1.GatewayExtensionTypeExtProc || gatewayExtension.ExtProc == nil {
		return pluginutils.ErrInvalidExtensionType(v1alpha1.GatewayExtensionTypeExtAuth, gatewayExtension.ExtType)
	}
	out.extProc = &extprocIR{
		perProviderConfig: []*perProviderExtProcConfig{
			{
				provider:       gatewayExtension,
				perRouteConfig: translateExtProcPerFilterConfig(spec),
			},
		},
		providerNames: sets.New(providerName(gatewayExtension)),
	}
	return nil
}

func translateExtProcPerFilterConfig(
	extProc *v1alpha1.ExtProcPolicy,
) *envoy_ext_proc_v3.ExtProcPerRoute {
	overrides := &envoy_ext_proc_v3.ExtProcOverrides{}
	if extProc.ProcessingMode != nil {
		overrides.ProcessingMode = toEnvoyProcessingMode(extProc.ProcessingMode)
	}

	return &envoy_ext_proc_v3.ExtProcPerRoute{
		Override: &envoy_ext_proc_v3.ExtProcPerRoute_Overrides{
			Overrides: overrides,
		},
	}
}

// headerSendModeFromString converts a string to envoy HeaderSendMode
func headerSendModeFromString(mode *string) envoy_ext_proc_v3.ProcessingMode_HeaderSendMode {
	if mode == nil {
		return envoy_ext_proc_v3.ProcessingMode_DEFAULT
	}
	switch *mode {
	case "SEND":
		return envoy_ext_proc_v3.ProcessingMode_SEND
	case "SKIP":
		return envoy_ext_proc_v3.ProcessingMode_SKIP
	default:
		return envoy_ext_proc_v3.ProcessingMode_DEFAULT
	}
}

// bodySendModeFromString converts a string to envoy BodySendMode
func bodySendModeFromString(mode *string) envoy_ext_proc_v3.ProcessingMode_BodySendMode {
	if mode == nil {
		return envoy_ext_proc_v3.ProcessingMode_NONE
	}
	switch *mode {
	case "STREAMED":
		return envoy_ext_proc_v3.ProcessingMode_STREAMED
	case "BUFFERED":
		return envoy_ext_proc_v3.ProcessingMode_BUFFERED
	case "BUFFERED_PARTIAL":
		return envoy_ext_proc_v3.ProcessingMode_BUFFERED_PARTIAL
	case "FULL_DUPLEX_STREAMED":
		return envoy_ext_proc_v3.ProcessingMode_FULL_DUPLEX_STREAMED
	default:
		return envoy_ext_proc_v3.ProcessingMode_NONE
	}
}

// toEnvoyProcessingMode converts our ProcessingMode to envoy's ProcessingMode
func toEnvoyProcessingMode(p *v1alpha1.ProcessingMode) *envoy_ext_proc_v3.ProcessingMode {
	if p == nil {
		return nil
	}

	return &envoy_ext_proc_v3.ProcessingMode{
		RequestHeaderMode:   headerSendModeFromString(p.RequestHeaderMode),
		ResponseHeaderMode:  headerSendModeFromString(p.ResponseHeaderMode),
		RequestBodyMode:     bodySendModeFromString(p.RequestBodyMode),
		ResponseBodyMode:    bodySendModeFromString(p.ResponseBodyMode),
		RequestTrailerMode:  headerSendModeFromString(p.RequestTrailerMode),
		ResponseTrailerMode: headerSendModeFromString(p.ResponseTrailerMode),
	}
}

func extProcFilterName(name string) string {
	if name == "" {
		return extProcFilterPrefix
	}
	return extProcFilterPrefix + name
}

func (p *trafficPolicyPluginGwPass) handleExtProc(filterChain string, pCtxTypedFilterConfig *ir.TypedFilterConfigMap, in *extprocIR) {
	if in == nil {
		return
	}

	// Add the global disable all filter if all providers are disabled
	if in.disableAllProviders {
		pCtxTypedFilterConfig.AddTypedConfig(extProcGlobalDisableFilterName, EnableFilterPerRoute)
		return
	}

	for _, cfg := range in.perProviderConfig {
		providerName := providerName(cfg.provider)
		p.extProcPerProvider.Add(filterChain, providerName, cfg.provider)
		pCtxTypedFilterConfig.AddTypedConfig(extProcFilterName(providerName), cfg.perRouteConfig)
	}
}

func providerName(provider *TrafficPolicyGatewayExtensionIR) string {
	if provider == nil {
		return ""
	}
	return provider.ResourceName()
}
