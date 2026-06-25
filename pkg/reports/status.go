package reports

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"istio.io/istio/pkg/ptr"
	"istio.io/istio/pkg/slices"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

// Status message constants
const (
	GatewayAcceptedMessage         = "Successfully accepted Gateway"
	GatewayProgrammedMessage       = "Successfully programmed Gateway"
	GatewayResolvedRefsMessage     = "Successfully resolved all Gateway references"
	GatewayListenersNotResolved    = "One or more listeners have unresolved references"
	GatewayInsecureFallbackMessage = "Gateway frontend validation is configured to allow insecure fallback"
	ListenerSetAcceptedMessage     = "Successfully accepted ListenerSet"
	ListenerSetProgrammedMessage   = "Successfully programmed ListenerSet"
	ListenerAcceptedMessage        = "Successfully accepted Listener"
	ListenerNoConflictsMessage     = "Successfully verified that Listener has no conflicts"
	ValidRefsMessage               = "Successfully resolved all references"
	ListenerProgrammedMessage      = "Successfully programmed Listener"
	RouteAcceptedMessage           = "Successfully accepted Route"
	GatewayClassAcceptedMessage    = "GatewayClass accepted by kgateway controller"
)

// TODO: refactor this struct + methods to better reflect the usage now in proxy_syncer

func (r *ReportMap) BuildGWStatus(ctx context.Context, gw gwv1.Gateway, attachedRoutes map[string]uint) *gwv1.GatewayStatus {
	gwReport := r.GatewayNamespaceName(key(&gw))
	if gwReport == nil {
		return nil
	}

	finalListeners := make([]gwv1.ListenerStatus, 0, len(gw.Spec.Listeners))
	var invalidListeners []string
	var invalidMessages []string

	for _, lis := range gw.Spec.Listeners {
		listenerStatus := listenerStatusFromGatewayReport(gwReport, lis)
		// Get attached routes for this listener
		if attachedRoutes != nil {
			if count, exists := attachedRoutes[string(lis.Name)]; exists {
				listenerStatus.AttachedRoutes = int32(count) //nolint:gosec // G115: route count is always non-negative
			}
		}

		finalConditions := make([]metav1.Condition, 0, len(listenerStatus.Conditions))
		oldLisStatusIndex := slices.IndexFunc(gw.Status.Listeners, func(l gwv1.ListenerStatus) bool {
			return l.Name == lis.Name
		})
		for _, lisCondition := range listenerStatus.Conditions {
			// Stamp the generation the report was built for, not the live object's
			// generation. The report is produced by translation (istio cache) while
			// the syncer reads the Gateway from a separate controller-runtime cache;
			// using the live generation here lets the two caches disagree and freeze
			// observedGeneration when they skew. The report's generation is internally
			// consistent with the conditions it carries.
			lisCondition.ObservedGeneration = gwReport.observedGeneration

			// copy old condition from gw so LastTransitionTime is set correctly below by SetStatusCondition()
			if oldLisStatusIndex != -1 {
				if cond := meta.FindStatusCondition(gw.Status.Listeners[oldLisStatusIndex].Conditions, lisCondition.Type); cond != nil {
					finalConditions = append(finalConditions, *cond)
				}
			}
			meta.SetStatusCondition(&finalConditions, lisCondition)

			// Check if this is the Programmed condition and it's False
			if lisCondition.Type == string(gwv1.ListenerConditionProgrammed) && lisCondition.Status == metav1.ConditionFalse {
				invalidListeners = append(invalidListeners, string(lis.Name))
				if lisCondition.Message != "" {
					invalidMessages = append(invalidMessages, fmt.Sprintf("%s: %s", lis.Name, lisCondition.Message))
				}
			}
		}
		listenerStatus.Conditions = finalConditions

		finalListeners = append(finalListeners, listenerStatus)
	}

	gwConditions := slices.Clone(gwReport.GetConditions())

	// If any listeners have Programmed=False, set Gateway Accepted=True with ListenersNotValid reason
	if len(invalidListeners) > 0 {
		message := fmt.Sprintf("Some listeners are not programmed: %s", strings.Join(invalidMessages, "; "))
		if len(invalidMessages) == 0 {
			message = fmt.Sprintf("Some listeners are not programmed: %s", strings.Join(invalidListeners, ", "))
		}

		meta.SetStatusCondition(&gwConditions, metav1.Condition{
			Type:    string(gwv1.GatewayConditionAccepted),
			Status:  metav1.ConditionTrue,
			Reason:  string(gwv1.GatewayReasonListenersNotValid),
			Message: message,
		})
	}

	gwConditions = handleInvalidAddresses(gwConditions, &gw)
	gwConditions = handleInsecureFrontendValidationMode(gwConditions, &gw)
	gwConditions = gatewayConditionsWithDefaults(gwConditions, &gw, finalListeners)

	finalConditions := make([]metav1.Condition, 0)
	for _, gwCondition := range gwConditions {
		// See note above: stamp the report's generation, not the live object's, so a
		// skew between the translation cache and the syncer's cache cannot freeze
		// observedGeneration.
		gwCondition.ObservedGeneration = gwReport.observedGeneration

		// copy old condition from gw so LastTransitionTime is set correctly below by SetStatusCondition()
		if cond := meta.FindStatusCondition(gw.Status.Conditions, gwCondition.Type); cond != nil {
			finalConditions = append(finalConditions, *cond)
		}
		meta.SetStatusCondition(&finalConditions, gwCondition)
	}
	// If there are conditions on the Gateway that are not owned by our reporter, include
	// them in the final list of conditions to preseve conditions we do not own
	for _, condition := range gw.Status.Conditions {
		if shouldPreserveGatewayCondition(condition, finalConditions) {
			finalConditions = append(finalConditions, condition)
		}
	}

	finalGwStatus := gwv1.GatewayStatus{}
	finalGwStatus.Addresses = gw.Status.Addresses
	finalGwStatus.Conditions = finalConditions
	finalGwStatus.Listeners = finalListeners
	if gwReport.attachedListenerSets > 0 {
		finalGwStatus.AttachedListenerSets = &gwReport.attachedListenerSets
	}
	return &finalGwStatus
}

