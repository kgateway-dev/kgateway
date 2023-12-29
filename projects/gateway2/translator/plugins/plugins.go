package plugins

import (
	"context"

	"github.com/solo-io/gloo/projects/gateway2/reports"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type RouteContext struct {
	// top-level HTTPRoute
	Route *gwv1.HTTPRoute
	// specific Rule of the HTTPRoute being processed
	Rule *gwv1.HTTPRouteRule
	// specific Match of the Rule being processed (as there may be multiple Matches per Rule)
	Match *gwv1.HTTPRouteMatch
	// Reporter for the correct ParentRef associated with this HTTPRoute
	Reporter reports.ParentRefReporter
}

type RoutePlugin interface {
	// called for each Match in a given Rule
	ApplyPlugin(
		ctx context.Context,
		routeCtx *RouteContext,
		outputRoute *v1.Route,
	) error
}
