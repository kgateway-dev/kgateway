package tracing

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	envoy_type "github.com/envoyproxy/go-control-plane/envoy/type"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/hcm"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	hcmp "github.com/solo-io/gloo/projects/gloo/pkg/plugins/hcm"
)

var (
	// always produce a trace whenever the header "x-client-trace-id" is passed
	clientSamplingNumerator uint32 = 100
	// never trace at random
	randomSamplingNumerator uint32 = 0
	// do not limit the number of traces
	// (always produce a trace whenever the header "x-client-trace-id" is passed)
	overallSamplingNumerator uint32 = 100

	// use the same fixed rates for the listener and route. Have to create separate vars due to different input types
	clientSamplingRate, clientSamplingRateFractional   = getDualPercentForms(clientSamplingNumerator)
	randomSamplingRate, randomSamplingRateFractional   = getDualPercentForms(randomSamplingNumerator)
	overallSamplingRate, overallSamplingRateFractional = getDualPercentForms(overallSamplingNumerator)
)

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

	// these fields are user-configurable
	trCfg.RequestHeadersForTags = tracingSettings.RequestHeadersForTags
	trCfg.Verbose = tracingSettings.Verbose

	// the following fields are hard-coded (the may be exposed in the future as desired)
	// Gloo configures envoy as an ingress, rather than an egress
	trCfg.OperationName = envoyhttp.INGRESS
	trCfg.ClientSampling = clientSamplingRate
	trCfg.RandomSampling = randomSamplingRate
	trCfg.OverallSampling = overallSamplingRate
	cfg.Tracing = trCfg
	return nil
}

func getDualPercentForms(numerator uint32) (*envoy_type.Percent, *envoy_type.FractionalPercent) {
	percentForm := &envoy_type.Percent{Value: float64(numerator)}
	fractionalForm := &envoy_type.FractionalPercent{Numerator: numerator, Denominator: envoy_type.FractionalPercent_HUNDRED}
	return percentForm, fractionalForm
}

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoyroute.Route) error {
	if in.RoutePlugins == nil || in.RoutePlugins.Tracing == nil {
		return nil
	}
	// set the constant values
	out.Tracing = &envoyroute.Tracing{
		ClientSampling:  clientSamplingRateFractional,
		RandomSampling:  randomSamplingRateFractional,
		OverallSampling: overallSamplingRateFractional,
	}
	// add a user-defined decorator if one is provided
	descriptor := in.RoutePlugins.Tracing.RouteDescriptor
	if descriptor != "" {
		out.Decorator = &envoyroute.Decorator{
			Operation: descriptor,
		}
	}
	return nil
}
