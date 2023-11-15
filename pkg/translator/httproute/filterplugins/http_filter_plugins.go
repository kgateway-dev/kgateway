package filterplugins

import (
	"context"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type RouteContext struct {
	Ctx      context.Context
	Route    *gwv1.HTTPRoute
	Rule     *gwv1.HTTPRouteRule
	Match    *gwv1.HTTPRouteMatch
	Queries  query.GatewayQueries
	Reporter reports.ParentRefReporter
}

type FilterPlugin interface {
	// outputRoute.Options is guaranteed to be non-nil
	ApplyFilter(
		ctx *RouteContext,
		filter gwv1.HTTPRouteFilter,
		outputRoute *routev3.Route,
	) error
}
