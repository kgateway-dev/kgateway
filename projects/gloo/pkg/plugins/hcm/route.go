package hcm

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

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
