package routeoptions

import (
	"context"
	"errors"
	"fmt"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	rtoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/routeoptions/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ plugins.RoutePlugin = &plugin{}

var routeOptionGK = schema.GroupKind{
	Group: sologatewayv1.RouteOptionGVK.Group,
	Kind:  sologatewayv1.RouteOptionGVK.Kind,
}

type plugin struct {
	gwQueries    gwquery.GatewayQueries
	rtOptQueries rtoptquery.RouteOptionQueries
}

func NewPlugin(gwQueries gwquery.GatewayQueries, client client.Client) *plugin {
	return &plugin{
		gwQueries,
		rtoptquery.NewQuery(client),
	}
}

func (p *plugin) ApplyRoutePlugin(
	ctx context.Context,
	routeCtx *plugins.RouteContext,
	outputRoute *v1.Route,
) error {
	// check for RouteOptions applied to full Route
	routeOptions := p.handleAttachment(ctx, routeCtx)

	// allow for ExtensionRef filter override
	filterRouteOptions, err := p.handleFilter(ctx, routeCtx)
	if err != nil {
		return err
	}
	if filterRouteOptions != nil {
		routeOptions = filterRouteOptions
	}

	if routeOptions != nil {
		// clobber the existing RouteOptions; merge semantics may be desired later
		outputRoute.Options = routeOptions
	}
	return nil
}

func (p *plugin) handleAttachment(
	ctx context.Context,
	routeCtx *plugins.RouteContext,
) *v1.RouteOptions {
	// TODO: This is far too naive and we should optimize the amount of querying we do.
	// Route plugins run on every match for every Rule in a Route but the attached options are
	// the same each time; i.e. HTTPRoute <-1:1-> RouteOptions.
	// We should only make this query once per HTTPRoute.

	attachedOptions := getAttachedRouteOptions(ctx, routeCtx.Route, p.rtOptQueries)
	if len(attachedOptions) == 0 {
		return nil
	}

	// sort attached options and apply only the earliest
	utils.SortByCreationTime(attachedOptions)
	earliestOption := attachedOptions[0]

	if earliestOption.Spec.GetOptions() != nil {
		return earliestOption.Spec.GetOptions()
	}
	return nil
}

func (p *plugin) handleFilter(
	ctx context.Context,
	routeCtx *plugins.RouteContext,
) (*v1.RouteOptions, error) {
	filter := utils.FindExtensionRefFilter(routeCtx, routeOptionGK)
	if filter == nil {
		return nil, nil
	}

	routeOption := &solokubev1.RouteOption{}
	err := utils.GetExtensionRefObj(context.Background(), routeCtx, p.gwQueries, filter.ExtensionRef, routeOption)
	if err != nil {
		switch {
		case apierrors.IsNotFound(err):
			notFoundMsg := formatNotFoundMessage(routeCtx, filter)
			routeCtx.Reporter.SetCondition(reports.HTTPRouteCondition{
				Type:    gwv1.RouteConditionResolvedRefs,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.RouteReasonBackendNotFound,
				Message: notFoundMsg,
			})
			return nil, errors.New(notFoundMsg)
		case errors.Is(err, utils.ErrTypesNotEqual):
		case errors.Is(err, utils.ErrNotSettable):
			devErr := fmt.Errorf("developer error while getting RouteOptions as ExtensionRef: %w", err)
			contextutils.LoggerFrom(ctx).DPanic(devErr)
			return nil, devErr
		default:
			return nil, fmt.Errorf("error while getting RouteOptions as ExtensionRef route filter: %w", err)
		}
	}

	if routeOption.Spec.GetOptions() != nil {
		return routeOption.Spec.GetOptions(), nil
	}
	return nil, nil
}

func getAttachedRouteOptions(ctx context.Context, route *gwv1.HTTPRoute, queries rtoptquery.RouteOptionQueries) []*solokubev1.RouteOption {
	var routeOptionList solokubev1.RouteOptionList
	err := queries.GetRouteOptionsForRoute(ctx, route, &routeOptionList)
	if err != nil {
		contextutils.LoggerFrom(ctx).Errorf("error while Listing RouteOptions: %v", err)
		// TODO: add status to policy on error
		return nil
	}

	// as the RouteOptionList does not contain pointers, and RouteOption is a concrete proto message,
	// we need to turn it into a pointer slice to avoid copying proto message state around, copying locks, etc.
	// while we perform operations on the RouteOptionList
	ptrSlice := []*solokubev1.RouteOption{}
	items := routeOptionList.Items
	for i := range items {
		ptrSlice = append(ptrSlice, &items[i])
	}
	return ptrSlice
}

func formatNotFoundMessage(routeCtx *plugins.RouteContext, filter *gwv1.HTTPRouteFilter) string {
	return fmt.Sprintf(
		"extensionRef '%s' of type %s.%s in namespace '%s' not found",
		filter.ExtensionRef.Group,
		filter.ExtensionRef.Kind,
		filter.ExtensionRef.Name,
		routeCtx.Route.GetNamespace(),
	)
}
