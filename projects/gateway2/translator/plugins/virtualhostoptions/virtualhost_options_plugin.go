package virtualhostoptions

import (
	"context"
	"fmt"
	"strings"

	"github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	gwquery "github.com/solo-io/gloo/projects/gateway2/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/listenerutils"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/utils"
	vhoptquery "github.com/solo-io/gloo/projects/gateway2/translator/plugins/virtualhostoptions/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/vhostutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/grpc/validation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	glooutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
)

var (
	_ plugins.ListenerPlugin = &plugin{}
	_ plugins.StatusPlugin   = &plugin{}
)

type plugin struct {
	gwQueries          gwquery.GatewayQueries
	vhOptQueries       vhoptquery.VirtualHostOptionQueries
	classicStatusCache classicStatusCache // The lifecycle of this cache is that of the plugin with the assumption that plugins are rebuilt on every translation
	vhOptionClient     sologatewayv1.VirtualHostOptionClient
	statusReporter     reporter.StatusReporter
}

// holds the data structures needed to derive and report a classic GE status
type classicStatus struct {
	// proxyStatus
	subresourceStatus map[string]*core.Status
	// *All* of the virtual host errors encountered during processing for gloov1.VirtualHost which receive their
	// options for this VirtualHostOption
	virtualHostErrors []*validation.VirtualHostReport_Error
	// Warnings to add to the status which can indicate that a VHO was not attached due to conflict with a more-specific VHO
	warnings []string
}

// holds status structure for each VirtualHostOption we have processed and attached.
// this is used because a VirtualHostOption is attached to a Gateway, but many VirtualHosts may be
// translated out of a Gateway, so we need a single status object to contain the subresourceStatus
// for each Proxy it was translated to, but also all the errors specifically encountered
type classicStatusCache map[types.NamespacedName]*classicStatus

func (c *classicStatusCache) getOrCreateEntry(key types.NamespacedName) *classicStatus {
	if cacheEntry, ok := (*c)[key]; ok {
		return cacheEntry
	}

	cacheEntry := &classicStatus{
		subresourceStatus: map[string]*core.Status{},
		virtualHostErrors: []*validation.VirtualHostReport_Error{},
		warnings:          []string{},
	}
	(*c)[key] = cacheEntry
	return cacheEntry
}

func NewPlugin(
	gwQueries gwquery.GatewayQueries,
	client client.Client,
	vhOptionClient sologatewayv1.VirtualHostOptionClient,
	statusReporter reporter.StatusReporter,
) *plugin {
	return &plugin{
		gwQueries:          gwQueries,
		vhOptQueries:       vhoptquery.NewQuery(client),
		vhOptionClient:     vhOptionClient,
		statusReporter:     statusReporter,
		classicStatusCache: make(map[types.NamespacedName]*classicStatus),
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
		return utils.ErrUnexpectedListener(outListener)
	}

	// attachedOption represents the VirtualHostOptions targeting the Gateway on which this listener resides, and/or
	// the VirtualHostOptions which specifies this listener in section name
	attachedOptions, err := p.vhOptQueries.GetVirtualHostOptionsForListener(ctx, listenerCtx.GwListener, listenerCtx.Gateway)
	if err != nil {
		return err
	}

	if len(attachedOptions) == 0 {
		return nil
	}

	// the first option is highest priority, so we will merge all options into this one,
	// see for more context: https://github.com/solo-io/solo-projects/issues/6313
	merged := attachedOptions[0]
	optionsUsed := []*solokubev1.VirtualHostOption{attachedOptions[0]}
	optionsIgnored := []*solokubev1.VirtualHostOption{}

	for _, opt := range attachedOptions[1:] {
		optionUsed := false
		merged.Spec.Options, optionUsed = glooutils.ShallowMergeVirtualHostOptions(merged.Spec.GetOptions(), opt.Spec.GetOptions())
		if optionUsed {
			optionsUsed = append(optionsUsed, opt)
		} else {
			optionsIgnored = append(optionsIgnored, opt)
		}
	}

	// The merged option should be applied to the VirtualHosts, and the sources should be tracked
	// per VirtualHostOption that affected the merged result
	for _, opt := range optionsUsed {
		for _, v := range aggListener.GetHttpResources().GetVirtualHosts() {
			v.Options = merged.Spec.GetOptions()
			vhostutils.AppendSourceToVirtualHost(v, opt)
		}
		listenerutils.AppendSourceToListener(outListener, opt)

		// track that we used this VirtualHostOption in our status cache
		// we do this so we can persist status later for all attached VirtualHostOptions
		// since we don't have any additional details to append, we just need to make sure the
		// cache entry exists
		p.classicStatusCache.getOrCreateEntry(client.ObjectKeyFromObject(opt))
	}

	for _, opt := range optionsIgnored {
		nn := client.ObjectKeyFromObject(opt)
		cacheEntry := p.classicStatusCache.getOrCreateEntry(nn)
		cacheEntry.warnings = append(cacheEntry.warnings, fmt.Sprintf("VirtualHostOption '%s' not attached to listener '%s' on Gateway '%s/%s' due to conflict with more specific or older VirtualHostOptions '%s'",
			nn, listenerCtx.GwListener.Name, listenerCtx.Gateway.Namespace, listenerCtx.Gateway.Name, optionsToStr(optionsUsed)))
		p.classicStatusCache[nn] = cacheEntry
	}

	return nil
}

