package timeout

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/errors"
)

type Plugin struct{}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *Plugin) ProcessRoute(params plugins.Params, in *v1.Route, out *envoyroute.Route) error {
	if in.RoutePlugins == nil {
		return nil
	}
	if in.RoutePlugins.Timeout == nil {
		return nil
	}
	routeAction, ok := out.Action.(*envoyroute.Route_Route)
	if !ok {
		return errors.Errorf("timeout is only available for Route Actions")
	}
	if routeAction.Route == nil {
		return errors.Errorf("internal error: route %v specified a prefix, but output Envoy object "+
			"had nil route", in.Action)
	}

	timeout, err := types.DurationFromProto(in.RoutePlugins.Timeout)
	if err != nil {
		return errors.Wrapf(err, "failed to parse routeplugins.timeout")
	}

	routeAction.Route.Timeout = &timeout
	return nil
}