func listenerStatusFromGatewayReport(report *GatewayReport, listener gwv1.Listener) gwv1.ListenerStatus {
	var listeners map[string]*ListenerReport
	if report != nil {
		listeners = report.listeners
	}
	return listenerStatusFromReports(listeners, listener)
}

func listenerStatusFromListenerSetReport(report *ListenerSetReport, listener gwv1.Listener) gwv1.ListenerStatus {
	var listeners map[string]*ListenerReport
	if report != nil {
		listeners = report.listeners
	}
	return listenerStatusFromReports(listeners, listener)
}

func listenerStatusFromReports(listeners map[string]*ListenerReport, listener gwv1.Listener) gwv1.ListenerStatus {
	status := newListenerStatus(string(listener.Name))
	if listeners != nil {
		if listenerReport := listeners[string(listener.Name)]; listenerReport != nil {
			status = listenerReport.Status
			status.Conditions = slices.Clone(listenerReport.Status.Conditions)
			status.SupportedKinds = slices.Clone(listenerReport.Status.SupportedKinds)
		}
	}
	status.Conditions = listenerConditionsWithDefaults(status.Conditions)
	return status
}

func newListenerStatus(name string) gwv1.ListenerStatus {
	return gwv1.ListenerStatus{
		Name:           gwv1.SectionName(name),
		SupportedKinds: []gwv1.RouteGroupKind{},
	}
}

func handleInvalidAddresses(conditions []metav1.Condition, g *gwv1.Gateway) []metav1.Condition {
	for _, addr := range g.Spec.Addresses {
		if addr.Type == nil {
			continue
		}
		switch *addr.Type {
		case gwv1.IPAddressType:
		case gwv1.HostnameAddressType:
			meta.SetStatusCondition(&conditions, metav1.Condition{
				Type:    string(gwv1.GatewayConditionProgrammed),
				Status:  metav1.ConditionFalse,
				Reason:  string(gwv1.GatewayReasonAddressNotUsable),
				Message: "Hostname addresses may not be used",
			})
		default:
			meta.SetStatusCondition(&conditions, metav1.Condition{
				Type:    string(gwv1.GatewayConditionAccepted),
				Status:  metav1.ConditionFalse,
				Reason:  string(gwv1.GatewayReasonUnsupportedAddress),
				Message: "Unknown address kind",
			})
		}
	}
	return conditions
}

