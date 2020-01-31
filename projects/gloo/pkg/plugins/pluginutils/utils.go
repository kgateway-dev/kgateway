package pluginutils

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"

	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

func getRouteActions(in *v1.Route, out *envoyroute.Route) (*v1.RouteAction, *envoyroute.RouteAction, error) {
	inRouteAction, ok := in.Action.(*v1.Route_RouteAction)
	if !ok {
		return nil, nil, errors.Errorf("input action was not a RouteAction")
	}
	inAction := inRouteAction.RouteAction

	outRouteAction, ok := out.Action.(*envoyroute.Route_Route)
	if !ok {
		return nil, nil, errors.Errorf("output action was not a RouteAction")
	}
	outAction := outRouteAction.Route
	return inAction, outAction, nil
}
