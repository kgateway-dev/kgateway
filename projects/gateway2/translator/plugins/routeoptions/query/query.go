package query

import (
	"context"

	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type RouteOptionQueries interface {
	// Populates the provided RouteOptionList with the RouteOption resources attached to the provided HTTPRoute.
	// Note that currently, only RouteOptions in the same namespace as the HTTPRoute can be attached.
	GetRouteOptionsForRoute(ctx context.Context, route *gwv1.HTTPRoute, list *solokubev1.RouteOptionList) error
}

type routeOptionQueries struct {
	c client.Client
}

func NewQuery(c client.Client) RouteOptionQueries {
	return &routeOptionQueries{c}
}

func (r *routeOptionQueries) GetRouteOptionsForRoute(ctx context.Context, route *gwv1.HTTPRoute, list *solokubev1.RouteOptionList) error {
	nn := types.NamespacedName{
		Namespace: route.Namespace,
		Name:      route.Name,
	}
	return r.c.List(
		ctx,
		list,
		client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector(RouteOptionTargetField, nn.String())},
		client.InNamespace(route.GetNamespace()),
	)
}