func handleInsecureFrontendValidationMode(conditions []metav1.Condition, g *gwv1.Gateway) []metav1.Condition {
	if !gatewayUsesInsecureFrontendValidationMode(g) {
		return conditions
	}

	meta.SetStatusCondition(&conditions, metav1.Condition{
		Type:    string(gwv1.GatewayConditionInsecureFrontendValidationMode),
		Status:  metav1.ConditionTrue,
		Reason:  string(gwv1.GatewayReasonConfigurationChanged),
		Message: GatewayInsecureFallbackMessage,
	})
	return conditions
}

func gatewayUsesInsecureFrontendValidationMode(g *gwv1.Gateway) bool {
	if g == nil || g.Spec.TLS == nil || g.Spec.TLS.Frontend == nil {
		return false
	}

	if validationUsesInsecureFrontendMode(g.Spec.TLS.Frontend.Default.Validation) {
		return true
	}

	for _, perPort := range g.Spec.TLS.Frontend.PerPort {
		if validationUsesInsecureFrontendMode(perPort.TLS.Validation) {
			return true
		}
	}

	return false
}

func validationUsesInsecureFrontendMode(validation *gwv1.FrontendTLSValidation) bool {
	return validation != nil && validation.Mode == gwv1.AllowInsecureFallback
}

func isReporterOwnedGatewayConditionType(conditionType gwv1.GatewayConditionType) bool {
	switch conditionType {
	case gwv1.GatewayConditionAccepted,
		gwv1.GatewayConditionProgrammed,
		gwv1.GatewayConditionResolvedRefs,
		gwv1.GatewayConditionInsecureFrontendValidationMode:
		return true
	default:
		return false
	}
}

func shouldPreserveGatewayCondition(condition metav1.Condition, finalConditions []metav1.Condition) bool {
	if meta.FindStatusCondition(finalConditions, condition.Type) != nil {
		return false
	}

	if condition.Type == string(gwv1.GatewayConditionAccepted) &&
		condition.Status == metav1.ConditionFalse &&
		condition.Reason == string(gwv1.GatewayReasonInvalidParameters) {
		return true
	}

	return !isReporterOwnedGatewayConditionType(gwv1.GatewayConditionType(condition.Type))
}

