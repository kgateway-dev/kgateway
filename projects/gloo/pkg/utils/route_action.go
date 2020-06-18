package utils

import (
	"errors"

	envoyroute "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
)

var (
	InvalidRouteActionError = errors.New("cannot use this plugin on non-Route_Route route actions")
)

func EnsureRouteAction(out *envoyroute.Route) error {
	if out.Action != nil && out.GetRoute() == nil {
		return InvalidRouteActionError
	}
	// we have already ensured that the output route action is either nil or of the proper type
	// if it is nil, we initialize it prior to transforming it
	if out.GetRoute() == nil {
		out.Action = &envoyroute.Route_Route{
			Route: &envoyroute.RouteAction{},
		}
	}
	return nil
}
