package directresponse

import (
	"context"
	"fmt"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/solo-io/gloo/projects/gateway2/api/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/utils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

type plugin struct {
	client.Client
}

func NewPlugin(c client.Client) *plugin {
	return &plugin{
		Client: c,
	}
}

var _ plugins.RoutePlugin = &plugin{}

func (p *plugin) ApplyRoutePlugin(
	ctx context.Context,
	routeCtx *plugins.RouteContext,
	outputRoute *v1.Route,
) error {
	// TODO(tim): Investigate whether this validation approach is consistent
	// with the status wellknown pattern in the upstream codebase. In particular,
	// the `RouteConditionPartiallyInvalid` condition has an intereting godoc comment.

	// determine whether there are any direct response routes that should be
	// applied to the current route. otherwise, we'll return early.
	match, err := findDirectResponseExtension(routeCtx)
	if err != nil {
		routeCtx.Reporter.SetCondition(reports.HTTPRouteCondition{
			Type:    gwv1.RouteConditionResolvedRefs,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteReasonBackendNotFound,
			Message: fmt.Sprintf("Error while resolving DirectResponseRoute extensionRef: %v", err),
		})
		outputRoute.Action = &v1.Route_DirectResponseAction{
			DirectResponseAction: &v1.DirectResponseAction{
				Status: http.StatusInternalServerError,
			},
		}
		return err
	}
	if match == nil {
		return nil
	}

	// find the direct response route that matches the extension ref on the route filter.
	// note: we don't support cross-namespace extension references, so we're always looking
	// for the DRR in the same namespace as the HTTPRoute.
	drr := &v1alpha1.DirectResponseRoute{}
	if err := p.Get(ctx, client.ObjectKey{
		Name:      string(match.ExtensionRef.Name),
		Namespace: routeCtx.Route.GetNamespace(),
	}, drr); err != nil {
		routeCtx.Reporter.SetCondition(reports.HTTPRouteCondition{
			Type:    gwv1.RouteConditionResolvedRefs,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.RouteReasonBackendNotFound,
			Message: fmt.Sprintf("No DirectResponseRoute resource matches the extensionRef specified on the HTTPRoute: %v", err),
		})
		outputRoute.Action = &v1.Route_DirectResponseAction{
			DirectResponseAction: &v1.DirectResponseAction{
				Status: http.StatusInternalServerError,
			},
		}
		return err
	}

	outputRoute.Action = &v1.Route_DirectResponseAction{
		DirectResponseAction: &v1.DirectResponseAction{
			Status: *drr.Spec.Status,
			Body:   *drr.Spec.Body,
		},
	}
	routeCtx.Reporter.SetCondition(reports.HTTPRouteCondition{
		Type:    gwv1.RouteConditionResolvedRefs,
		Status:  metav1.ConditionTrue,
		Reason:  gwv1.RouteReasonResolvedRefs,
		Message: "DirectResponseRoute successfully resolved",
	})

	return nil
}

// findDirectResponseExtension searches for any extension ref filters on the current route ctx
// and returns the first DirectResponseRoute that matches the extension ref. In the case that
// multiple DRRs are found, an error is returned. If no DRRs are found, nil is returned.
func findDirectResponseExtension(routeCtx *plugins.RouteContext) (*gwv1.HTTPRouteFilter, error) {
	// search for any extension ref filters on the current route ctx.
	filters := utils.FindAppliedRouteFilters(routeCtx, gwv1.HTTPRouteFilterExtensionRef)
	if len(filters) == 0 {
		return nil, nil
	}

	// we're now looking for any direct response routes that match the extension ref on the route filter.
	// TODO(tim): cache this relationship so we don't have to search for the DRR every time we apply the plugin.
	matches := make([]gwv1.HTTPRouteFilter, 0, len(filters))
	for _, filter := range filters {
		if filter.ExtensionRef.Group != v1alpha1.Group {
			continue
		}
		if filter.ExtensionRef.Kind != v1alpha1.DirectResponseRouteKind {
			continue
		}
		matches = append(matches, filter)
	}
	if len(matches) == 0 {
		// exit early, no DRRs were found in the extension refs.
		return nil, nil
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("multiple DirectResponseRoute resources found in extension refs. only one is allowed")
	}
	// else, return the first match we found.
	// TODO(tim): is this deterministic? do we need to sort the matches? AFAIK, I
	// know upstream doesn't have guidance on the order of filters in the HTTPRoute,
	// but I think mirroring Envoy's fitler chain semantics is a good idea.
	return &matches[0], nil
}
