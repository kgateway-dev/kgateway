package reports

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
)

type AncestorRefReport struct {
	Conditions      []metav1.Condition
	AttachmentState reporter.PolicyAttachmentState
}

type PolicyReport struct {
	Ancestors          map[ParentRefKey]*AncestorRefReport
	observedGeneration int64
}

func (r *PolicyReport) AncestorRef(ref gwv1.ParentReference) reporter.AncestorRefReporter {
	return r.ancestorRef(ref)
}

func (prr *AncestorRefReport) SetCondition(c reporter.PolicyCondition) {
	condition := metav1.Condition{
		Type:    c.Type,
		Status:  c.Status,
		Reason:  c.Reason,
		Message: c.Message,
	}
	meta.SetStatusCondition(&prr.Conditions, condition)
}

func (prr *AncestorRefReport) SetAttachmentState(
	state reporter.PolicyAttachmentState,
) {
	prr.AttachmentState |= state
}

func (r *statusReporter) Policy(key PolicyKey, observedGeneration int64) reporter.PolicyReporter {
	pr := r.report.policy(key)
	if pr == nil {
		pr = r.report.newPolicyReport(key, observedGeneration)
	}
	return pr
}

func (r *ReportMap) policy(key PolicyKey) *PolicyReport {
	return r.Policies[key]
}

func (r *ReportMap) newPolicyReport(key PolicyKey, observedGeneration int64) *PolicyReport {
	pr := &PolicyReport{
		observedGeneration: observedGeneration,
	}
	r.Policies[key] = pr
	return pr
}

func (r *PolicyReport) ancestorRef(parentRef gwv1.ParentReference) *AncestorRefReport {
	key := getParentRefKey(&parentRef)
	if r.Ancestors == nil {
		r.Ancestors = make(map[ParentRefKey]*AncestorRefReport)
	}
	var prr *AncestorRefReport
	prr, ok := r.Ancestors[key]
	if !ok {
		prr = &AncestorRefReport{}
		r.Ancestors[key] = prr
	}
	return prr
}

// ancestorRefs returns a list of ParentReferences associated with the PolicyReport.
func (r *PolicyReport) ancestorRefs() []gwv1.ParentReference {
	var refs []gwv1.ParentReference
	for key := range r.Ancestors {
		var ns *gwv1.Namespace
		if key.Namespace != "" {
			ns = ptr.To(gwv1.Namespace(key.Namespace))
		}
		parentRef := gwv1.ParentReference{
			Group:     ptr.To(gwv1.Group(key.Group)),
			Kind:      ptr.To(gwv1.Kind(key.Kind)),
			Name:      gwv1.ObjectName(key.Name),
			Namespace: ns,
		}
		refs = append(refs, parentRef)
	}
	return refs
}

func (r *ReportMap) BuildPolicyStatus(
	ctx context.Context,
	key PolicyKey,
	controller string,
	currentStatus gwv1alpha2.PolicyStatus,
) *gwv1alpha2.PolicyStatus {
	report := r.policy(key)
	if report == nil {
		// no report for this policy
		return nil
	}

	ancestorRefs := report.ancestorRefs()
	status := gwv1alpha2.PolicyStatus{}

	// Process the parent references to build the RouteParentStatus
	for _, ancestorRef := range ancestorRefs {
		parentStatusReport := report.getAncestorRefOrNil(&ancestorRef)
		if parentStatusReport == nil {
			// report doesn't have an entry for this parentRef, meaning we didn't translate it
			// probably because it's a parent that we don't control (e.g. Gateway from diff. controller)
			continue
		}
		addMissingAncestorRefConditions(parentStatusReport)

		// Get the status of the current parentRef conditions if they exist
		var currentParentRefConditions []metav1.Condition
		currentParentRefIdx := slices.IndexFunc(currentStatus.Ancestors, func(s gwv1alpha2.PolicyAncestorStatus) bool {
			return reflect.DeepEqual(s.AncestorRef, ancestorRef)
		})
		if currentParentRefIdx != -1 {
			currentParentRefConditions = currentStatus.Ancestors[currentParentRefIdx].Conditions
		}

		// Build and append the Attached Condition.Type
		existingConditions := addAttachmentCondition(parentStatusReport)

		finalConditions := make([]metav1.Condition, 0, len(existingConditions))
		for _, pCondition := range existingConditions {
			pCondition.ObservedGeneration = report.observedGeneration

			// Copy old condition to preserve LastTransitionTime, if it exists
			if cond := meta.FindStatusCondition(currentParentRefConditions, pCondition.Type); cond != nil {
				finalConditions = append(finalConditions, *cond)
			}
			meta.SetStatusCondition(&finalConditions, pCondition)
		}
		// If there are conditions on the route that are not owned by our reporter, include
		// them in the final list of conditions to preserve conditions we do not own
		for _, condition := range currentParentRefConditions {
			if meta.FindStatusCondition(finalConditions, condition.Type) == nil {
				finalConditions = append(finalConditions, condition)
			}
		}

		ancestorStatus := gwv1alpha2.PolicyAncestorStatus{
			AncestorRef:    ancestorRef,
			ControllerName: gwv1.GatewayController(controller),
			Conditions:     finalConditions,
		}
		status.Ancestors = append(status.Ancestors, ancestorStatus)
	}

	// now we have a status object reflecting the state of translation according to our reportMap
	// let's add status from other controllers on the current object status
	for _, ancestor := range currentStatus.Ancestors {
		if ancestor.ControllerName != gwv1.GatewayController(controller) {
			status.Ancestors = append(status.Ancestors, ancestor)
		}
	}

	// sort all parents for consistency with Equals and for Update
	// match sorting semantics of istio/istio, see:
	// https://github.com/istio/istio/blob/6dcaa0206bcaf20e3e3b4e45e9376f0f96365571/pilot/pkg/config/kube/gateway/conditions.go#L188-L193
	slices.SortStableFunc(status.Ancestors, func(a, b gwv1alpha2.PolicyAncestorStatus) int {
		return strings.Compare(ParentString(a.AncestorRef), ParentString(b.AncestorRef))
	})

	// TODO: ensure status.Ancestors is bounded by the max allowed limit, currently 16
	if len(status.Ancestors) > 15 {
		ignored := status.Ancestors[15:]
		status.Ancestors = status.Ancestors[:15]
		status.Ancestors = append(status.Ancestors, gwv1alpha2.PolicyAncestorStatus{
			AncestorRef: gwv1.ParentReference{
				Group: ptr.To(gwv1.Group("gateway.kgateway.dev")),
				Name:  "StatusSummary",
			},
			ControllerName: gwv1.GatewayController(controller),
			Conditions: []metav1.Condition{
				{
					Type:    "StatusSummarized",
					Status:  metav1.ConditionTrue,
					Reason:  "StatusSummary",
					Message: fmt.Sprintf("%d AncestorRefs ignored due to max status size", len(ignored)),
				},
			},
		})
	}

	return &status
}

