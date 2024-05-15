package routeoptions

import (
	"container/list"
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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var (
	_ plugins.RoutePlugin  = &plugin{}
	_ plugins.StatusPlugin = &plugin{}
)

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
	routeOptions, routeOptionKube, filterOverride, err := p.handleAttachment(ctx, routeCtx)
	if err != nil {
		return err
	}

	if routeOptions != nil {
		// clobber the existing RouteOptions; merge semantics may be desired later
		outputRoute.Options = routeOptions

		if !filterOverride {
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
) (*gloov1.RouteOptions, *solokubev1.RouteOption, bool, error) {
	// parentRouteRef refers to the parent route in a delegation chain.
	// The condition below allows it to be optional in the RouteContext
	var parentRouteRef *list.Element
	if routeCtx.DelegationChain != nil {
		parentRouteRef = routeCtx.DelegationChain.Front()
	}

	// TODO: This is far too naive and we should optimize the amount of querying we do.
	// Route plugins run on every match for every Rule in a Route but the attached options are
	// the same each time; i.e. HTTPRoute <-1:1-> RouteOptions.
	// We should only make this query once per HTTPRoute.
	attachedOption, filterOverride, err := p.rtOptQueries.GetRouteOptionForRouteRule(
		ctx,
		types.NamespacedName{Name: routeCtx.Route.Name, Namespace: routeCtx.Route.Namespace},
		routeCtx.Rule,
		parentRouteRef,
		p.gwQueries,
	)
	if err != nil {
		contextutils.LoggerFrom(ctx).Errorf("error getting RouteOptions for Route: %v", err)
		switch {
		case errors.Is(err, utils.ErrTypesNotEqual):
		case errors.Is(err, utils.ErrNotSettable):
			devErr := fmt.Errorf("developer error while getting RouteOptions as ExtensionRef: %w", err)
			contextutils.LoggerFrom(ctx).DPanic(devErr)
		default:
			routeCtx.Reporter.SetCondition(reports.HTTPRouteCondition{
				Type:    gwv1.RouteConditionResolvedRefs,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.RouteReasonBackendNotFound,
				Message: err.Error(),
			})
		}
		return nil, nil, false, err
	}
	if attachedOption == nil || attachedOption.Spec.GetOptions() == nil {
		return nil, nil, false, nil
	}

	return attachedOption.Spec.GetOptions(), attachedOption, filterOverride, nil
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
