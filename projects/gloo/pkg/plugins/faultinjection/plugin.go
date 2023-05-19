package faultinjection

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyfault "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/common/fault/v3"
	envoyhttpfault "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/fault/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	fault "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/faultinjection"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/internal/common"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
)

var (
	_ plugins.Plugin           = new(plugin)
	_ plugins.HttpFilterPlugin = new(plugin)
	_ plugins.RoutePlugin      = new(plugin)
)

// https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/fault_filter
const (
	ExtensionName = "fault_injection"
)

var pluginStage = plugins.DuringStage(plugins.FaultStage)

type plugin struct {
	removeUnused              bool
	filterRequiredForListener map[*v1.HttpListener]struct{}
}

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) {
	p.removeUnused = params.Settings.GetGloo().GetRemoveUnusedFilters().GetValue()
	p.filterRequiredForListener = make(map[*v1.HttpListener]struct{})
}

// HttpFilters addes the fault injection listener which can then be configured at a reoute level.
func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	_, ok := p.filterRequiredForListener[listener]
	if !ok && p.removeUnused {
		return []plugins.StagedHttpFilter{}, nil
	}
	// put the filter in the chain, but the actual faults will be configured on the routes
	return []plugins.StagedHttpFilter{plugins.MustNewStagedFilter(wellknown.Fault, &envoyhttpfault.HTTPFault{}, pluginStage)}, nil
}

// ProcessRoute will add the desired fault parameters on each given route.
// There is no higher level configuration of the fault filter so this is where
// actual functional configuration takes place.
func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	markFilterConfigFunc := func(spec *v1.Destination) (proto.Message, error) {
		if in.GetOptions() == nil {
			return nil, nil
		}
		routeFaults := in.GetOptions().GetFaults()
		if routeFaults == nil {
			return nil, nil
		}
		routeAbort := routeFaults.GetAbort()
		routeDelay := routeFaults.GetDelay()
		envoyAbort, err := toEnvoyAbort(routeAbort)
		if err != nil {
			return nil, err
		}
		envoyDelay, err := toEnvoyDelay(routeDelay)
		if err != nil {
			return nil, err
		}

		// neither were configured on the route so return without error
		if envoyAbort == nil && envoyDelay == nil {
			return nil, nil
		}

		// mark configured and return the wrapped envoy configuration
		p.filterRequiredForListener[params.HttpListener] = struct{}{}
		return &envoyhttpfault.HTTPFault{
			Abort: envoyAbort,
			Delay: envoyDelay,
		}, nil
	}
	return pluginutils.MarkPerFilterConfig(params.Ctx, params.Snapshot, in, out, wellknown.Fault, markFilterConfigFunc)
}

// toEnvoyAbort converts the abort config from the gloo api to the envoy api.
// Will error if there is config present but it is invalid.
func toEnvoyAbort(abort *fault.RouteAbort) (*envoyhttpfault.FaultAbort, error) {
	if abort == nil {
		return nil, nil
	}
	// Validation really should catch this at proto level but sometimes things can sneak by
	// https://github.com/envoyproxy/envoy/blob/bc8f0cd19f991a56269f1ea30b5b8d8d331da0dc/api/envoy/config/filter/http/fault/v2/fault.proto#L39
	if abort.GetHttpStatus() >= 600 || abort.GetHttpStatus() < 200 {
		return nil, errors.Errorf("invalid abort status code '%v', must be in range of [200,600)", abort.GetHttpStatus())
	}
	percentage := common.ToEnvoyPercentage(abort.GetPercentage())
	errorType := &envoyhttpfault.FaultAbort_HttpStatus{
		HttpStatus: uint32(abort.GetHttpStatus()),
	}
	return &envoyhttpfault.FaultAbort{
		Percentage: percentage,
		ErrorType:  errorType,
	}, nil
}

// toEnvoyDelay converts the delay config from the gloo api to the envoy api.
// Will error if there is config present but it is invalid.
func toEnvoyDelay(delay *fault.RouteDelay) (*envoyfault.FaultDelay, error) {
	if delay == nil {
		return nil, nil
	}
	// Validation really should catch this at proto level but sometimes things can sneak by
	// https://github.com/envoyproxy/envoy/blob/bc8f0cd19f991a56269f1ea30b5b8d8d331da0dc/api/envoy/extensions/filters/common/fault/v3/fault.proto#L53
	if delay.GetFixedDelay().GetSeconds() <= 0 {
		return nil, errors.Errorf("invalid delay duration '%v', must be greater than 0", delay.GetFixedDelay())
	}
	percentage := common.ToEnvoyPercentage(delay.GetPercentage())
	delaySpec := &envoyfault.FaultDelay_FixedDelay{
		FixedDelay: delay.GetFixedDelay(),
	}
	return &envoyfault.FaultDelay{
		Percentage:         percentage,
		FaultDelaySecifier: delaySpec,
	}, nil
}
