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
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
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

// holds the data structures needed to derive and report a classic GE status
type legacyStatus struct {
	// maps proxyName -> proxyStatus
	subresourceStatus map[string]*core.Status
	// *All* of the route errors encountered during processing for gloov1.Routes which receive their
	// options for this RouteOption
	routeErrors []*validation.RouteReport_Error
}

// holds status structure for each RouteOption we have processed and attached
// this is used because a RouteOption is attached to a Route, but a Route may be
// attached to multiple Gateways/Listeners, so we need a single status object
// to contain the subresourceStatus for each Proxy it was translated too, but also
// all the errors specifically encountered
type legacyStatusCache = map[types.NamespacedName]legacyStatus

type plugin struct {
	gwQueries         gwquery.GatewayQueries
	rtOptQueries      rtoptquery.RouteOptionQueries
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
	legacyStatusCache := make(legacyStatusCache)
	return &plugin{
		gwQueries:         gwQueries,
		rtOptQueries:      rtoptquery.NewQuery(client),
		legacyStatusCache: legacyStatusCache,
		routeOptionClient: routeOptionClient,
		statusReporter:    statusReporter,
	}
}

func (p *plugin) ApplyRoutePlugin(
	ctx context.Context,
	routeCtx *plugins.RouteContext,
	outputRoute *gloov1.Route,
) error {
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

			// track that we used this RouteOption is our status cache
			// we do this so we can persist status later for all attached RouteOptions
			p.trackAcceptedRouteOption(routeOptionKube)
		}
	}
	return nil
}

func (p *plugin) ApplyStatusPlugin(ctx context.Context, statusCtx *plugins.StatusContext) error {
	// gather all RouteOptions we need to report status for
	for _, proxyWithReport := range statusCtx.ProxiesWithReports {
		// get proxy status to use for RouteOption status
		proxyStatus := p.statusReporter.StatusFromReport(proxyWithReport.Reports.ResourceReports[proxyWithReport.Proxy], nil)

		// for this specific proxy, get all the route errors and their associated RouteOption sources
		routeErrors := extractRouteErrors(proxyWithReport.Reports.ProxyReport)
		for roKey, rerrs := range routeErrors {
			// grab the existing status object for this RouteOption
			statusForRO, ok := p.legacyStatusCache[roKey]
			if !ok {
				// we are processing an error that has a RouteOption source that we hadn't encountered until now
				// this shouldn't happen
				contextutils.LoggerFrom(ctx).DPanic("while trying to apply status for RouteOptions, we found a Route error sourced by an unknown RouteOption", "RouteOption", roKey)
			}

			// set the subresource status for this specific proxy on the RO
			thisSubresourceStatus := statusForRO.subresourceStatus
			thisSubresourceStatus[xds.SnapshotCacheKey(proxyWithReport.Proxy)] = proxyStatus
			statusForRO.subresourceStatus = thisSubresourceStatus

			// add any routeErrors from this Proxy translation
			statusForRO.routeErrors = append(statusForRO.routeErrors, rerrs...)

			// update the cache
			p.legacyStatusCache[roKey] = statusForRO
		}
	}
	routeOptionReport := make(reporter.ResourceReports)
	for roKey, status := range p.legacyStatusCache {
		// get the obj by namespacedName
		roObj, _ := p.routeOptionClient.Read(roKey.Namespace, roKey.Name, clients.ReadOpts{Ctx: ctx})

		// mark this object to be processed
		routeOptionReport.Accept(roObj)

		// add any route errors for this obj
		for _, rerr := range status.routeErrors {
			routeOptionReport.AddError(roObj, errors.New(rerr.GetReason()))
		}

		// actually write out the reports!
		err := p.statusReporter.WriteReports(ctx, routeOptionReport, status.subresourceStatus)
		if err != nil {
			return fmt.Errorf("error writing status report from RouteOptionPlugin: %w", err)
		}
	}
	return nil
}

// tracks the attachment of a RouteOption so we know which RouteOptions to report status for
func (p *plugin) trackAcceptedRouteOption(
	routeOption *solokubev1.RouteOption,
) {
	newStatus := legacyStatus{}
	newStatus.subresourceStatus = make(map[string]*core.Status)
	p.legacyStatusCache[client.ObjectKeyFromObject(routeOption)] = newStatus
}

func (p *plugin) handleAttachment(
	ctx context.Context,
	routeCtx *plugins.RouteContext,
) (*gloov1.RouteOptions, *solokubev1.RouteOption) {
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
) (*gloov1.RouteOptions, error) {
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

// given a ProxyReport, extract and aggregate all Route errors that have RouteOption source metadata
// and key them by the source RouteOption NamespacedName
func extractRouteErrors(proxyReport *validation.ProxyReport) map[types.NamespacedName][]*validation.RouteReport_Error {
	routeErrors := make(map[types.NamespacedName][]*validation.RouteReport_Error)
	routeReports := getAllRouteReports(proxyReport.GetListenerReports())
	for _, rr := range routeReports {
		for _, rerr := range rr.GetErrors() {
			// if we've found a RouteReport with an Error, let's check if it has a sourced RouteOption
			// if so, we will add that error to the list of errors associated to that RouteOption
			if roKey, ok := extractRouteOptionSourceKeys(rerr); ok {
				errors := routeErrors[roKey]
				errors = append(errors, rerr)
				routeErrors[roKey] = errors
			}
		}
	}
	return routeErrors
}

// given a list of ListenerReports, iterate all HttpListeners to find and return all RouteReports
func getAllRouteReports(listenerReports []*validation.ListenerReport) []*validation.RouteReport {
	routeReports := []*validation.RouteReport{}
	for _, lr := range listenerReports {
		for _, hlr := range lr.GetAggregateListenerReport().GetHttpListenerReports() {
			for _, vhr := range hlr.GetVirtualHostReports() {
				routeReports = append(routeReports, vhr.GetRouteReports()...)
			}
		}
	}
	return routeReports
}

// if the Route error has a RouteOption source associated with it, extract the source and return it
func extractRouteOptionSourceKeys(routeErr *validation.RouteReport_Error) (types.NamespacedName, bool) {
	metadata := routeErr.GetMetadata()
	if metadata == nil {
		return types.NamespacedName{}, false
	}

	for _, src := range metadata.GetSources() {
		if src.GetResourceKind() == sologatewayv1.RouteOptionGVK.Kind {
			key := types.NamespacedName{
				Namespace: src.GetResourceRef().GetNamespace(),
				Name:      src.GetResourceRef().GetName(),
			}
			return key, true
		}
	}

	return types.NamespacedName{}, false
}
