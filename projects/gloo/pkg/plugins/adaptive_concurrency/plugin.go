package adaptiveconcurrency

import (
	"fmt"

	envoy_adaptive_concurrency_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/adaptive_concurrency/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/adaptive_concurrency"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var (
	pluginStage = plugins.DuringStage(plugins.RouteStage)
)

const (
	ExtensionName = "envoy.extensions.filters.http.adaptive_concurrency.v3.AdaptiveConcurrency"
)

var (
	_ plugins.Plugin           = new(plugin)
	_ plugins.HttpFilterPlugin = new(plugin)
)

type plugin struct{}

func NewPlugin() *plugin {
	return &plugin{}
}
func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) {}

func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	adaptiveConcurrencyConfig, err := translateAdaptiveConcurrency(&v1.ListenerOptions{})

	if err != nil {
		return nil, err
	}

	if adaptiveConcurrencyConfig == nil {
		return []plugins.StagedHttpFilter{}, nil
	}

	return []plugins.StagedHttpFilter{plugins.MustNewStagedFilter(ExtensionName, adaptiveConcurrencyConfig, pluginStage)}, nil
}

func translateAdaptiveConcurrency(in *v1.ListenerOptions) (*envoy_adaptive_concurrency_v3.AdaptiveConcurrency, error) {
	adaptiveConcurrency := in.GetAdaptiveConcurrency()

	concurrencyLimitParams, err := translateConcurrencyLimitParams(adaptiveConcurrency)
	if err != nil {
		return nil, err
	}

	minRttCalcParams, err := translateMinRttCalcParams(adaptiveConcurrency.GetMinRttCalcParams())
	if err != nil {
		return nil, err
	}

	out := &envoy_adaptive_concurrency_v3.AdaptiveConcurrency{
		ConcurrencyControllerConfig: &envoy_adaptive_concurrency_v3.AdaptiveConcurrency_GradientControllerConfig{
			GradientControllerConfig: &envoy_adaptive_concurrency_v3.GradientControllerConfig{
				ConcurrencyLimitParams: concurrencyLimitParams,
				MinRttCalcParams:       minRttCalcParams,
			},
		},
	}

	return out, nil
	// return &envoy_adaptive_concurrency_v3.AdaptiveConcurrency{
	// 	ConcurrencyControllerConfig: &envoy_adaptive_concurrency_v3.AdaptiveConcurrency_GradientControllerConfig{
	// 		GradientControllerConfig: &envoy_adaptive_concurrency_v3.GradientControllerConfig{
	// 			ConcurrencyLimitParams: &envoy_adaptive_concurrency_v3.GradientControllerConfig_ConcurrencyLimitCalculationParams{
	// 				MaxConcurrencyLimit: &wrapperspb.UInt32Value{Value: 100},
	// 				ConcurrencyUpdateInterval: &durationpb.Duration{ // Required!
	// 					Seconds: 10,
	// 				},
	// 			},
	// 			MinRttCalcParams: &envoy_adaptive_concurrency_v3.GradientControllerConfig_MinimumRTTCalculationParams{
	// 				MinConcurrency: &wrapperspb.UInt32Value{Value: 2},
	// 				Interval: &durationpb.Duration{ // Required!
	// 					Seconds: 60,
	// 				},
	// 				RequestCount: &wrapperspb.UInt32Value{Value: 3},
	// 			},
	// 		},
	// 	},
	// }, nil
}

func translateConcurrencyLimitParams(in *adaptive_concurrency.AdaptiveRequestConcurrencyPolicySpec) (*envoy_adaptive_concurrency_v3.GradientControllerConfig_ConcurrencyLimitCalculationParams, error) {

	if in.GetConcurrencyUpdateIntervalMillis() == 0 {
		return nil, fmt.Errorf("concurrency_update_interval_millis is required")
	}

	out := &envoy_adaptive_concurrency_v3.GradientControllerConfig_ConcurrencyLimitCalculationParams{}
	out.ConcurrencyUpdateInterval = &durationpb.Duration{ // Required!
		Seconds: int64(in.GetConcurrencyUpdateIntervalMillis()) * 1000,
	}

	if in.GetMaxConcurrencyLimit() != nil {
		out.MaxConcurrencyLimit = &wrapperspb.UInt32Value{Value: in.GetMaxConcurrencyLimit().GetValue()}
	}

	return out, nil
}

func translateMinRttCalcParams(in *adaptive_concurrency.AdaptiveRequestConcurrencyPolicySpec_MinRoundtripTimeCalculationParams) (*envoy_adaptive_concurrency_v3.GradientControllerConfig_MinimumRTTCalculationParams, error) {
	if in == nil {
		return nil, fmt.Errorf("min_rtt_calc_params is required")
	}

	if in.GetIntervalMillis() == 0 {
		return nil, fmt.Errorf("interval_millis is required")
	}

	out := &envoy_adaptive_concurrency_v3.GradientControllerConfig_MinimumRTTCalculationParams{
		Interval: &durationpb.Duration{
			Seconds: int64(in.GetIntervalMillis()) * 1000,
		},
	}

	if in.GetRequestCount() != nil {
		out.RequestCount = &wrapperspb.UInt32Value{Value: in.GetRequestCount().GetValue()}
	}

	if in.GetMinConcurrency() != nil {
		out.MinConcurrency = &wrapperspb.UInt32Value{Value: in.GetMinConcurrency().GetValue()}
	}

	return out, nil
}
