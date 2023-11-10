package reports

import (
	"context"
	"slices"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func (r *ReportMap) BuildGWStatus(ctx context.Context, gw gwv1.Gateway) gwv1.GatewayStatus {
	key := client.ObjectKeyFromObject(&gw)
	gwReport := r.GetGateway(key)

	//TODO(Law): deterministic sorting
	finalListeners := make([]gwv1.ListenerStatus, 0)
	for _, lis := range gw.Spec.Listeners {
		lisReport := gwReport.GetListenerReport(string(lis.Name))

		addMissingListenerConditions(lisReport)
		finalConditions := make([]metav1.Condition, 0)
		oldLisStatusIndex := slices.IndexFunc(gw.Status.Listeners, func(l gwv1.ListenerStatus) bool {
			return l.Name == lis.Name
		})
		for _, lisCondition := range lisReport.Status.Conditions {
			lisCondition.ObservedGeneration = gw.Generation // don't have generation is the report, should consider adding it
			// copy the old condition from the gw so last transition time is set correctly
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

	addMissingGatewayConditions(gwReport)
	// recalculate top-level GatewayStatus
	finalConditions := make([]metav1.Condition, 0)
	for _, gwCondition := range gwReport.GetConditions() {
		gwCondition.ObservedGeneration = gw.Generation
		// copy the old condition from the gw so last transition time is set correctly
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

func addMissingGatewayConditions(gwReport *GatewayReport) {
	// set missing conditions, i.e. set healthy conditions
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