func (r *ReportMap) BuildListenerSetStatus(ctx context.Context, ls gwv1.ListenerSet) *gwv1.ListenerSetStatus {
	lsReport := r.ListenerSet(&ls)
	if lsReport == nil {
		return nil
	}

	finalListeners := make([]gwv1.ListenerStatus, 0, len(ls.Spec.Listeners))
	var invalidListeners []string
	var invalidMessages []string

	// We check if the ls has been rejected since no status implies that it will be accepted later on
	listenerSetRejected := func(lsReport *ListenerSetReport) bool {
		if cond := meta.FindStatusCondition(lsReport.GetConditions(), string(gwv1.GatewayConditionAccepted)); cond != nil {
			return cond.Status == metav1.ConditionFalse
		}
		return false
	}

	if !listenerSetRejected(lsReport) {
		for _, l := range ls.Spec.Listeners {
			lis := utils.ToListener(l)
			listenerStatus := listenerStatusFromListenerSetReport(lsReport, lis)

			finalConditions := make([]metav1.Condition, 0, len(listenerStatus.Conditions))
			oldLisStatusIndex := slices.IndexFunc(ls.Status.Listeners, func(l gwv1.ListenerEntryStatus) bool {
				return l.Name == lis.Name
			})
			for _, lisCondition := range listenerStatus.Conditions {
				// Stamp the report's generation, not the live object's, for the same
				// cross-cache reason as Gateway and Route status.
				lisCondition.ObservedGeneration = lsReport.observedGeneration

				// copy old condition from ls so LastTransitionTime is set correctly below by SetStatusCondition()
				if oldLisStatusIndex != -1 {
					if cond := meta.FindStatusCondition(ls.Status.Listeners[oldLisStatusIndex].Conditions, lisCondition.Type); cond != nil {
						finalConditions = append(finalConditions, *cond)
					}
				}
				meta.SetStatusCondition(&finalConditions, lisCondition)

				// Check if this is the Programmed condition and it's False
				if lisCondition.Type == string(gwv1.ListenerConditionProgrammed) && lisCondition.Status == metav1.ConditionFalse {
					invalidListeners = append(invalidListeners, string(lis.Name))
					if lisCondition.Message != "" {
						invalidMessages = append(invalidMessages, fmt.Sprintf("%s: %s", lis.Name, lisCondition.Message))
					}
				}
			}
			listenerStatus.Conditions = finalConditions
			finalListeners = append(finalListeners, listenerStatus)
		}
	}

	lsConditions := slices.Clone(lsReport.GetConditions())

	// If any listeners have Programmed=False, set ListenerSet Accepted=True with ListenersNotValid reason
	if len(invalidListeners) > 0 {
		message := fmt.Sprintf("Some listeners are not programmed: %s", strings.Join(invalidMessages, "; "))
		if len(invalidMessages) == 0 {
			message = fmt.Sprintf("Some listeners are not programmed: %s", strings.Join(invalidListeners, ", "))
		}

		meta.SetStatusCondition(&lsConditions, metav1.Condition{
			Type:    string(gwv1.GatewayConditionAccepted),
			Status:  metav1.ConditionTrue,
			Reason:  string(gwv1.GatewayReasonListenersNotValid),
			Message: message,
		})
	}

	// If there are no valid listeners, reject the listenerSet
	if len(finalListeners) != 0 {
		if len(invalidListeners) == len(finalListeners) {
			meta.SetStatusCondition(&lsConditions, metav1.Condition{
				Type:    string(gwv1.GatewayConditionAccepted),
				Status:  metav1.ConditionFalse,
				Reason:  string(gwv1.GatewayReasonListenersNotValid),
				Message: "No valid listeners",
			})
			meta.SetStatusCondition(&lsConditions, metav1.Condition{
				Type:    string(gwv1.GatewayConditionProgrammed),
				Status:  metav1.ConditionFalse,
				Reason:  string(gwv1.GatewayReasonListenersNotValid),
				Message: "No valid listeners",
			})
		}
	}

	lsConditions = listenerSetConditionsWithDefaults(lsConditions)

	finalConditions := make([]metav1.Condition, 0)
	for _, lsCondition := range lsConditions {
		// See note above: stamp the report's generation, not the live object's.
		lsCondition.ObservedGeneration = lsReport.observedGeneration

		// copy old condition from ls so LastTransitionTime is set correctly below by SetStatusCondition()
		if cond := meta.FindStatusCondition(ls.Status.Conditions, lsCondition.Type); cond != nil {
			finalConditions = append(finalConditions, *cond)
		}
		meta.SetStatusCondition(&finalConditions, lsCondition)
	}
	// If there are conditions on the Listener Set that are not owned by our reporter, include
	// them in the final list of conditions to preseve conditions we do not own
	for _, condition := range ls.Status.Conditions {
		if meta.FindStatusCondition(finalConditions, condition.Type) == nil {
			finalConditions = append(finalConditions, condition)
		}
	}

	finalLsStatus := gwv1.ListenerSetStatus{}
	finalLsStatus.Conditions = finalConditions
	fl := make([]gwv1.ListenerEntryStatus, 0, len(finalListeners))
	for _, f := range finalListeners {
		fl = append(fl, gwv1.ListenerEntryStatus{
			Name:           f.Name,
			SupportedKinds: f.SupportedKinds,
			AttachedRoutes: f.AttachedRoutes,
			Conditions:     f.Conditions,
		})
	}
	finalLsStatus.Listeners = fl
	return &finalLsStatus
}

