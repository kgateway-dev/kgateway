package irtranslator

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"golang.org/x/net/context"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/slices"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	sdkreporter "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/validator"
)

const (
	// ListenerOmittedMessage is a fallback message for when no specific reason was provided in translation
	ListenerOmittedMessage                      = "Listener could not be generated for data plane for unknown reason"
	GatewayAcceptedListenersOmittedMessage      = "Listeners not accepted: %s"
	GatewayProgrammedListenersOmittedMessage    = "Listeners not programmed: %s"
	GatewayProgrammedAllListenersOmittedMessage = "No Listeners programmed. " + GatewayProgrammedListenersOmittedMessage
	GatewayAcceptedAllListenersOmittedMessage   = "All Listeners not accepted: %s"
)

var logger = logging.New("translator/ir")

type Translator struct {
	ContributedPolicies  map[schema.GroupKind]extensionsplug.PolicyPlugin
	RouteReplacementMode settings.RouteReplacementMode
	Validator            validator.Validator
}

type TranslationPassPlugins map[schema.GroupKind]*TranslationPass

type TranslationResult struct {
	Routes        []*envoyroutev3.RouteConfiguration
	Listeners     []*envoylistenerv3.Listener
	ExtraClusters []*envoyclusterv3.Cluster
}

// Translate IR to gateway. IR is self contained, so no need for krt context
func (t *Translator) Translate(ctx context.Context, gw ir.GatewayIR, reporter sdkreporter.Reporter) TranslationResult {
	pass := t.newPass(reporter)
	var res TranslationResult

	var notAcceptedListeners []string
	var notProgrammedListeners []string

	for _, l := range gw.Listeners {
		outListener, routes := t.ComputeListener(ctx, pass, gw, l, reporter)
		// Envoy rejects listeners with no filter chains; skip adding such listeners.
		if outListener == nil || len(outListener.GetFilterChains()) == 0 {
			listenerName, wasNotAccepted := checkListenerStatusForOmittedListener(gw, l, reporter)

			if wasNotAccepted {
				notAcceptedListeners = append(notAcceptedListeners, listenerName)
			}
			// All omitted listeners are not programmed by definition
			notProgrammedListeners = append(notProgrammedListeners, listenerName)

			continue
		}
		res.Listeners = append(res.Listeners, outListener)
		res.Routes = append(res.Routes, routes...)
	}

	// Gateway status when listeners were omitted
	if len(notProgrammedListeners) > 0 {
		reportGatewayStatusForOmittedListeners(gw, reporter, notAcceptedListeners, notProgrammedListeners)
	}

	for _, c := range pass {
		if c != nil {
			r := c.ResourcesToAdd(ctx)
			res.ExtraClusters = append(res.ExtraClusters, r.Clusters...)
		}
	}

	return res
}

func findOriginalListenerName(gw ir.GatewayIR, listener ir.ListenerIR) string {
	for _, origListener := range gw.SourceObject.Listeners {
		if uint32(origListener.Port) == listener.BindPort {
			return string(origListener.Name)
		}
	}
	return ""
}

func getReporterForFilterChain(gw ir.GatewayIR, reporter sdkreporter.Reporter, filterChainName string) sdkreporter.ListenerReporter {
	listener := slices.FindFunc(gw.SourceObject.Listeners, func(l ir.Listener) bool {
		return filterChainName == query.GenerateRouteKey(l.Parent, string(l.Name))
	})
	if listener == nil {
		// This should never happen, but keep this as a safeguard.
		return reporter.Gateway(gw.SourceObject.Obj).ListenerName(filterChainName)
	}
	return listener.GetParentReporter(reporter).ListenerName(string(listener.Name))
}

