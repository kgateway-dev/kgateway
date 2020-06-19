package faultinjection

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoycommonfault "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/common/fault/v3"
	envoyhttpfault "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/fault/v3"
	"github.com/gogo/protobuf/proto"
	"github.com/solo-io/gloo/pkg/utils/gogoutils"
	fault "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/faultinjection"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/internal/common"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
)

const (
	FilterName = "envoy.fault"
)

var pluginStage = plugins.DuringStage(plugins.FaultStage)

type Plugin struct {
}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	// put the filter in the chain, but the actual faults will be configured on the routes
	return []plugins.StagedHttpFilter{
		pluginutils.NewStagedFilter(FilterName, pluginStage),
	}, nil
}

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoyroute.Route) error {
	markFilterConfigFunc := func(spec *v1.Destination) (proto.Message, error) {
		if in.Options == nil {
			return nil, nil
		}
		routeFaults := in.GetOptions().GetFaults()
		if routeFaults == nil {
			return nil, nil
		}
		routeAbort := routeFaults.GetAbort()
		routeDelay := routeFaults.GetDelay()
		if routeAbort == nil && routeDelay == nil {
			return nil, nil
		}
		return generateEnvoyConfigForHttpFault(routeAbort, routeDelay), nil
	}
	return pluginutils.MarkPerFilterConfig(params.Ctx, params.Snapshot, in, out, FilterName, markFilterConfigFunc)
}

func toEnvoyAbort(abort *fault.RouteAbort) *envoyhttpfault.FaultAbort {
	if abort == nil {
		return nil
	}
	percentage := common.ToEnvoyPercentage(abort.Percentage)
	errorType := &envoyhttpfault.FaultAbort_HttpStatus{
		HttpStatus: uint32(abort.HttpStatus),
	}
	return &envoyhttpfault.FaultAbort{
		Percentage: percentage,
		ErrorType:  errorType,
	}
}

func toEnvoyDelay(delay *fault.RouteDelay) *envoycommonfault.FaultDelay {
	if delay == nil {
		return nil
	}
	percentage := common.ToEnvoyPercentage(delay.Percentage)
	delaySpec := &envoycommonfault.FaultDelay_FixedDelay{
		FixedDelay: gogoutils.DurationStdToProto(delay.FixedDelay),
	}
	return &envoycommonfault.FaultDelay{
		Percentage:         percentage,
		FaultDelaySecifier: delaySpec,
	}
}

func generateEnvoyConfigForHttpFault(routeAbort *fault.RouteAbort, routeDelay *fault.RouteDelay) *envoyhttpfault.HTTPFault {
	abort := toEnvoyAbort(routeAbort)
	delay := toEnvoyDelay(routeDelay)
	return &envoyhttpfault.HTTPFault{
		Abort: abort,
		Delay: delay,
	}
}