// BuildBackendStatus builds the Backend's status from its report, preserving the
// LastTransitionTime of unchanged conditions and any conditions we don't own. Returns nil
// if the Backend has no report (i.e. it wasn't translated).
func (r *ReportMap) BuildBackendStatus(
	ctx context.Context,
	obj metav1.Object,
	currentStatus kgateway.BackendStatus,
) *kgateway.BackendStatus {
	report := r.backend(obj)
	if report == nil {
		return nil
	}

	// Stamp the generation the report was built for, not the live object's, for
	// the same cross-cache reason as Gateway and Route status.
	observedGeneration := report.observedGeneration
	finalConditions := make([]metav1.Condition, 0, len(report.Conditions))
	for _, condition := range report.Conditions {
		condition.ObservedGeneration = observedGeneration
		// Copy old condition to preserve LastTransitionTime, if it exists.
		if cond := meta.FindStatusCondition(currentStatus.Conditions, condition.Type); cond != nil {
			finalConditions = append(finalConditions, *cond)
		}
		meta.SetStatusCondition(&finalConditions, condition)
	}
	// Preserve conditions on the current status whose type we do not own. A condition
	// type that kgateway manages (e.g. EndpointsDiscovered) but that the fresh report no
	// longer contains must be dropped rather than carried forward: it means the backend
	// stopped producing that condition (e.g. it is no longer an EC2 backend, or runtime
	// discovery was disabled), so retaining it would advertise stale state forever.
	for _, condition := range currentStatus.Conditions {
		if _, owned := backendConditionTypesOwnedByKgateway[condition.Type]; owned {
			continue
		}
		if meta.FindStatusCondition(finalConditions, condition.Type) == nil {
			finalConditions = append(finalConditions, condition)
		}
	}

	return &kgateway.BackendStatus{Conditions: finalConditions}
}

// backendConditionTypesOwnedByKgateway is the set of Backend status condition types
// that kgateway is the authoritative writer for. When such a type is absent from a
// freshly built report it is dropped from the persisted status instead of being
// preserved, so a backend that stops contributing it does not retain a stale condition.
var backendConditionTypesOwnedByKgateway = map[string]struct{}{
	string(kgateway.BackendConditionAccepted):            {},
	string(kgateway.BackendConditionEndpointsDiscovered): {},
}

// BuildRouteStatus returns a newly constructed and fully defined RouteStatus for the supplied route object
// according to the state of the ReportMap and the current status of the route.
// The gwv1.RouteStatus returned will contain all non-gateway parents from the provided current route status
// along with the newly built kgw status per ReportMap, sorted in deterministic fashion.
// If the ReportMap does not have a RouteReport for the given route, e.g. because it did not encounter
// the route during translation, or the object is an unsupported route kind, nil is returned.
// Supported route types are: HTTPRoute, TCPRoute, TLSRoute, GRPCRoute
func (r *ReportMap) BuildRouteStatus(
	ctx context.Context,
	obj client.Object,
	controller string,
) *gwv1.RouteStatus {
	return r.BuildRouteStatusWithParentRefDefaulting(ctx, obj, controller, false)
}

