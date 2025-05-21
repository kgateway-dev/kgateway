package httproute

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/rotisserie/eris"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	reports "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

// flattenDelegatedRoutes recursively translates a delegated route tree.
//
// It returns an error if it cannot determine the delegatee (child) routes.
//
// In the following cases, a child route will be ignored/dropped, and its Status updated with the reason:
// - If the child route is invalid (right now we only validate that it doesn't specify hostnames)
// - If there is a cycle in the delegation tree
func flattenDelegatedRoutes(
	ctx context.Context,
	parentInfo *query.RouteInfo,
	backend ir.HttpBackendOrDelegate,
	parentReporter reports.ParentRefReporter,
	baseReporter reports.Reporter,
	parentMatch gwv1.HTTPRouteMatch,
	outputs *[]ir.HttpRouteRuleMatchIR,
	routesVisited sets.Set[types.NamespacedName],
	delegatingParent *ir.HttpRouteRuleMatchIR,
) error {
	parentRoute, ok := parentInfo.Object.(*ir.HttpRouteIR)
	if !ok {
		return eris.Errorf("unsupported route type: %T", parentInfo.Object)
	}
	parentRef := types.NamespacedName{Namespace: parentRoute.Namespace, Name: parentRoute.Name}
	routesVisited.Insert(parentRef)
	defer routesVisited.Delete(parentRef)

	rawChildren, err := parentInfo.GetChildrenForRef(*backend.Delegate)
	if len(rawChildren) == 0 || err != nil {
		if err == nil {
			err = eris.Errorf("unresolved reference %s", backend.Delegate.ResourceName())
		}
		return err
	}
	children := filterDelegatedChildren(parentRef, parentMatch, rawChildren)

	// Child routes inherit the hostnames from the parent route
	hostnames := make([]string, len(parentRoute.Hostnames))
	copy(hostnames, parentRoute.Hostnames)

	// For these child routes, recursively flatten them
	for _, child := range children {
		childRoute, ok := child.Object.(*ir.HttpRouteIR)
		if !ok {
			slog.Warn("ignoring unsupported child route type",
				"route_type", fmt.Sprintf("%T", child.Object), "parent_resource_ref", parentRef)
			continue
		}
		childRef := types.NamespacedName{Namespace: childRoute.Namespace, Name: childRoute.Name}
		if routesVisited.Has(childRef) {
			// Loop detected, ignore child route
			// This is an _extra_ safety check, but the given HTTPRouteInfo shouldn't ever contain cycles.
			msg := fmt.Sprintf("cyclic reference detected while evaluating delegated routes for parent: %s; child route %s will be ignored",
				parentRef, childRef)
			slog.Warn(msg) //nolint:sloglint // ignore formatting
			parentReporter.SetCondition(reports.RouteCondition{
				Type:    gwv1.RouteConditionResolvedRefs,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.RouteReasonRefNotPermitted,
				Message: msg,
			})
			continue
		}

		// Create a new reporter for the child route
		reporter := baseReporter.Route(childRoute.GetSourceObject()).ParentRef(&gwv1.ParentReference{
			Group:     ptr.To(gwv1.Group(wellknown.GatewayGroup)),
			Kind:      ptr.To(gwv1.Kind(wellknown.HTTPRouteKind)),
			Name:      gwv1.ObjectName(parentRef.Name),
			Namespace: ptr.To(gwv1.Namespace(parentRef.Namespace)),
		})

		if err := validateChildRoute(*childRoute); err != nil {
			reporter.SetCondition(reports.RouteCondition{
				Type:    gwv1.RouteConditionAccepted,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.RouteReasonUnsupportedValue,
				Message: err.Error(),
			})
			continue
		}

		translateGatewayHTTPRouteRulesUtil(
			ctx, child, reporter, baseReporter, outputs, routesVisited, delegatingParent)
	}

	return nil
}

func validateChildRoute(
	route ir.HttpRouteIR,
) error {
	if len(route.Hostnames) > 0 {
		return errors.New("spec.hostnames must be unset on a delegatee route as they are inherited from the parent route")
	}
	return nil
}
