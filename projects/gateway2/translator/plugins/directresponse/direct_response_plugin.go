package directresponse

import (
	"context"
	"fmt"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

type plugin struct {
	gwQueries query.GatewayQueries
}

func NewPlugin(gwQueries query.GatewayQueries) *plugin {
	return &plugin{
		gwQueries: gwQueries,
	}
}

var _ plugins.RoutePlugin = &plugin{}

func (p *plugin) ApplyRoutePlugin(
	ctx context.Context,
	routeCtx *plugins.RouteContext,
	outputRoute *v1.Route,
) error {
	// determine whether there are any direct response routes that should be
	// applied to the current route. otherwise, we'll return early.
	drr, err := findDirectResponseExtension(ctx, routeCtx, p.gwQueries)
	if err != nil {
		outputRoute.Action = ErrorResponseAction()
		routeCtx.Reporter.SetCondition(reports.HTTPRouteCondition{
			Type:    gwv1.RouteConditionResolvedRefs,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteReasonBackendNotFound,
			Message: fmt.Sprintf("Error while resolving DirectResponseRoute extensionRef: %v", err),
		})
		return err
	}
	if drr == nil {
		// exit early, no DRRs were found in the extension refs.
		return nil
	}

	// at this point, we have a valid DRR reference that we should apply to the route.
	if outputRoute.GetAction() != nil {
		// the output route already has an action, which is incompatible with the DirectResponseRoute,
		// so we'll return an error. note: the direct response plugin runs after other route plugins
		// that modify the output route (e.g. the redirect plugin), so this should be a rare case.
		errMsg := fmt.Sprintf("DirectResponseRoute cannot be applied to route with existing action: %T", outputRoute.GetAction())
		routeCtx.Reporter.SetCondition(reports.HTTPRouteCondition{
			Type:    gwv1.RouteConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteReasonIncompatibleFilters,
			Message: errMsg,
		})
		outputRoute.Action = ErrorResponseAction()
		return fmt.Errorf(errMsg)
	}

	outputRoute.Action = &v1.Route_DirectResponseAction{
		DirectResponseAction: &v1.DirectResponseAction{
			Status: drr.GetStatus(),
			Body:   drr.GetBody(),
		},
	}

	return nil
}

// findDirectResponseExtension searches for any extension ref filters on the current route ctx
// and returns the first DirectResponseRoute that matches the extension ref. In the case that
// multiple DRRs are found, an error is returned. If no DRRs are found, nil is returned.
func findDirectResponseExtension(
	ctx context.Context,
	routeCtx *plugins.RouteContext,
	queries query.GatewayQueries,
) (*v1alpha1.DirectResponseRoute, error) {
	// search for any extension ref filters on the current route ctx.
	filters := utils.FindExtensionRefFilters(routeCtx.Rule, v1alpha1.DirectResponseRouteGVK.GroupKind())
	if len(filters) == 0 {
		// no extension ref filters were found on the route.
		return nil, nil
	}

	var (
		errors []error
		drrs   []*v1alpha1.DirectResponseRoute
	)
	for _, filter := range filters {
		drr, err := utils.GetExtensionRefObj[*v1alpha1.DirectResponseRoute](ctx, routeCtx.Route, queries, filter.ExtensionRef)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		drrs = append(drrs, drr)
	}
	if len(errors) > 0 {
		return nil, fmt.Errorf("failed to resolve the DirectResponseRoute extension refs: %v", utilerrors.NewAggregate(errors))
	}

	switch len(drrs) {
	case 0:
		// no DRRs were found in the extension refs. nothing to do.
		return nil, nil
	case 1:
		// we found a single DRR, which we'll return.
		return drrs[0], nil
	default:
		// we don't support multiple DRRs on a single route.
		return nil, fmt.Errorf("multiple DirectResponseRoute resources found in extension refs. expected 1, found %d", len(drrs))
	}
}

// ErrorResponseAction returns a direct response action with a 500 status code.
// This is primarily used when an error occurs while translating the route.
// Exported for testing purposes.
func ErrorResponseAction() *v1.Route_DirectResponseAction {
	return &v1.Route_DirectResponseAction{
		DirectResponseAction: &v1.DirectResponseAction{
			Status: http.StatusInternalServerError,
		},
	}
}