func (r *ReportMap) BuildRouteStatusWithParentRefDefaulting(
	ctx context.Context,
	obj client.Object,
	controller string,
	defaultParentRef bool,
) *gwv1.RouteStatus {
	routeReport := r.route(obj)
	if routeReport == nil {
		slog.Info("missing route report", "type", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName(), "namespace", obj.GetNamespace())
		return nil
	}

	// Stamp the generation the report was built for, not the live object's. As
	// with Gateway status, the route is re-read by the syncer from a separate
	// cache than the one translation used; sourcing the generation from the
	// report keeps the sync trigger and the published value consistent and
	// avoids freezing observedGeneration on a cache skew.
	observedGeneration := routeReport.observedGeneration

	slog.Debug("building status", "type", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName(), "namespace", obj.GetNamespace())

	var existingStatus gwv1.RouteStatus
	// Default to using spec.ParentRefs when building the parent statuses for a route.
	// However, for delegatee (child) routes, the parentRefs field is optional and such routes
	// may not specify it. In this case, we infer the parentRefs form the RouteReport
	// corresponding to the delegatee (child) route as the route's report is associated to a parentRef.
	var parentRefs []gwv1.ParentReference
	switch route := obj.(type) {
	case *gwv1.HTTPRoute:
		existingStatus = route.Status.RouteStatus
		parentRefs = append(parentRefs, route.Spec.ParentRefs...)
		if len(parentRefs) == 0 {
			parentRefs = append(parentRefs, routeReport.parentRefs()...)
		}
	case *gwv1a2.TCPRoute:
		existingStatus = route.Status.RouteStatus
		parentRefs = append(parentRefs, route.Spec.ParentRefs...)
		if len(parentRefs) == 0 {
			parentRefs = append(parentRefs, routeReport.parentRefs()...)
		}
	case *gwv1.TLSRoute:
		existingStatus = route.Status.RouteStatus
		parentRefs = append(parentRefs, route.Spec.ParentRefs...)
		if len(parentRefs) == 0 {
			parentRefs = append(parentRefs, routeReport.parentRefs()...)
		}
	case *gwv1a2.TLSRoute:
		existingStatus = route.Status.RouteStatus
		parentRefs = append(parentRefs, route.Spec.ParentRefs...)
		if len(parentRefs) == 0 {
			parentRefs = append(parentRefs, routeReport.parentRefs()...)
		}
	case *gwv1.GRPCRoute:
		existingStatus = route.Status.RouteStatus
		parentRefs = append(parentRefs, route.Spec.ParentRefs...)
		if len(parentRefs) == 0 {
			parentRefs = append(parentRefs, routeReport.parentRefs()...)
		}
	default:
		slog.Error("unsupported route type for status reporting", "route_type", fmt.Sprintf("%T", obj))
		return nil
	}
	if defaultParentRef {
		parentRefs = ensureParentRefNamespaces(parentRefs, obj.GetNamespace())
	}

	newStatus := gwv1.RouteStatus{}
	// Process the parent references to build the RouteParentStatus
	for _, parentRef := range parentRefs {
		parentStatusReport := routeReport.getParentRefOrNil(&parentRef)
		if parentStatusReport == nil {
			// report doesn't have an entry for this parentRef, meaning we didn't translate it
			// probably because it's a parent that we don't control (e.g. Gateway from diff. controller)
			continue
		}
		parentConditions := parentRefConditionsWithDefaults(parentStatusReport.Conditions)

		// Get the status of the current parentRef conditions if they exist
		var currentParentRefConditions []metav1.Condition
		currentParentRefIdx := slices.IndexFunc(existingStatus.Parents, func(s gwv1.RouteParentStatus) bool {
			return reflect.DeepEqual(s.ParentRef, parentRef)
		})
		if currentParentRefIdx != -1 {
			currentParentRefConditions = existingStatus.Parents[currentParentRefIdx].Conditions
		}

		finalConditions := make([]metav1.Condition, 0, len(parentConditions))
		for _, pCondition := range parentConditions {
			pCondition.ObservedGeneration = observedGeneration

			// Copy old condition to preserve LastTransitionTime, if it exists
			if cond := meta.FindStatusCondition(currentParentRefConditions, pCondition.Type); cond != nil {
				finalConditions = append(finalConditions, *cond)
			}
			meta.SetStatusCondition(&finalConditions, pCondition)
		}
		// If there are conditions on the route that are not owned by our reporter, include
		// them in the final list of conditions to preseve conditions we do not own
		for _, condition := range currentParentRefConditions {
			if meta.FindStatusCondition(finalConditions, condition.Type) == nil {
				finalConditions = append(finalConditions, condition)
			}
		}

		routeParentStatus := gwv1.RouteParentStatus{
			ParentRef:      parentRef,
			ControllerName: gwv1.GatewayController(controller),
			Conditions:     finalConditions,
		}
		newStatus.Parents = append(newStatus.Parents, routeParentStatus)
	}

	// now we have a status object reflecting the state of translation according to our reportMap
	// let's add status from other controllers on the current object status
	var kgwStatus *gwv1.RouteStatus = &newStatus
	for _, rps := range existingStatus.Parents {
		if rps.ControllerName != gwv1.GatewayController(controller) {
			kgwStatus.Parents = append(kgwStatus.Parents, rps)
		}
	}

	// sort all parents for consistency with Equals and for Update
	// match sorting semantics of istio/istio, see:
	// https://github.com/istio/istio/blob/6dcaa0206bcaf20e3e3b4e45e9376f0f96365571/pilot/pkg/config/kube/gateway/conditions.go#L188-L193
	slices.SortStableFunc(kgwStatus.Parents, func(a, b gwv1.RouteParentStatus) int {
		return strings.Compare(ParentString(a.ParentRef), ParentString(b.ParentRef))
	})
	if newStatus.Parents == nil {
		// Kubernetes will not let us send "nil", so we need an empty
		newStatus.Parents = []gwv1.RouteParentStatus{}
	}

	return &newStatus
}

