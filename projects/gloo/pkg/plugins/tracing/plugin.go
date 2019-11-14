package tracing

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"
	"github.com/gogo/protobuf/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	hcmp "github.com/solo-io/gloo/projects/gloo/pkg/plugins/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/internal/common"
)

// default all tracing percentages to 100%
const oneHundredPercent float32 = 100.0

func NewPlugin() *Plugin {
	return &Plugin{}
}

var _ plugins.Plugin = new(Plugin)
var _ hcmp.HcmPlugin = new(Plugin)
var _ plugins.RoutePlugin = new(Plugin)

type Plugin struct {
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

// Manage the tracing portion of the HCM settings
func (p *Plugin) ProcessHcmSettings(cfg *envoyhttp.HttpConnectionManager, hcmSettings *hcm.HttpConnectionManagerSettings) error {

	// only apply tracing config to the listener is using the HCM plugin
	if hcmSettings == nil {
		return nil
	}

	tracingSettings := hcmSettings.Tracing
	if tracingSettings == nil {
		return nil
	}

	// this plugin will overwrite any prior tracing config
	trCfg := &envoyhttp.HttpConnectionManager_Tracing{}

	trCfg.RequestHeadersForTags = tracingSettings.RequestHeadersForTags
	trCfg.Verbose = tracingSettings.Verbose

	// Gloo configures envoy as an ingress, rather than an egress
	trCfg.OperationName = envoyhttp.INGRESS
	if percentages := tracingSettings.GetTracePercentages(); percentages != nil {
		trCfg.ClientSampling = envoySimplePercentWithDefault(percentages.GetClientSamplePercentage(), oneHundredPercent)
		trCfg.RandomSampling = envoySimplePercentWithDefault(percentages.GetRandomSamplePercentage(), oneHundredPercent)
		trCfg.OverallSampling = envoySimplePercentWithDefault(percentages.GetOverallSamplePercentage(), oneHundredPercent)
	} else {
		trCfg.ClientSampling = envoySimplePercent(oneHundredPercent)
		trCfg.RandomSampling = envoySimplePercent(oneHundredPercent)
		trCfg.OverallSampling = envoySimplePercent(oneHundredPercent)
	}
	cfg.Tracing = trCfg
	return nil
}

func envoySimplePercent(numerator float32) *envoy_type.Percent {
	return &envoy_type.Percent{Value: float64(numerator)}
}

// use FloatValue to detect when nil (avoids error-prone float comparisons)
func envoySimplePercentWithDefault(numerator *types.FloatValue, defaultValue float32) *envoy_type.Percent {
	if numerator == nil {
		return envoySimplePercent(defaultValue)
	}
	return envoySimplePercent(numerator.Value)
}

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoyroute.Route) error {
	if in.Options == nil || in.Options.Tracing == nil {
		return nil
	}
	if percentages := in.GetOptions().GetTracing().TracePercentages; percentages != nil {
		out.Tracing = &envoyroute.Tracing{
			ClientSampling:  common.ToEnvoyPercentageWithDefault(percentages.GetClientSamplePercentage(), oneHundredPercent),
			RandomSampling:  common.ToEnvoyPercentageWithDefault(percentages.GetRandomSamplePercentage(), oneHundredPercent),
			OverallSampling: common.ToEnvoyPercentageWithDefault(percentages.GetOverallSamplePercentage(), oneHundredPercent),
		}
	} else {
		out.Tracing = &envoyroute.Tracing{
			ClientSampling:  common.ToEnvoyPercentage(oneHundredPercent),
			RandomSampling:  common.ToEnvoyPercentage(oneHundredPercent),
			OverallSampling: common.ToEnvoyPercentage(oneHundredPercent),
		}
	}
	descriptor := in.Options.Tracing.RouteDescriptor
	if descriptor != "" {
		out.Decorator = &envoyroute.Decorator{
			Operation: descriptor,
		}
	}
	return nil
}
