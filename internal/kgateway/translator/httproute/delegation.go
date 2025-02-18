package httproute

import (
	"container/list"
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/contextutils"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

type DelegationCtx struct {
	Ref types.NamespacedName
}

// flattenDelegatedRoutes recursively translates a delegated route tree.
//
// It returns an error if it cannot determine the delegatee (child) routes or
// if it detects a cycle in the delegation tree.
// If the child route is invalid, it will be ignored and its Status will be updated accordingly.
func flattenDelegatedRoutes(
	ctx context.Context,
	parent *query.RouteInfo,
	backend ir.HttpBackendOrDelegate,
	parentReporter reports.ParentRefReporter,
	baseReporter reports.Reporter,
	gwListener gwv1.Listener,
	parentMatch gwv1.HTTPRouteMatch,
	outputs *[]ir.HttpRouteRuleMatchIR,
	routesVisited sets.Set[types.NamespacedName],
	delegationChain *list.List,
) error {
	parentRoute, ok := parent.Object.(*ir.HttpRouteIR)
	if !ok {
		return eris.Errorf("unsupported route type: %T", parent.Object)
	}
	parentRef := types.NamespacedName{Namespace: parentRoute.Namespace, Name: parentRoute.Name}
	routesVisited.Insert(parentRef)
	defer routesVisited.Delete(parentRef)

	delegationCtx := DelegationCtx{
		Ref: parentRef,
	}
	lRef := delegationChain.PushFront(delegationCtx)
	defer delegationChain.Remove(lRef)

	rawChildren, err := parent.GetChildrenForRef(*backend.Delegate)
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
			msg := fmt.Sprintf("ignoring unsupported child route type %T for parent httproute %v", child.Object, parentRef)
			contextutils.LoggerFrom(ctx).Warn(msg)
			continue
		}
		childRef := types.NamespacedName{Namespace: childRoute.Namespace, Name: childRoute.Name}
		if routesVisited.Has(childRef) {
			// Loop detected, ignore child route
			// This is an _extra_ safety check, but the given HTTPRouteInfo shouldn't ever contain cycles.
			msg := fmt.Sprintf("cyclic reference detected while evaluating delegated routes for parent: %s; child route %s will be ignored",
				parentRef, childRef)
			contextutils.LoggerFrom(ctx).Warn(msg)
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
			ctx, gwListener, child, reporter, baseReporter, outputs, routesVisited, hostnames, delegationChain)
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