func optionsToStr(opts []*solokubev1.VirtualHostOption) string {
	resourceNames := make([]string, len(opts))
	for i, opt := range opts {
		resourceNames[i] = client.ObjectKeyFromObject(opt).String()
	}
	return strings.Join(resourceNames, ", ")
}

// Add all statuses for processed VirtualHostOptions. These could come from the VHO itself or
// or any VH to which it is attached.
func (p *plugin) ApplyStatusPlugin(ctx context.Context, statusCtx *plugins.StatusContext) error {
	logger := contextutils.LoggerFrom(ctx)
	// gather all VirtualHostOptions we need to report status for
	for _, proxyWithReport := range statusCtx.ProxiesWithReports {
		proxy := proxyWithReport.Proxy
		if proxy == nil {
			// we should never have this occur
			logger.DPanic("while trying to apply status for VirtualHostOptions, we attempted to apply status for nil Proxy")
		}
		// get proxy status to use for VirtualHostOption status
		proxyStatus := p.statusReporter.StatusFromReport(proxyWithReport.Reports.ResourceReports[proxy], nil)

		// for this specific proxy, get all the virtualHost errors and their associated VirtualHostOption sources
		virtualHostErrors := extractVirtualHostErrors(proxyWithReport.Reports.ProxyReport)
		for vhKey, errs := range virtualHostErrors {
			// grab the existing status object for this VirtualHostOption
			statusForVhO, ok := p.classicStatusCache[vhKey]
			if !ok {
				// we are processing an error that has a VirtualHostOption source that we hadn't encountered until now
				// this shouldn't happen
				logger.DPanic("while trying to apply status for VirtualHostOptions, we found a VirtualHost error sourced by an unknown VirtualHostOption", "VirtualHostOption", vhKey)
			}

			// set the subresource status for this specific proxy on the VHO
			thisSubresourceStatus := statusForVhO.subresourceStatus
			thisSubresourceStatus[xds.SnapshotCacheKey(proxyWithReport.Proxy)] = proxyStatus
			statusForVhO.subresourceStatus = thisSubresourceStatus

			// add any virtualHostErrors from this Proxy translation
			statusForVhO.virtualHostErrors = append(statusForVhO.virtualHostErrors, errs...)

			// update the cache
			p.classicStatusCache[vhKey] = statusForVhO
		}
	}
	virtualHostOptionReport := make(reporter.ResourceReports)
	// Loop through vhostopts we processed and have a status for
	for vhOptKey, status := range p.classicStatusCache {
		// get the obj by namespacedName
		vhOptObj, _ := p.vhOptionClient.Read(vhOptKey.Namespace, vhOptKey.Name, clients.ReadOpts{Ctx: ctx})

		// mark this object to be processed
		virtualHostOptionReport.Accept(vhOptObj)

		// add any virtualHost errors for this obj
		for _, rerr := range status.virtualHostErrors {
			virtualHostOptionReport.AddError(vhOptObj, eris.New(rerr.GetReason()))
		}

		virtualHostOptionReport.AddWarnings(vhOptObj, status.warnings...)

		// actually write out the reports!
		err := p.statusReporter.WriteReports(ctx, virtualHostOptionReport, status.subresourceStatus)
		if err != nil {
			return eris.Wrap(err, "writing status report from VirtualHostOptionPlugin")
		}

	}
	return nil
}

// given a ProxyReport, extract and aggregate all VirtualHost errors that have VirtualHostOption source metadata
// and key them by the source VirtualHostOption NamespacedName
func extractVirtualHostErrors(proxyReport *validation.ProxyReport) map[types.NamespacedName][]*validation.VirtualHostReport_Error {
	virtualHostErrors := make(map[types.NamespacedName][]*validation.VirtualHostReport_Error)
	virtualHostReports := getAllVirtualHostReports(proxyReport.GetListenerReports())
	for _, vhr := range virtualHostReports {
		for _, vherr := range vhr.GetErrors() {
			// if we've found a VirtualHostReport with an Error, let's check if it has a sourced VirtualHostOption
			// if so, we will add that error to the list of errors associated to that VirtualHostOption
			if vhKey, ok := extractVirtualHostOptionSourceKeys(vherr); ok {
				virtualHostErrors[vhKey] = append(virtualHostErrors[vhKey], vherr)
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