func ensureParentRefNamespaces(parentRefs []gwv1.ParentReference, routeNamespace string) []gwv1.ParentReference {
	return slices.Map(parentRefs, func(e gwv1.ParentReference) gwv1.ParentReference {
		if e.Namespace == nil {
			routeNs := gwv1.Namespace(routeNamespace)
			e.Namespace = &routeNs
		}
		if e.Group == nil {
			e.Group = new(gwv1.Group(wellknown.GatewayGVK.Group))
		}
		if e.Kind == nil {
			e.Kind = new(gwv1.Kind(wellknown.GatewayGVK.Kind))
		}
		return e
	})
}

// match istio/istio logic, see:
// https://github.com/istio/istio/blob/6dcaa0206bcaf20e3e3b4e45e9376f0f96365571/pilot/pkg/config/kube/gateway/conversion.go#L2714-L2722
func ParentString(ref gwv1.ParentReference) string {
	return fmt.Sprintf("%s/%s/%s/%s/%d.%s",
		ptr.OrEmpty(ref.Group),
		ptr.OrEmpty(ref.Kind),
		ref.Name,
		ptr.OrEmpty(ref.SectionName),
		ptr.OrEmpty(ref.Port),
		ptr.OrEmpty(ref.Namespace))
}

func gatewayConditionsWithDefaults(conditions []metav1.Condition, gw *gwv1.Gateway, listeners []gwv1.ListenerStatus) []metav1.Condition {
	out := slices.Clone(conditions)
	// If the existing Gateway status contains an Accepted=False with Reason=InvalidParameters,
	// we don't want to override it with a true Accepted status. The controller will set Accepted=True
	// when the GatewayParameters are valid again. Otherwise there is a race condition between the controller and reporter.
	// HACK: This is because both the controller and reporter set Accepted status.
	existingAccepted := meta.FindStatusCondition(gw.Status.Conditions, string(gwv1.GatewayConditionAccepted))
	hasInvalidParams := existingAccepted != nil && existingAccepted.Status == metav1.ConditionFalse && existingAccepted.Reason == string(gwv1.GatewayReasonInvalidParameters)
	if !hasInvalidParams && meta.FindStatusCondition(out, string(gwv1.GatewayConditionAccepted)) == nil {
		meta.SetStatusCondition(&out, metav1.Condition{
			Type:    string(gwv1.GatewayConditionAccepted),
			Status:  metav1.ConditionTrue,
			Reason:  string(gwv1.GatewayReasonAccepted),
			Message: GatewayAcceptedMessage,
		})
	}
	if cond := meta.FindStatusCondition(out, string(gwv1.GatewayConditionProgrammed)); cond == nil {
		meta.SetStatusCondition(&out, metav1.Condition{
			Type:    string(gwv1.GatewayConditionProgrammed),
			Status:  metav1.ConditionTrue,
			Reason:  string(gwv1.GatewayReasonProgrammed),
			Message: GatewayProgrammedMessage,
		})
	}
	if cond := meta.FindStatusCondition(out, string(gwv1.GatewayConditionResolvedRefs)); cond == nil {
		reason := gwv1.GatewayReasonResolvedRefs
		status := metav1.ConditionTrue
		message := GatewayResolvedRefsMessage
		for _, lisStatus := range listeners {
			lisResolvedRefs := meta.FindStatusCondition(lisStatus.Conditions, string(gwv1.ListenerConditionResolvedRefs))
			if lisResolvedRefs != nil && lisResolvedRefs.Status == metav1.ConditionFalse {
				reason = gwv1.GatewayReasonListenersNotResolved
				status = metav1.ConditionFalse
				message = GatewayListenersNotResolved
				break
			}
		}
		meta.SetStatusCondition(&out, metav1.Condition{
			Type:    string(gwv1.GatewayConditionResolvedRefs),
			Status:  status,
			Reason:  string(reason),
			Message: message,
		})
	}
	return out
}

// Reports will initially only contain negative conditions found during translation,
// so all missing conditions are assumed to be positive. Here we will add all missing conditions
// to a given report, i.e. set healthy conditions
func AddMissingListenerSetConditions(lsReport *ListenerSetReport) {
	lsReport.conditions = listenerSetConditionsWithDefaults(lsReport.conditions)
}