func (t *Translator) ComputeListener(
	ctx context.Context,
	pass TranslationPassPlugins,
	gw ir.GatewayIR,
	lis ir.ListenerIR,
	reporter sdkreporter.Reporter,
) (*envoylistenerv3.Listener, []*envoyroutev3.RouteConfiguration) {
	gwreporter := reporter.Gateway(gw.SourceObject.Obj)
	ret := &envoylistenerv3.Listener{
		Name:    lis.Name,
		Address: computeListenerAddress(lis.BindAddress, lis.BindPort, gwreporter),
	}
	if gw.PerConnectionBufferLimitBytes != nil {
		ret.PerConnectionBufferLimitBytes = &wrapperspb.UInt32Value{Value: *gw.PerConnectionBufferLimitBytes}
	}
	t.runListenerPlugins(ctx, pass, gw, lis, reporter, ret)

	var routes []*envoyroutev3.RouteConfiguration
	hasTls := false
	domains := map[string]struct{}{}

	for _, hfc := range lis.HttpFilterChain {
		fct := filterChainTranslator{
			listener:        lis,
			gateway:         gw,
			routeConfigName: hfc.FilterChainName,
			reporter:        reporter,
			pluginPass:      pass,
		}

		// compute routes
		hr := httpRouteConfigurationTranslator{
			gw:                       gw,
			listener:                 lis,
			routeConfigName:          hfc.FilterChainName,
			fc:                       hfc.FilterChainCommon,
			attachedPolicies:         hfc.AttachedPolicies,
			reporter:                 reporter,
			requireTlsOnVirtualHosts: hfc.FilterChainCommon.TLS != nil,
			pluginPass:               pass,
			logger:                   logger.With("route_config_name", hfc.FilterChainName),
			routeReplacementMode:     t.RouteReplacementMode,
			validator:                t.Validator,
		}
		rc := hr.ComputeRouteConfiguration(ctx, hfc.Vhosts)
		if rc != nil {
			routes = append(routes, rc)

			// Record metrics for the number of domains per listener.
			// Only one domain per virtual host is supported currently, but that may change in the future,
			// so loop through the virtual hosts and count the unique domains.
			for _, vhost := range rc.VirtualHosts {
				for _, domain := range vhost.Domains {
					domains[domain] = struct{}{}
				}
			}
		}

		// compute chains

		// TODO: make sure that all matchers are unique
		rl := getReporterForFilterChain(gw, reporter, hfc.FilterChainName)
		fc := fct.initFilterChain(hfc.FilterChainCommon)
		fc.Filters = fct.computeHttpFilters(ctx, hfc, rl)
		ret.FilterChains = append(ret.GetFilterChains(), fc)
		if len(hfc.Matcher.SniDomains) > 0 {
			hasTls = true
		}
	}

	// Update the domains per listener metric with number of domains found.
	setDomainsPerListener(domainsPerListenerMetricLabels{
		Namespace:   gw.SourceObject.GetNamespace(),
		GatewayName: gw.SourceObject.GetName(),
		Port:        strconv.Itoa(int(lis.BindPort)),
	}, len(domains))

	fct := filterChainTranslator{
		listener:   lis,
		gateway:    gw,
		pluginPass: pass,
	}

	for _, tfc := range lis.TcpFilterChain {
		rl := getReporterForFilterChain(gw, reporter, tfc.FilterChainName)
		fc := fct.initFilterChain(tfc.FilterChainCommon)
		fc.Filters = fct.computeTcpFilters(ctx, tfc, rl)
		ret.FilterChains = append(ret.GetFilterChains(), fc)
		if len(tfc.Matcher.SniDomains) > 0 {
			hasTls = true
		}
	}
	// sort filter chains for idempotency
	sort.Slice(ret.GetFilterChains(), func(i, j int) bool {
		return ret.GetFilterChains()[i].GetName() < ret.GetFilterChains()[j].GetName()
	})
	if hasTls {
		ret.ListenerFilters = append(ret.GetListenerFilters(), tlsInspectorFilter())
	}

	return ret, routes
}

func (t *Translator) runListenerPlugins(
	ctx context.Context,
	pass TranslationPassPlugins,
	gw ir.GatewayIR,
	l ir.ListenerIR,
	reporter sdkreporter.Reporter,
	out *envoylistenerv3.Listener,
) {
	var attachedPolicies ir.AttachedPolicies
	// Listener policies take precedence over gateway policies, so they are ordered first
	attachedPolicies.Append(l.AttachedPolicies, gw.AttachedHttpPolicies)
	for _, gk := range attachedPolicies.ApplyOrderedGroupKinds() {
		pols := attachedPolicies.Policies[gk]
		pass := pass[gk]
		if pass == nil {
			// TODO: report user error - they attached a non http policy
			continue
		}
		reportPolicyAcceptanceStatus(reporter, l.PolicyAncestorRef, pols...)
		policies, mergeOrigins := mergePolicies(pass, pols)
		for _, pol := range policies {
			pctx := &ir.ListenerContext{
				Policy: pol.PolicyIr,
				PolicyAncestorRef: gwv1.ParentReference{
					Group:     ptr.To(gwv1.Group(wellknown.GatewayGVK.Group)),
					Kind:      ptr.To(gwv1.Kind(wellknown.GatewayGVK.Kind)),
					Namespace: ptr.To(gwv1.Namespace(gw.SourceObject.GetNamespace())),
					Name:      gwv1.ObjectName(gw.SourceObject.GetName()),
				},
			}
			pass.ApplyListenerPlugin(ctx, pctx, out)
		}
		out.Metadata = addMergeOriginsToFilterMetadata(gk, mergeOrigins, out.GetMetadata())
		reportPolicyAttachmentStatus(reporter, l.PolicyAncestorRef, mergeOrigins, pols...)
	}
}

func (t *Translator) newPass(reporter sdkreporter.Reporter) TranslationPassPlugins {
	ret := TranslationPassPlugins{}
	for k, v := range t.ContributedPolicies {
		if v.NewGatewayTranslationPass == nil {
			continue
		}
		tp := v.NewGatewayTranslationPass(context.TODO(), ir.GwTranslationCtx{}, reporter)
		if tp != nil {
			ret[k] = &TranslationPass{
				ProxyTranslationPass: tp,
				Name:                 v.Name,
				MergePolicies:        v.MergePolicies,
			}
		}
	}
	return ret
}

