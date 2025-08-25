package trafficpolicy

import (
	"fmt"

	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"

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
	provider            *TrafficPolicyGatewayExtensionIR
	perRoute            *envoy_ext_proc_v3.ExtProcPerRoute
	disableAllProviders bool
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
	if !proto.Equal(e.perRoute, otherExtProc.perRoute) {
		return false
	}
	if !cmputils.CompareWithNils(e.provider, otherExtProc.provider, func(a, b *TrafficPolicyGatewayExtensionIR) bool {
		return a.Equals(*b)
	}) {
		return false
	}
	return true
}

func (e *extprocIR) Validate() error {
	if e == nil {
		return nil
	}
	if e.perRoute != nil {
		if err := e.perRoute.ValidateAll(); err != nil {
			return err
		}
	}
	if e.provider != nil {
		if err := e.provider.Validate(); err != nil {
			return err
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
		provider: gatewayExtension,
		perRoute: translateExtProcPerFilterConfig(spec),
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

func (p *trafficPolicyPluginGwPass) handleExtProc(filterChain string, pCtxTypedFilterConfig *ir.TypedFilterConfigMap, extProc *extprocIR) {
	if extProc == nil {
		return
	}

	// Add the global disable all filter if all providers are disabled
	if extProc.disableAllProviders {
		pCtxTypedFilterConfig.AddTypedConfig(extProcGlobalDisableFilterName, EnableFilterPerRoute)
		return
	}

	providerName := extProc.provider.ResourceName()
	p.extProcPerProvider.Add(filterChain, providerName, extProc.provider)
	pCtxTypedFilterConfig.AddTypedConfig(extProcFilterName(providerName), extProc.perRoute)
}