// getAncestorRefOrNil returns a ParentRefReport for the given parentRef if and only if
// that parentRef exists in the report (i.e. the parentRef was encountered during translation)
// If no report is found, nil is returned, signaling this parentRef is unknown to the report
func (r *PolicyReport) getAncestorRefOrNil(parentRef *gwv1.ParentReference) *AncestorRefReport {
	key := getParentRefKey(parentRef)
	if r.Ancestors == nil {
		r.Ancestors = make(map[ParentRefKey]*AncestorRefReport)
	}
	return r.Ancestors[key]
}

// addMissingAncestorRefConditions initializes the AncestorRefReport with a default Pending
// condition reason for the Accepted and Attached conditions.
// Positive conditions will be added when the policy is processed and attached to targeted resources.
func addMissingAncestorRefConditions(report *AncestorRefReport) {
	if cond := meta.FindStatusCondition(report.Conditions, string(v1alpha1.PolicyConditionAccepted)); cond == nil {
		meta.SetStatusCondition(&report.Conditions, metav1.Condition{
			Type:   string(v1alpha1.PolicyConditionAccepted),
			Status: metav1.ConditionFalse,
			Reason: string(v1alpha1.PolicyReasonPending),
		})
	}
	if cond := meta.FindStatusCondition(report.Conditions, string(v1alpha1.PolicyConditionAttached)); cond == nil {
		meta.SetStatusCondition(&report.Conditions, metav1.Condition{
			Type:   string(v1alpha1.PolicyConditionAttached),
			Status: metav1.ConditionFalse,
			Reason: string(v1alpha1.PolicyReasonPending),
		})
	}
}

func addAttachmentCondition(report *AncestorRefReport) []metav1.Condition {
	if report.AttachmentState == reporter.PolicyAttachmentStatePending {
		// no attachment state set, return the conditions as is
		return report.Conditions
	}

	// avoid modifying the existing Conditions on the report
	existing := slices.Clone(report.Conditions)

	switch {
	case report.AttachmentState.Has(reporter.PolicyAttachmentStateOverridden):
		meta.SetStatusCondition(&existing, metav1.Condition{
			Type:    string(v1alpha1.PolicyConditionAttached),
			Status:  metav1.ConditionFalse,
			Reason:  string(v1alpha1.PolicyReasonOverridden),
			Message: reporter.PolicyOverriddenMsg,
		})

	case report.AttachmentState.Has(reporter.PolicyAttachmentStateMerged):
		meta.SetStatusCondition(&existing, metav1.Condition{
			Type:    string(v1alpha1.PolicyConditionAttached),
			Status:  metav1.ConditionTrue,
			Reason:  string(v1alpha1.PolicyReasonMerged),
			Message: reporter.PolicyMergedMsg,
		})

	case report.AttachmentState.Has(reporter.PolicyAttachmentStateAttached):
		meta.SetStatusCondition(&existing, metav1.Condition{
			Type:    string(v1alpha1.PolicyConditionAttached),
			Status:  metav1.ConditionTrue,
			Reason:  string(v1alpha1.PolicyReasonAttached),
			Message: reporter.PolicyAttachedMsg,
		})
	}

	return existing
}
