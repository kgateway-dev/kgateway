package adaptiveconcurrency

import (
	envoy_adaptive_concurrency_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/adaptive_concurrency/v3"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"

	durationpb "google.golang.org/protobuf/types/known/durationpb"
	wrapperspb "google.golang.org/protobuf/types/known/wrapperspb"
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

	out := &envoy_adaptive_concurrency_v3.AdaptiveConcurrency{}

	adaptiveConcurrency := in.GetAdaptiveConcurrency()

	if adaptiveConcurrency == nil {
		return nil, nil
	}

	return &envoy_adaptive_concurrency_v3.AdaptiveConcurrency{
		ConcurrencyControllerConfig: &envoy_adaptive_concurrency_v3.AdaptiveConcurrency_GradientControllerConfig{
			GradientControllerConfig: &envoy_adaptive_concurrency_v3.GradientControllerConfig{
				ConcurrencyLimitParams: &envoy_adaptive_concurrency_v3.GradientControllerConfig_ConcurrencyLimitCalculationParams{
					MaxConcurrencyLimit: &wrapperspb.UInt32Value{Value: 100},
					ConcurrencyUpdateInterval: &durationpb.Duration{ // Required!
						Seconds: 10,
					},
				},
				MinRttCalcParams: &envoy_adaptive_concurrency_v3.GradientControllerConfig_MinimumRTTCalculationParams{
					MinConcurrency: &wrapperspb.UInt32Value{Value: 2},
					Interval: &durationpb.Duration{ // Required!
						Seconds: 60,
					},
					RequestCount: &wrapperspb.UInt32Value{Value: 3},
				},
			},
		},
	}
}
