package virtualhostoptions

import (
	"context"

	"github.com/rotisserie/eris"
	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/listenerutils"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	vhoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/virtualhostoptions/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/vhostutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ plugins.ListenerPlugin = &plugin{}
var _ plugins.StatusPlugin = &plugin{}

type plugin struct {
	gwQueries         gwquery.GatewayQueries
	vhOptQueries      vhoptquery.VirtualHostOptionQueries
	legacyStatusCache legacyStatusCache
	vhOptionClient    sologatewayv1.VirtualHostOptionClient
	statusReporter    reporter.StatusReporter
}

// holds the data structures needed to derive and report a classic GE status
type legacyStatus struct {
	// maps proxyName -> proxyStatus
	subresourceStatus map[string]*core.Status
	// *All* of the virtual host errors encountered during processing for gloov1.Routes which receive their
	// options for this VirtualHostOption
	virtualHostErrors []*validation.VirtualHostReport_Error
}

// holds status structure for each VirtualHostOption we have processed and attached
// this is used because a VirtualHostOption is attached to a Route, but a Route may be
// attached to multiple Gateways/Listeners, so we need a single status object
// to contain the subresourceStatus for each Proxy it was translated too, but also
// all the errors specifically encountered
type legacyStatusCache = map[types.NamespacedName]legacyStatus

var (
	ErrUnexpectedListenerType = eris.New("unexpected listener type")
	errUnexpectedListenerType = func(l *v1.Listener) error {
		return eris.Wrapf(ErrUnexpectedListenerType, "expected AggregateListener, got %T", l.GetListenerType())
	}
)

func NewPlugin(
	gwQueries gwquery.GatewayQueries,
	client client.Client,
	vhOptionClient sologatewayv1.VirtualHostOptionClient,
	statusReporter reporter.StatusReporter,
) *plugin {
	return &plugin{
		gwQueries:         gwQueries,
		vhOptQueries:      vhoptquery.NewQuery(client),
		vhOptionClient:    vhOptionClient,
		statusReporter:    statusReporter,
		legacyStatusCache: make(map[types.NamespacedName]legacyStatus),
	}
}

func (p *plugin) ApplyListenerPlugin(
	ctx context.Context,
	listenerCtx *plugins.ListenerContext,
	outListener *v1.Listener,
) error {
	// Currently we only create AggregateListeners in k8s gateway translation.
	// If that ever changes, we will need to handle other listener types more gracefully here.
	aggListener := outListener.GetAggregateListener()
	if aggListener == nil {
		return errUnexpectedListenerType(outListener)
	}

	// attachedOption represents the VirtualHostOptions targeting the Gateway on which this listener resides, and/or
	// the VirtualHostOptions which specifies this listener in section name
	attachedOptions, err := p.vhOptQueries.GetVirtualHostOptionsForListener(ctx, listenerCtx.GwListener, listenerCtx.Gateway)
	if err != nil {
		return err
	}

	if attachedOptions == nil || len(attachedOptions) == 0 {
		return nil
	}

	if numOpts := len(attachedOptions); numOpts > 1 {
		// TODO: Report conflicts on the [1:] options
	}

	if attachedOptions[0] == nil {
		// unsure if this should be an error case
		return nil
	}

	for _, v := range aggListener.GetHttpResources().GetVirtualHosts() {
		v.Options = attachedOptions[0].Spec.GetOptions()
		vhostutils.AppendSourceToVirtualHost(v, attachedOptions[0])
	}
	listenerutils.AppendSourceToListener(outListener, attachedOptions[0])

	// track that we used this VirtualHostOption in our status cache
	// we do this so we can persist status later for all attached VirtualHostOptions
	p.legacyStatusCache[client.ObjectKeyFromObject(attachedOptions[0])] = legacyStatus{
		subresourceStatus: map[string]*core.Status{},
	}

	// TODO track all matching but unused attached vhopts to show conflicted attachment

	return nil
}

func (p *plugin) ApplyStatusPlugin(ctx context.Context, statusCtx *plugins.StatusContext) error {
	contextutils.LoggerFrom(ctx).Infof("status plugin running over %d proxies with reports", len(statusCtx.ProxiesWithReports))
	// gather all VirtualHostOptions we need to report status for
	for _, proxyWithReport := range statusCtx.ProxiesWithReports {
		// get proxy status to use for VirtualHostOption status
		proxyStatus := p.statusReporter.StatusFromReport(proxyWithReport.Reports.ResourceReports[proxyWithReport.Proxy], nil)

		// for this specific proxy, get all the virtualHost errors and their associated VirtualHostOption sources
		virtualHostErrors := extractVirtualHostErrors(proxyWithReport.Reports.ProxyReport)
		for vhoKey, errs := range virtualHostErrors {
			// grab the existing status object for this VirtualHostOption
			statusForVhO, ok := p.legacyStatusCache[vhoKey]
			if !ok {
				// we are processing an error that has a VirtualHostOption source that we hadn't encountered until now
				// this shouldn't happen
				contextutils.LoggerFrom(ctx).DPanic("while trying to apply status for VirtualHostOptions, we found a VirtualHost error sourced by an unknown VirtualHostOption", "VirtualHostOption", vhoKey)
			}

			// set the subresource status for this specific proxy on the RO
			thisSubresourceStatus := statusForVhO.subresourceStatus
			thisSubresourceStatus[xds.SnapshotCacheKey(proxyWithReport.Proxy)] = proxyStatus
			statusForVhO.subresourceStatus = thisSubresourceStatus

			// add any virtualHostErrors from this Proxy translation
			statusForVhO.virtualHostErrors = append(statusForVhO.virtualHostErrors, errs...)

			// update the cache
			p.legacyStatusCache[vhoKey] = statusForVhO
		}
	}
	contextutils.LoggerFrom(ctx).Infof("status plugin writing reports for %d resources", len(p.legacyStatusCache))
	virtualHostOptionReport := make(reporter.ResourceReports)
	for vhKey, status := range p.legacyStatusCache {
		// get the obj by namespacedName
		vhObj, _ := p.vhOptionClient.Read(vhKey.Namespace, vhKey.Name, clients.ReadOpts{Ctx: ctx})

		// mark this object to be processed
		virtualHostOptionReport.Accept(vhObj)

		// add any virtualHost errors for this obj
		for _, rerr := range status.virtualHostErrors {
			virtualHostOptionReport.AddError(vhObj, eris.New(rerr.GetReason()))
		}

		// actually write out the reports!
		contextutils.LoggerFrom(ctx).Infof("writing report for %s: %v", vhKey, status.subresourceStatus)
		err := p.statusReporter.WriteReports(ctx, virtualHostOptionReport, status.subresourceStatus)
		if err != nil {
			return eris.Errorf("error writing status report from VirtualHostOptionPlugin: %w", err)
		}
	}
	return nil

}

// given a ProxyReport, extract and aggregate all VirtualHost errors that have VirtualHostOption source metadata
// and key them by the source VirtualHostOption NamespacedName
func extractVirtualHostErrors(proxyReport *validation.ProxyReport) map[types.NamespacedName][]*validation.VirtualHostReport_Error {
	virtualHostErrors := make(map[types.NamespacedName][]*validation.VirtualHostReport_Error)
	virtualHostReports := getAllVirtualHostReports(proxyReport.GetListenerReports())
	for _, rr := range virtualHostReports {
		for _, rerr := range rr.GetErrors() {
			// if we've found a VirtualHostReport with an Error, let's check if it has a sourced VirtualHostOption
			// if so, we will add that error to the list of errors associated to that VirtualHostOption
			if roKey, ok := extractVirtualHostOptionSourceKeys(rerr); ok {
				errors := virtualHostErrors[roKey]
				errors = append(errors, rerr)
				virtualHostErrors[roKey] = errors
			}
		}
	}
	return virtualHostErrors
}

// given a list of ListenerReports, iterate all HttpListeners to find and return all VirtualHostReports
func getAllVirtualHostReports(listenerReports []*validation.ListenerReport) []*validation.VirtualHostReport {
	virtualHostReports := []*validation.VirtualHostReport{}
	for _, lr := range listenerReports {
		for _, hlr := range lr.GetAggregateListenerReport().GetHttpListenerReports() {
			virtualHostReports = append(virtualHostReports, hlr.GetVirtualHostReports()...)
		}
	}
	return virtualHostReports
}

// if the VirtualHost error has a VirtualHostOption source associated with it, extract the source and return it
func extractVirtualHostOptionSourceKeys(virtualHostErr *validation.VirtualHostReport_Error) (types.NamespacedName, bool) {
	metadata := virtualHostErr.GetMetadata()
	if metadata == nil {
		return types.NamespacedName{}, false
	}

	for _, src := range metadata.GetSources() {
		if src.GetResourceKind() == sologatewayv1.VirtualHostOptionGVK.Kind {
			key := types.NamespacedName{
				Namespace: src.GetResourceRef().GetNamespace(),
				Name:      src.GetResourceRef().GetName(),
			}
			return key, true
		}
	}

	return types.NamespacedName{}, false
}
