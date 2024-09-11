package directresponse

import (
	"context"
	"fmt"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// search for any extension ref filters on the current route ctx.
	filters := utils.FindExtensionRefFilters(routeCtx.Rule, v1alpha1.DirectResponseGVK.GroupKind())
	if len(filters) == 0 {
		// no need to run the plugin if there are no DirectResponse extension refs.
		return nil
	}
	if len(filters) > 1 {
		// we don't support multiple extension ref filters on a single route.
		errMsg := fmt.Sprintf("multiple DirectResponse extension refs found. expected 1, found %d", len(filters))
		routeCtx.Reporter.SetCondition(reports.HTTPRouteCondition{
			Type:    gwv1.RouteConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteReasonIncompatibleFilters,
			Message: errMsg,
		})
		outputRoute.Action = ErrorResponseAction()
		return fmt.Errorf(errMsg)
	}

	// verify the DR reference is valid and get the DR object from the cluster.
	dr, err := utils.GetExtensionRefObj[*v1alpha1.DirectResponse](ctx, routeCtx.Route, p.gwQueries, filters[0].ExtensionRef)
	if err != nil {
		outputRoute.Action = ErrorResponseAction()
		routeCtx.Reporter.SetCondition(reports.HTTPRouteCondition{
			Type:    gwv1.RouteConditionResolvedRefs,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteReasonBackendNotFound,
			Message: err.Error(),
		})
		return err
	}

	// at this point, we have a valid DR reference that we should apply to the route.
	if outputRoute.GetAction() != nil {
		// the output route already has an action, which is incompatible with the DirectResponse,
		// so we'll return an error. note: the direct response plugin runs after other route plugins
		// that modify the output route (e.g. the redirect plugin), so this should be a rare case.
		errMsg := fmt.Sprintf("DirectResponse cannot be applied to route with existing action: %T", outputRoute.GetAction())
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
			Status: dr.GetStatusCode(),
			Body:   dr.GetBody(),
		},
	}

	return nil
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