type TranslationPass struct {
	ir.ProxyTranslationPass
	Name string
	// If the plugin supports policy merging, it must implement MergePolicies
	// such that policies ordered from high to low priority, both hierarchically
	// and within the same hierarchy, are Merged into a single Policy
	MergePolicies func(policies []ir.PolicyAtt) ir.PolicyAtt
}

// checkListenerStatusForOmittedListener will create a condition with "Type: Programmed" and "Status: False" if a Programmed condition does not exist
func checkListenerStatusForOmittedListener(
	gw ir.GatewayIR,
	listener ir.ListenerIR,
	reporter sdkreporter.Reporter,
) (listenerName string, wasNotAccepted bool) {
	originalListenerName := findOriginalListenerName(gw, listener)
	listenerReporter := getReporterForFilterChain(gw, reporter, originalListenerName)

	// This is fallback behavior. If the listener is returned nil or the filter chains are empty,there should already be a programmed condition set with a specific reason.
	if lr, ok := listenerReporter.(*reports.ListenerReport); ok {
		// Check existing conditions to determine why the listener was omitted
		acceptedCondition := meta.FindStatusCondition(lr.Status.Conditions, string(gwv1.ListenerConditionAccepted))
		programmedCondition := meta.FindStatusCondition(lr.Status.Conditions, string(gwv1.ListenerConditionProgrammed))

		// Determine if listener was not accepted
		wasNotAccepted = acceptedCondition != nil && acceptedCondition.Status == metav1.ConditionFalse

		// Set default programmed condition if it doesn't exist (fallback behavior)
		if programmedCondition == nil {
			listenerCondition := sdkreporter.ListenerCondition{
				Type:    gwv1.ListenerConditionProgrammed,
				Status:  metav1.ConditionFalse,
				Reason:  gwv1.ListenerReasonInvalid,
				Message: ListenerOmittedMessage,
			}
			listenerReporter.SetCondition(listenerCondition)
		}
	} else {
		logger.Error("listener reporter type not supported", "reporter", fmt.Sprintf("%T", listenerReporter))
	}

	return originalListenerName, wasNotAccepted
}

// reportGatewayStatusForOmittedListeners sets gateway conditions when listeners were omitted
func reportGatewayStatusForOmittedListeners(
	gw ir.GatewayIR,
	reporter sdkreporter.Reporter,
	notAcceptedListeners []string,
	notProgrammedListeners []string,
) {
	gwreporter := reporter.Gateway(gw.SourceObject.Obj)

	// Sort for idempotency
	sort.Strings(notAcceptedListeners)
	sort.Strings(notProgrammedListeners)

	// Set Accepted condition - if there are listeners that were explicitly not accepted,
	// mark gateway as not accepted. Otherwise, mark as accepted but mention omitted listeners.
	if len(notAcceptedListeners) > 0 {
		var acceptedMessage string
		if len(notAcceptedListeners) == len(gw.Listeners) {
			acceptedMessage = fmt.Sprintf(GatewayAcceptedAllListenersOmittedMessage, strings.Join(notAcceptedListeners, ", "))
		} else {
			acceptedMessage = fmt.Sprintf(GatewayAcceptedListenersOmittedMessage, strings.Join(notAcceptedListeners, ", "))
		}

		acceptedCondition := sdkreporter.GatewayCondition{
			Type:    gwv1.GatewayConditionAccepted,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.GatewayReasonListenersNotValid,
			Message: acceptedMessage,
		}
		gwreporter.SetCondition(acceptedCondition)
	} else {
		// Gateway is accepted, but some listeners were omitted (just not programmed)
		acceptedCondition := sdkreporter.GatewayCondition{
			Type:    gwv1.GatewayConditionAccepted,
			Status:  metav1.ConditionTrue,
			Reason:  gwv1.GatewayReasonListenersNotValid,
			Message: fmt.Sprintf(GatewayAcceptedListenersOmittedMessage, strings.Join(notProgrammedListeners, ", ")),
		}
		gwreporter.SetCondition(acceptedCondition)
	}

	// Set Programmed condition based on whether any listeners were programmed
	var programmedMessage string
	if len(notProgrammedListeners) == len(gw.Listeners) {
		programmedMessage = fmt.Sprintf(GatewayProgrammedAllListenersOmittedMessage, strings.Join(notProgrammedListeners, ", "))
	} else {
		programmedMessage = fmt.Sprintf(GatewayProgrammedListenersOmittedMessage, strings.Join(notProgrammedListeners, ", "))
	}

	if len(notProgrammedListeners) == len(gw.Listeners) {
		programmedCondition := sdkreporter.GatewayCondition{
			Type:    gwv1.GatewayConditionProgrammed,
			Status:  metav1.ConditionFalse,
			Reason:  gwv1.GatewayReasonListenersNotValid,
			Message: programmedMessage,
		}
		gwreporter.SetCondition(programmedCondition)
	} else {
		programmedCondition := sdkreporter.GatewayCondition{
			Type:    gwv1.GatewayConditionProgrammed,
			Status:  metav1.ConditionTrue,
			Reason:  gwv1.GatewayReasonProgrammed,
			Message: programmedMessage,
		}
		gwreporter.SetCondition(programmedCondition)
	}
}
