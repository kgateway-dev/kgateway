package reports

import (
	"context"
	"slices"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func (r *ReportMap) BuildGWStatus(ctx context.Context, gw gwv1.Gateway) gwv1.GatewayStatus {
	gwReport := r.Gateway(&gw)
	//TODO(Law): deterministic sorting
	finalListeners := make([]gwv1.ListenerStatus, 0, len(gw.Spec.Listeners))
	for _, lis := range gw.Spec.Listeners {
		lisReport := gwReport.listener(&lis)
		addMissingListenerConditions(lisReport)

		finalConditions := make([]metav1.Condition, 0)
		oldLisStatusIndex := slices.IndexFunc(gw.Status.Listeners, func(l gwv1.ListenerStatus) bool {
			return l.Name == lis.Name
		})
		for _, lisCondition := range lisReport.Status.Conditions {
			// the report was generated over a single pass of translation, safe to set generation here
			// assuming statuses are synced and reported in the same pass
			lisCondition.ObservedGeneration = gw.Generation

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
		// the report was generated over a single pass of translation, safe to set generation here
		// assuming statuses are synced and reported in the same pass
		gwCondition.ObservedGeneration = gw.Generation

		// copy old condition from gw so LastTransitionTime is set correctly below by SetStatusCondition()
		if cond := meta.FindStatusCondition(gw.Status.Conditions, gwCondition.Type); cond != nil {
			finalConditions = append(finalConditions, *cond)
		}
		meta.SetStatusCondition(&finalConditions, gwCondition)
	}

	finalGwStatus := gwv1.GatewayStatus{}
	finalGwStatus.Conditions = finalConditions
	finalGwStatus.Listeners = finalListeners
	return finalGwStatus
}

func (r *ReportMap) BuildRouteStatus(ctx context.Context, route gwv1.HTTPRoute, cName string) gwv1.HTTPRouteStatus {
	routeReport, ok := r.Routes[client.ObjectKeyFromObject(&route)]
	if !ok {
		//TODO more thought here
	}
	routeStatus := gwv1.RouteStatus{}
	for _, parentRef := range route.Spec.ParentRefs {
		key := GetParentRefKey(&parentRef)
		parentStatus, ok := routeReport.Parents[key]
		if !ok {
			//todo think
			continue
		}
		if cond := meta.FindStatusCondition(parentStatus.Conditions, string(gwv1.RouteConditionAccepted)); cond == nil {
			parentStatus.SetCondition(HTTPRouteCondition{
				Type:   gwv1.RouteConditionAccepted,
				Status: metav1.ConditionTrue,
				Reason: gwv1.RouteReasonAccepted,
			})
		}
		if cond := meta.FindStatusCondition(parentStatus.Conditions, string(gwv1.RouteConditionResolvedRefs)); cond == nil {
			parentStatus.SetCondition(HTTPRouteCondition{
				Type:   gwv1.RouteConditionResolvedRefs,
				Status: metav1.ConditionTrue,
				Reason: gwv1.RouteReasonResolvedRefs,
			})
		}

		//TODO add logic for partially invalid condition

		finalConditions := make([]metav1.Condition, 0)
		for _, pCondition := range parentStatus.Conditions {
			pCondition.ObservedGeneration = route.Generation           // don't have generation is the report, should consider adding it
			pCondition.LastTransitionTime = metav1.NewTime(time.Now()) // same as above, should calculate at report time possibly
			finalConditions = append(finalConditions, pCondition)
		}

		routeParentStatus := gwv1.RouteParentStatus{
			ParentRef:      parentRef,
			ControllerName: gwv1.GatewayController(cName),
			Conditions:     finalConditions,
		}
		routeStatus.Parents = append(routeStatus.Parents, routeParentStatus)
	}
	return gwv1.HTTPRouteStatus{
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
