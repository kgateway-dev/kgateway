package filterplugins

import (
	"context"

	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
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
		outputRoute *v1.Route,
	) error
}
