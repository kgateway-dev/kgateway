package query

import (
	"container/list"
	"context"

	"github.com/rotisserie/eris"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/utils"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

var routeOptionGK = schema.GroupKind{
	Group: sologatewayv1.RouteOptionGVK.Group,
	Kind:  sologatewayv1.RouteOptionGVK.Kind,
}

type RouteOptionQueries interface {
	// Gets the RouteOption attached to the provided HTTPRoute.
	// If multiple RouteOptions are attached to the route, it returns the earliest created resource.
	// Note that currently, only RouteOptions in the same namespace as the HTTPRoute can be attached.
	//
	// When the given route is a delegatee/child route in a delegation tree, the RouteOption of its ancestors
	// are recursively merged in priority order along the given delegation chain, along with any RouteOption
	// that may be attached to the route itself. The creation timestamp is only considered if multiple RouteOptions
	// attach to the same route for a route in the delegation chain, in which case the earliest created resource is
	// picked for the merging process.
	GetRouteOptionForRouteRule(
		ctx context.Context,
		route types.NamespacedName,
		rule *gwv1.HTTPRouteRule,
		parentRef *list.Element,
		gwQueries gwquery.GatewayQueries,
	) (*solokubev1.RouteOption, bool, error)
}

type routeOptionQueries struct {
	c client.Client
}

func NewQuery(c client.Client) RouteOptionQueries {
	return &routeOptionQueries{c}
}

func (r *routeOptionQueries) GetRouteOptionForRouteRule(
	ctx context.Context,
	route types.NamespacedName,
	rule *gwv1.HTTPRouteRule,
	parentRef *list.Element,
	gwQueries gwquery.GatewayQueries,
) (*solokubev1.RouteOption, bool, error) {
	var err error
	var mergedOpt *solokubev1.RouteOption

	if parentRef != nil {
		delegationCtx, ok := parentRef.Value.(plugins.DelegationCtx)
		if !ok {
			return nil, false, eris.Errorf("invalid type %T in delegation chain, expected DelegationCtx", parentRef.Value)
		}
		mergedOpt, _, err = r.GetRouteOptionForRouteRule(
			ctx, delegationCtx.Ref, delegationCtx.Rule, parentRef.Next(), gwQueries)
	}

	routeOpt, filterOverride, err := r.getPreferredRouteOption(ctx, route, rule, gwQueries)
	if err != nil {
		return nil, false, err
	}
	if mergedOpt == nil {
		return routeOpt, filterOverride, nil
	}

	// merge scoped RouteOptions with parent giving preference to the parent
	if routeOpt == nil {
		routeOpt = new(solokubev1.RouteOption)
	}
	glooutils.ShallowMergeRouteOptions(mergedOpt.Spec.GetOptions(), routeOpt.Spec.GetOptions())

	return mergedOpt, filterOverride, nil
}

// getPreferredRouteOption returns the most preferred RouteOption for the given route
// when multiple RouteOptions are attached to the route by returning the earliest created RouteOption.
func (r *routeOptionQueries) getPreferredRouteOption(
	ctx context.Context,
	route types.NamespacedName,
	rule *gwv1.HTTPRouteRule,
	gwQueries gwquery.GatewayQueries,
) (*solokubev1.RouteOption, bool, error) {
	merged, err := lookupFilterOverride(ctx, route, rule, gwQueries)
	if err != nil {
		return nil, false, err
	}

	var list solokubev1.RouteOptionList
	if err := r.c.List(
		ctx,
		&list,
		client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector(RouteOptionTargetField, route.String())},
		client.InNamespace(route.Namespace),
	); err != nil {
		return nil, false, err
	}

	if len(list.Items) == 0 {
		return merged, false, nil
	}

	out := make([]*solokubev1.RouteOption, len(list.Items))
	for i := range list.Items {
		out[i] = &list.Items[i]
	}
	utils.SortByCreationTime(out)
	attached := out[0]

	if merged == nil {
		return attached, false, nil
	}

	glooutils.ShallowMergeRouteOptions(merged.Spec.GetOptions(), attached.Spec.GetOptions())

	return merged, true, nil
}

func lookupFilterOverride(
	ctx context.Context,
	route types.NamespacedName,
	rule *gwv1.HTTPRouteRule,
	gwQueries gwquery.GatewayQueries,
) (*solokubev1.RouteOption, error) {
	if rule == nil {
		return nil, nil
	}

	filter := utils.FindExtensionRefFilter(rule, routeOptionGK)
	if filter == nil {
		return nil, nil
	}

	extLookup := extensionRefLookup{namespace: route.Namespace}
	routeOption := &solokubev1.RouteOption{}
	err := utils.GetExtensionRefObjFrom(ctx, extLookup, gwQueries, filter.ExtensionRef, routeOption)

	// If the filter is not found, report a specific error so that it can reflect more
	// clearly on the status of the HTTPRoute.
	if err != nil && apierrors.IsNotFound(err) {
		return nil, errFilterNotFound(route.Namespace, filter)
	}

	return routeOption, err
}

type extensionRefLookup struct {
	namespace string
}

func (e extensionRefLookup) GroupKind() (metav1.GroupKind, error) {
	return metav1.GroupKind{
		Group: routeOptionGK.Group,
		Kind:  routeOptionGK.Kind,
	}, nil
}

func (e extensionRefLookup) Namespace() string {
	return e.namespace
}

func errFilterNotFound(namespace string, filter *gwv1.HTTPRouteFilter) error {
	return eris.Errorf(
		"extensionRef '%s' of type %s.%s in namespace '%s' not found",
		filter.ExtensionRef.Group,
		filter.ExtensionRef.Kind,
		filter.ExtensionRef.Name,
		namespace,
	)
}
