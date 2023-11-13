package mirror

import (
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/shadowing"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type Plugin struct{}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) ApplyFilter(
	ctx *filterplugins.RouteContext,
	filter gwv1.HTTPRouteFilter,
	outputRoute *v1.Route,
) error {
	config := filter.RequestMirror
	if config == nil {
		return errors.Errorf("RequestMirror filter supplied does not define requestMirror config")
	}

	routeAction := outputRoute.GetAction()
	if routeAction == nil {
		return errors.Errorf("RequestMirror must have destinations")
	}

	obj, err := ctx.Queries.GetBackendForRef(ctx.Ctx, ctx.Queries.ObjToFrom(ctx.Route), &config.BackendRef)
	clusterName := query.ProcessBackendRef(
		obj,
		err,
		ctx.Reporter,
		config.BackendRef,
	)
	if clusterName == nil {
		return nil //TODO https://github.com/solo-io/gloo/pull/8890/files#r1391523183
	}

	outputRoute.Options.Shadowing = &shadowing.RouteShadowing{
		Upstream: &core.ResourceRef{
			Name:      *clusterName,
			Namespace: obj.GetNamespace(),
		},
		Percentage: 100.0,
	}

	return nil
}
