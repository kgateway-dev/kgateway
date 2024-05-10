package query

import (
	"container/list"
	"context"

	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/utils"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"

	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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
	GetRouteOptionForRoute(ctx context.Context, route types.NamespacedName, delegationChain *list.Element) (*solokubev1.RouteOption, error)
}

type routeOptionQueries struct {
	c client.Client
}

func NewQuery(c client.Client) RouteOptionQueries {
	return &routeOptionQueries{c}
}

func (r *routeOptionQueries) GetRouteOptionForRoute(
	ctx context.Context,
	route types.NamespacedName,
	parentRef *list.Element,
) (*solokubev1.RouteOption, error) {
	var err error
	var mergedOpt *solokubev1.RouteOption
	if parentRef != nil {
		mergedOpt, err = r.GetRouteOptionForRoute(ctx, parentRef.Value.(types.NamespacedName), parentRef.Next())
	}

	routeOpt, err := r.getPreferredRouteOption(ctx, route)
	if err != nil {
		return nil, err
	}
	if mergedOpt == nil {
		return routeOpt, nil
	}

	// merge scoped RouteOptions with parent giving preference to the parent
	if routeOpt == nil {
		routeOpt = new(solokubev1.RouteOption)
	}
	glooutils.ShallowMergeRouteOptions(mergedOpt.Spec.GetOptions(), routeOpt.Spec.GetOptions())

	return mergedOpt, nil
}

// getPreferredRouteOption returns the most preferred RouteOption for the given route
// when multiple RouteOptions are attached to the route by returning the earliest created RouteOption.
func (r *routeOptionQueries) getPreferredRouteOption(
	ctx context.Context,
	route types.NamespacedName,
) (*solokubev1.RouteOption, error) {
	var list solokubev1.RouteOptionList
	if err := r.c.List(
		ctx,
		&list,
		client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector(RouteOptionTargetField, route.String())},
		client.InNamespace(route.Namespace),
	); err != nil {
		return nil, err
	}

	if len(list.Items) == 0 {
		return nil, nil
	}

	out := make([]*solokubev1.RouteOption, len(list.Items))
	for i := range list.Items {
		out[i] = &list.Items[i]
	}
	utils.SortByCreationTime(out)

	return out[0], nil
}
