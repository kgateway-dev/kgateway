package routeoptions

import (
	"context"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type plugin struct {
	queries query.GatewayQueries
}

func NewPlugin(queries query.GatewayQueries) *plugin {
	return &plugin{
		queries,
	}
}

func (p *plugin) ApplyRoutePlugin(
	ctx context.Context,
	routeCtx *plugins.RouteContext,
	outputRoute *v1.Route,
) error {
	gk := schema.GroupKind{
		Group: sologatewayv1.RouteOptionGVK.Group,
		Kind:  sologatewayv1.RouteOptionGVK.Kind,
	}
	// contextutils.LoggerFrom(ctx).Debugf("LAW: looking for RouteOption filter with gk: %+v", gk)
	filter := utils.FindExtensionRefFilter(routeCtx, gk)
	if filter == nil {
		return nil
	}

	// contextutils.LoggerFrom(ctx).Debugf("LAW: found RouteOptions filter: %+v", filter)
	routeOption := &solokubev1.RouteOption{}
	err := utils.GetExtensionRefObj(context.Background(), routeCtx, p.queries, filter.ExtensionRef, routeOption)
	if err != nil {
		return nil
	}
	if routeOption.Spec.Options != nil {
		// set options from RouteOptions resource and clobber any existing options
		// should be revisited if/when we support merging options from e.g. other HTTPRouteFilters
		outputRoute.Options = routeOption.Spec.Options
	}
	return nil
}
