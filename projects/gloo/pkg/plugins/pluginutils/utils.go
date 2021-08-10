package pluginutils

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

func getRouteActions(
	in *v1.Route,
	out *envoy_config_route_v3.Route,
) (*v1.RouteAction, *envoy_config_route_v3.RouteAction, error) {
	inRouteAction, ok := in.GetAction().(*v1.Route_RouteAction)
	if !ok {
		return nil, nil, errors.Errorf("input action was not a RouteAction")
	}
	inAction := inRouteAction.RouteAction

	outRouteAction, ok := out.GetAction().(*envoy_config_route_v3.Route_Route)
	if !ok {
		return nil, nil, errors.Errorf("output action was not a RouteAction")
	}
	outAction := outRouteAction.Route
	return inAction, outAction, nil
}
