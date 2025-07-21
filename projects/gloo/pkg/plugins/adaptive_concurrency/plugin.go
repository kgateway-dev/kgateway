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
	pluginStage                               = plugins.DuringStage(plugins.RouteStage)
	ErrConcurrencyUpdateIntervalMillisMissing = func() error {
		return fmt.Errorf("concurrency_update_interval_millis is required")
	}
	ErrMinRttCalcParamsMissing = func() error {
		return fmt.Errorf("min_rtt_calc_params is required")
	}
	ErrIntervalMillisMissing = func() error {
		return fmt.Errorf("interval_millis is required")
	}
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
	fmt.Printf("GREPME Adaptive concurrency HttpFilters\n")
	in := listener.GetOptions()
	adaptiveConcurrencyConfig, err := translateAdaptiveConcurrency(in)

	if err != nil {
		return nil, err
	}

	if adaptiveConcurrencyConfig == nil {
		return []plugins.StagedHttpFilter{}, nil
	}

	return []plugins.StagedHttpFilter{plugins.MustNewStagedFilter(ExtensionName, adaptiveConcurrencyConfig, pluginStage)}, nil
}

func translateAdaptiveConcurrency(in *v1.HttpListenerOptions) (*envoy_adaptive_concurrency_v3.AdaptiveConcurrency, error) {
	fmt.Printf("GREPME Adaptive concurrency translateAdaptiveConcurrency\n")
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
}

func translateConcurrencyLimitParams(in *adaptive_concurrency.AdaptiveRequestConcurrencyPolicySpec) (*envoy_adaptive_concurrency_v3.GradientControllerConfig_ConcurrencyLimitCalculationParams, error) {
	fmt.Printf("GREPME Adaptive concurrency translateConcurrencyLimitParams - in: %v\n", in)
	if in.GetConcurrencyUpdateIntervalMillis() == 0 {
		fmt.Printf("GREPME Adaptive concurrency translateConcurrencyLimitParams concurrency_update_interval_millis is required\n")
		return nil, ErrConcurrencyUpdateIntervalMillisMissing()
	}

	out := &envoy_adaptive_concurrency_v3.GradientControllerConfig_ConcurrencyLimitCalculationParams{}
	out.ConcurrencyUpdateInterval = &durationpb.Duration{
		Seconds: int64(in.GetConcurrencyUpdateIntervalMillis()) * 1000,
	}

	if in.GetMaxConcurrencyLimit() != nil {
		out.MaxConcurrencyLimit = &wrapperspb.UInt32Value{Value: in.GetMaxConcurrencyLimit().GetValue()}
	}

	return out, nil
}

func translateMinRttCalcParams(in *adaptive_concurrency.AdaptiveRequestConcurrencyPolicySpec_MinRoundtripTimeCalculationParams) (*envoy_adaptive_concurrency_v3.GradientControllerConfig_MinimumRTTCalculationParams, error) {
	fmt.Printf("GREPME Adaptive concurrency translateMinRttCalcParams - in: %v\n", in)
	if in == nil {
		fmt.Printf("GREPME Adaptive concurrency translateMinRttCalcParams min_rtt_calc_params is required\n")
		return nil, ErrMinRttCalcParamsMissing()
	}

	if in.GetIntervalMillis() == 0 {
		fmt.Printf("GREPME Adaptive concurrency translateMinRttCalcParams interval_millis is required\n")
		return nil, ErrIntervalMillisMissing()
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