func listenerSetConditionsWithDefaults(conditions []metav1.Condition) []metav1.Condition {
	out := slices.Clone(conditions)
	if cond := meta.FindStatusCondition(out, string(gwv1.GatewayConditionAccepted)); cond == nil {
		meta.SetStatusCondition(&out, metav1.Condition{
			Type:    string(gwv1.GatewayConditionAccepted),
			Status:  metav1.ConditionTrue,
			Reason:  string(gwv1.GatewayReasonAccepted),
			Message: ListenerSetAcceptedMessage,
		})
	}
	if cond := meta.FindStatusCondition(out, string(gwv1.GatewayConditionProgrammed)); cond == nil {
		meta.SetStatusCondition(&out, metav1.Condition{
			Type:    string(gwv1.GatewayConditionProgrammed),
			Status:  metav1.ConditionTrue,
			Reason:  string(gwv1.GatewayReasonProgrammed),
			Message: ListenerSetProgrammedMessage,
		})
	}
	return out
}

// Reports will initially only contain negative conditions found during translation,
// so all missing conditions are assumed to be positive. Here we will add all missing conditions
// to a given report, i.e. set healthy conditions
func AddMissingListenerConditions(lisReport *ListenerReport) {
	lisReport.Status.Conditions = listenerConditionsWithDefaults(lisReport.Status.Conditions)
}

func listenerConditionsWithDefaults(conditions []metav1.Condition) []metav1.Condition {
	out := slices.Clone(conditions)
	// set healthy conditions for Condition Types not set yet (i.e. no negative status yet, we can assume positive)
	if cond := meta.FindStatusCondition(out, string(gwv1.ListenerConditionAccepted)); cond == nil {
		meta.SetStatusCondition(&out, metav1.Condition{
			Type:    string(gwv1.ListenerConditionAccepted),
			Status:  metav1.ConditionTrue,
			Reason:  string(gwv1.ListenerReasonAccepted),
			Message: ListenerAcceptedMessage,
		})
	}
	if cond := meta.FindStatusCondition(out, string(gwv1.ListenerConditionConflicted)); cond == nil {
		meta.SetStatusCondition(&out, metav1.Condition{
			Type:    string(gwv1.ListenerConditionConflicted),
			Status:  metav1.ConditionFalse,
			Reason:  string(gwv1.ListenerReasonNoConflicts),
			Message: ListenerNoConflictsMessage,
		})
	}
	if cond := meta.FindStatusCondition(out, string(gwv1.ListenerConditionResolvedRefs)); cond == nil {
		meta.SetStatusCondition(&out, metav1.Condition{
			Type:    string(gwv1.ListenerConditionResolvedRefs),
			Status:  metav1.ConditionTrue,
			Reason:  string(gwv1.ListenerReasonResolvedRefs),
			Message: ValidRefsMessage,
		})
	}
	if cond := meta.FindStatusCondition(out, string(gwv1.ListenerConditionProgrammed)); cond == nil {
		meta.SetStatusCondition(&out, metav1.Condition{
			Type:    string(gwv1.ListenerConditionProgrammed),
			Status:  metav1.ConditionTrue,
			Reason:  string(gwv1.ListenerReasonProgrammed),
			Message: ListenerProgrammedMessage,
		})
	}
	return out
}

func parentRefConditionsWithDefaults(conditions []metav1.Condition) []metav1.Condition {
	out := slices.Clone(conditions)
	if cond := meta.FindStatusCondition(out, string(gwv1.RouteConditionAccepted)); cond == nil {
		meta.SetStatusCondition(&out, metav1.Condition{
			Type:    string(gwv1.RouteConditionAccepted),
			Status:  metav1.ConditionTrue,
			Reason:  string(gwv1.RouteReasonAccepted),
			Message: RouteAcceptedMessage,
		})
	}
	if cond := meta.FindStatusCondition(out, string(gwv1.RouteConditionResolvedRefs)); cond == nil {
		meta.SetStatusCondition(&out, metav1.Condition{
			Type:    string(gwv1.RouteConditionResolvedRefs),
			Status:  metav1.ConditionTrue,
			Reason:  string(gwv1.RouteReasonResolvedRefs),
			Message: ValidRefsMessage,
		})
	}
	return out
}
