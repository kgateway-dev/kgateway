package utils

import (
	"errors"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
)

var (
	InvalidRouteActionError = errors.New("cannot use this plugin on non-Route_Route route actions")
)

func EnsureRouteAction(out *envoy_config_route_v3.Route) error {
	if out.GetAction() != nil && out.GetRoute() == nil {
		return InvalidRouteActionError
	}
	// we have already ensured that the output route action is either nil or of the proper type
	// if it is nil, we initialize it prior to transforming it
	if out.GetRoute() == nil {
		out.Action = &envoy_config_route_v3.Route_Route{
			Route: &envoy_config_route_v3.RouteAction{},
		}
	}
	return nil
}
