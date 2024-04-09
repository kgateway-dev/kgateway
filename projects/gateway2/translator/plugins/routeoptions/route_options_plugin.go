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
	"github.com/solo-io/gloo/projects/gateway2/translator/routeutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	xdsutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ plugins.RoutePlugin = &plugin{}
var _ plugins.StatusPlugin = &plugin{}

var routeOptionGK = schema.GroupKind{
	Group: sologatewayv1.RouteOptionGVK.Group,
	Kind:  sologatewayv1.RouteOptionGVK.Kind,
}

type PolicyReport struct {
	Ancestors          map[types.NamespacedName]*PolicyAncestorReport
	ObservedGeneration int64
}

type PolicyAncestorReport struct {
	Condition metav1.Condition
}

// RouteOption resource ->  PolicyReport
// PolictReport: AncestorRef(HTTPRoute) -> Actual condition
// NOTE: This assumes the only ancestor type of a RouteOption is HTTPRoute; if this changes, we need
// to track Group & Kind along with the types.NN
type policyStatusCache = map[types.NamespacedName]*PolicyReport

type legacyStatus struct {
	subresourceStatus map[string]*core.Status
	routeErrors       []*validation.RouteReport_Error
}
type legacyStatusCache = map[types.NamespacedName]legacyStatus

type plugin struct {
	gwQueries         gwquery.GatewayQueries
	rtOptQueries      rtoptquery.RouteOptionQueries
	policyStatusCache policyStatusCache
	legacyStatusCache legacyStatusCache
	routeOptionClient sologatewayv1.RouteOptionClient
	statusReporter    reporter.StatusReporter
}

func NewPlugin(
	gwQueries gwquery.GatewayQueries,
	client client.Client,
	routeOptionClient sologatewayv1.RouteOptionClient,
	statusReporter reporter.StatusReporter,
) *plugin {
	policyStatusCache := make(policyStatusCache)
	legacyStatusCache := make(legacyStatusCache)
	return &plugin{
		gwQueries,
		rtoptquery.NewQuery(client),
		policyStatusCache,
		legacyStatusCache,
		routeOptionClient,
		statusReporter,
	}
}

func (p *plugin) ApplyStatusPlugin(ctx context.Context, statusCtx *plugins.StatusContext) {
	// gather all RouteOptions we need to report status for
	for _, proxyWithReport := range statusCtx.ProxiesWithReports {
		// get proxy status to use for RouteOption status
		proxyStatus := p.statusReporter.StatusFromReport(proxyWithReport.Reports.ResourceReports[proxyWithReport.Proxy], nil)

		// get the route errors for this specific proxy
		routeErrors := extractRouteErrors(proxyWithReport.Reports.ProxyReport)
		for roKey, rerrs := range routeErrors {
			// set the subresource status for this proxy on the RO
			statusForRO := p.legacyStatusCache[roKey]
			thisSubresourceStatus := statusForRO.subresourceStatus
			thisSubresourceStatus[xds.SnapshotCacheKey(xdsutils.GlooGatewayTranslatorValue, proxyWithReport.Proxy)] = proxyStatus
			statusForRO.subresourceStatus = thisSubresourceStatus
			statusForRO.routeErrors = rerrs
			p.legacyStatusCache[roKey] = statusForRO
		}
	}
	routeOptionReport := make(reporter.ResourceReports)
	for k, v := range p.legacyStatusCache {
		// get the obj by namespacedName
		roObj, _ := p.routeOptionClient.Read(k.Namespace, k.Name, clients.ReadOpts{Ctx: ctx})

		// mark this object to be processed
		routeOptionReport.Accept(roObj)

		// add any route errors for this obj
		for _, rerr := range v.routeErrors {
			routeOptionReport.AddError(roObj, errors.New(rerr.GetReason()))
		}

		// actually write out the reports!
		p.statusReporter.WriteReports(ctx, routeOptionReport, v.subresourceStatus)
	}
}

func (p *plugin) setLegacyStatusAccepted(
	routeOption *solokubev1.RouteOption,
) {
	newStatus := legacyStatus{}
	newStatus.subresourceStatus = make(map[string]*core.Status)
	p.legacyStatusCache[client.ObjectKeyFromObject(routeOption)] = newStatus
}

func extractRouteErrors(proxyReport *validation.ProxyReport) map[types.NamespacedName][]*validation.RouteReport_Error {
	routeErrors := make(map[types.NamespacedName][]*validation.RouteReport_Error)
	for _, lr := range proxyReport.GetListenerReports() {
		for _, hlr := range lr.GetAggregateListenerReport().GetHttpListenerReports() {
			for _, vhr := range hlr.GetVirtualHostReports() {
				for _, rr := range vhr.GetRouteReports() {
					for _, rerr := range rr.GetErrors() {
						_, roKey := extractSourceKeys(rerr.GetMetadata())
						errors := routeErrors[roKey]
						errors = append(errors, rerr)
						routeErrors[roKey] = errors
					}
				}
			}
		}
	}
	return routeErrors
}

func (p *plugin) ApplyRoutePlugin(
	ctx context.Context,
	routeCtx *plugins.RouteContext,
	outputRoute *v1.Route,
) error {
	var routeOptions *v1.RouteOptions
	// check for RouteOptions applied to full Route
	routeOptions, routeOptionKube := p.handleAttachment(ctx, routeCtx)

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

		if filterRouteOptions == nil {
			// if we didn't use a filter to derive the RouteOptions for this v1.Route, let's track the
			// RouteOption policy object that was used so we can report status on it
			routeutils.AppendSourceToRoute(outputRoute, routeOptionKube)

			// report success on attaching this policy, although it may be overturned
			// later if the Proxy translation results in a processing error for these RouteOptions
			// we will track the ancestor as the targeted HTTPRoute; we may want change to Gateway later
			p.setPolicyStatusAccepted(routeOptionKube, routeCtx.Route)
			p.setLegacyStatusAccepted(routeOptionKube)
		}
	}
	return nil
}

func (p *plugin) handleAttachment(
	ctx context.Context,
	routeCtx *plugins.RouteContext,
) (*v1.RouteOptions, *solokubev1.RouteOption) {
	// TODO: This is far too naive and we should optimize the amount of querying we do.
	// Route plugins run on every match for every Rule in a Route but the attached options are
	// the same each time; i.e. HTTPRoute <-1:1-> RouteOptions.
	// We should only make this query once per HTTPRoute.
	attachedOptions := getAttachedRouteOptions(ctx, routeCtx.Route, p.rtOptQueries)
	if len(attachedOptions) == 0 {
		return nil, nil
	}

	// sort attached options and apply only the earliest
	utils.SortByCreationTime(attachedOptions)
	earliestOption := attachedOptions[0]

	if earliestOption.Spec.GetOptions() != nil {
		return earliestOption.Spec.GetOptions(), earliestOption
	}
	return nil, nil
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
			routeCtx.ParentRefReporter.SetCondition(reports.HTTPRouteCondition{
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
