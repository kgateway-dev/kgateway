package reports

import (
	"context"
	"reflect"
	"slices"

	"github.com/solo-io/go-utils/contextutils"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var (
	missingGatewayReportErr = "building status for Gateway '%s' (namespace: '%s') but no GatewayReport was present"
	missingRouteReportErr   = "building status for HTTPRoute '%s' (namespace: '%s') but no RouteReport was present"
)

func (r *ReportMap) BuildGWStatus(ctx context.Context, gw gwv1.Gateway) *gwv1.GatewayStatus {
	gwReport := r.Gateway(&gw)
	if gwReport == nil {
		// If the gwReport for a gateway we translated is missing, something is not correct in the reporting flow.
		// If we hit this DPanic() we need to understand what has changed in the flow where we are translating Gateways but
		// not initializing a report for it.
		contextutils.LoggerFrom(ctx).DPanicf(missingGatewayReportErr, gw.Name, gw.Namespace)
		return nil
	}

	finalListeners := make([]gwv1.ListenerStatus, 0, len(gw.Spec.Listeners))
	for _, lis := range gw.Spec.Listeners {
		lisReport := gwReport.listener(&lis)
		addMissingListenerConditions(lisReport)

		finalConditions := make([]metav1.Condition, 0, len(lisReport.Status.Conditions))
		oldLisStatusIndex := slices.IndexFunc(gw.Status.Listeners, func(l gwv1.ListenerStatus) bool {
			return l.Name == lis.Name
		})
		for _, lisCondition := range lisReport.Status.Conditions {
			lisCondition.ObservedGeneration = gwReport.observedGeneration

			// copy old condition from gw so LastTransitionTime is set correctly below by SetStatusCondition()
			if oldLisStatusIndex != -1 {
				if cond := meta.FindStatusCondition(gw.Status.Listeners[oldLisStatusIndex].Conditions, lisCondition.Type); cond != nil {
					finalConditions = append(finalConditions, *cond)
				}
			}
			meta.SetStatusCondition(&finalConditions, lisCondition)
		}
		lisReport.Status.Conditions = finalConditions
		finalListeners = append(finalListeners, lisReport.Status)
	}

	addMissingGatewayConditions(r.Gateway(&gw))

	finalConditions := make([]metav1.Condition, 0)
	for _, gwCondition := range gwReport.GetConditions() {
		gwCondition.ObservedGeneration = gwReport.observedGeneration

		// copy old condition from gw so LastTransitionTime is set correctly below by SetStatusCondition()
		if cond := meta.FindStatusCondition(gw.Status.Conditions, gwCondition.Type); cond != nil {
			finalConditions = append(finalConditions, *cond)
		}
		meta.SetStatusCondition(&finalConditions, gwCondition)
	}

	finalGwStatus := gwv1.GatewayStatus{}
	finalGwStatus.Conditions = finalConditions
	finalGwStatus.Listeners = finalListeners
	return &finalGwStatus
}

// BuildRouteStatus returns a newly constructed and fully defined HTTPRouteStatus for the supplied route
// according to the state of the ReportMap. If the ReportMap does not have a RouteReport for the given HTTPRoute,
// e.g. because it did not encounter the route during translation, nil is returned
func (r *ReportMap) BuildRouteStatus(ctx context.Context, route gwv1.HTTPRoute, cName string) *gwv1.HTTPRouteStatus {
	routeReport := r.route(&route)
	if routeReport == nil {
		// a route report may be missing because of the disconnect between when routes are retrieved for translation,
		// which the query engine performs inside gateway_translator.go/TranslateProxy(), and when the list of routes
		// for status syncing is retrieved after translation, separately in xds_syncer.go/syncRouteStatus().
		// Since there may have been additions/deletions in that window, a missing route report will just be treated
		// as informational and we will return nil, signaling to status syncer to not touch this Routes status.
		contextutils.LoggerFrom(ctx).Infof(missingRouteReportErr, route.Name, route.Namespace)
		return nil
	}
	contextutils.LoggerFrom(ctx).Infof("building status for route %s/%s", route.Namespace, route.Name)

	routeStatus := gwv1.RouteStatus{}

	// Default to using spec.ParentRefs when building the parent statuses for a route.
	// However, for delegatee (child) routes, the parentRefs field is optional and such routes
	// may not specify it. In this case, we infer the parentRefs form the RouteReport
	// corresponding to the delegatee (child) route as the route's report is associated to a parentRef.
	var parentRefs []gwv1.ParentReference
	parentRefs = append(parentRefs, route.Spec.ParentRefs...)
	if len(parentRefs) == 0 {
		parentRefs = append(parentRefs, routeReport.parentRefs()...)
	}

	for _, parentRef := range parentRefs {
		parentStatusReport := routeReport.parentRef(&parentRef)
		addMissingParentRefConditions(parentStatusReport)

		// get status of current parentRef status if it exists
		var currentParentRefConditions []metav1.Condition
		currentParentRefIdx := slices.IndexFunc(route.Status.Parents, func(s gwv1.RouteParentStatus) bool {
			return reflect.DeepEqual(s.ParentRef, parentRef)
		})
		if currentParentRefIdx != -1 {
			currentParentRefConditions = route.Status.Parents[currentParentRefIdx].Conditions
		}

		finalConditions := make([]metav1.Condition, 0, len(parentStatusReport.Conditions))
		for _, pCondition := range parentStatusReport.Conditions {
			pCondition.ObservedGeneration = routeReport.observedGeneration

			// copy old condition from gw so LastTransitionTime is set correctly below by SetStatusCondition()
			if cond := meta.FindStatusCondition(currentParentRefConditions, pCondition.Type); cond != nil {
				finalConditions = append(finalConditions, *cond)
			}
			meta.SetStatusCondition(&finalConditions, pCondition)
		}

		routeParentStatus := gwv1.RouteParentStatus{
			ParentRef:      parentRef,
			ControllerName: gwv1.GatewayController(cName),
			Conditions:     finalConditions,
		}
		routeStatus.Parents = append(routeStatus.Parents, routeParentStatus)
	}

	return &gwv1.HTTPRouteStatus{
		RouteStatus: routeStatus,
	}
}

// Reports will initially only contain negative conditions found during translation,
// so all missing conditions are assumed to be positive. Here we will add all missing conditions
// to a given report, i.e. set healthy conditions
func addMissingGatewayConditions(gwReport *GatewayReport) {
	if cond := meta.FindStatusCondition(gwReport.GetConditions(), string(gwv1.GatewayConditionAccepted)); cond == nil {
		gwReport.SetCondition(GatewayCondition{
			Type:   gwv1.GatewayConditionAccepted,
			Status: metav1.ConditionTrue,
			Reason: gwv1.GatewayReasonAccepted,
		})
	}
	if cond := meta.FindStatusCondition(gwReport.GetConditions(), string(gwv1.GatewayConditionProgrammed)); cond == nil {
		gwReport.SetCondition(GatewayCondition{
			Type:   gwv1.GatewayConditionProgrammed,
			Status: metav1.ConditionTrue,
			Reason: gwv1.GatewayReasonProgrammed,
		})
	}
}

// Reports will initially only contain negative conditions found during translation,
// so all missing conditions are assumed to be positive. Here we will add all missing conditions
// to a given report, i.e. set healthy conditions
func addMissingListenerConditions(lisReport *ListenerReport) {
	// set healthy conditions for Condition Types not set yet (i.e. no negative status yet, we can assume positive)
	if cond := meta.FindStatusCondition(lisReport.Status.Conditions, string(gwv1.ListenerConditionAccepted)); cond == nil {
		lisReport.SetCondition(ListenerCondition{
			Type:   gwv1.ListenerConditionAccepted,
			Status: metav1.ConditionTrue,
			Reason: gwv1.ListenerReasonAccepted,
		})
	}
	if cond := meta.FindStatusCondition(lisReport.Status.Conditions, string(gwv1.ListenerConditionConflicted)); cond == nil {
		lisReport.SetCondition(ListenerCondition{
			Type:   gwv1.ListenerConditionConflicted,
			Status: metav1.ConditionFalse,
			Reason: gwv1.ListenerReasonNoConflicts,
		})
	}
	if cond := meta.FindStatusCondition(lisReport.Status.Conditions, string(gwv1.ListenerConditionResolvedRefs)); cond == nil {
		lisReport.SetCondition(ListenerCondition{
			Type:   gwv1.ListenerConditionResolvedRefs,
			Status: metav1.ConditionTrue,
			Reason: gwv1.ListenerReasonResolvedRefs,
		})
	}
	if cond := meta.FindStatusCondition(lisReport.Status.Conditions, string(gwv1.ListenerConditionProgrammed)); cond == nil {
		lisReport.SetCondition(ListenerCondition{
			Type:   gwv1.ListenerConditionProgrammed,
			Status: metav1.ConditionTrue,
			Reason: gwv1.ListenerReasonProgrammed,
		})
	}
}

// Reports will initially only contain negative conditions found during translation,
// so all missing conditions are assumed to be positive. Here we will add all missing conditions
// to a given report, i.e. set healthy conditions
func addMissingParentRefConditions(report *ParentRefReport) {
	if cond := meta.FindStatusCondition(report.Conditions, string(gwv1.RouteConditionAccepted)); cond == nil {
		report.SetCondition(HTTPRouteCondition{
			Type:   gwv1.RouteConditionAccepted,
			Status: metav1.ConditionTrue,
			Reason: gwv1.RouteReasonAccepted,
		})
	}
	if cond := meta.FindStatusCondition(report.Conditions, string(gwv1.RouteConditionResolvedRefs)); cond == nil {
		report.SetCondition(HTTPRouteCondition{
			Type:   gwv1.RouteConditionResolvedRefs,
			Status: metav1.ConditionTrue,
			Reason: gwv1.RouteReasonResolvedRefs,
		})
	}
}
