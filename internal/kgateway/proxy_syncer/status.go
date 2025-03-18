package proxy_syncer

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	plug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

type attachmentReport struct {
	Errors   []error
	Ancestor ir.ObjectSource
}

// policyObjsWithReports maps from a policy object to a list of ancestors it was attached to
// and the associated errors/conditions from the attachment/IR translation, if any
type policyObjsWithReports map[ir.AttachedPolicyRef][]attachmentReport

type GKPolicyReport struct {
	SeenPolicies map[schema.GroupKind]plug.PolicyReport
}

func (r GKPolicyReport) ResourceName() string {
	return "GKPolicyReport"
}

func comparePolicyErrors(i, j []error) bool {
	if !slices.Equal(i, j) {
		return false
	}
	return true
}
func comparePolicyWithAncestorReports(x, y plug.AncestorReports) bool {
	if !maps.EqualFunc(x, y, comparePolicyErrors) {
		return false
	}
	return true
}

func (r GKPolicyReport) Equals(in GKPolicyReport) bool {
	for gk, reports := range r.SeenPolicies {
		inreports, ok := in.SeenPolicies[gk]
		if !ok {
			return false
		}
		if !maps.EqualFunc(reports, inreports, comparePolicyWithAncestorReports) {
			return false
		}
	}
	return true
}

func generateBackendPolicyReport(backends []ir.BackendObjectIR) *GKPolicyReport {
	seenPolicyResources := policyObjsWithReports{}
	// iterate all backends and aggregate all policies attached to them
	// we track each attachment point of the policy to be tracked as an
	// ancestor for reporting status
	for _, backendObj := range backends {
		for _, polAtts := range backendObj.AttachedPolicies.Policies {
			for _, polAtt := range polAtts {
				if polAtt.PolicyRef == nil {
					// the policyRef may be nil in the case of virtual plugins (e.g. istio settings)
					// since there's no real policy object, we don't need to generate status for it
					continue
				}
				ar := attachmentReport{
					Ancestor: backendObj.ObjectSource,
					Errors:   polAtt.Errors,
				}
				reports := seenPolicyResources[*polAtt.PolicyRef]
				reports = append(reports, ar)
				seenPolicyResources[*polAtt.PolicyRef] = reports
			}
		}
	}

	// now generate a map keyed by each policy object we saw during attachment
	// and store the full set of ancestors/attachments associated with it
	policiesToAncestorReports := plug.PolicyReport{}
	for policyRef, reports := range seenPolicyResources {
		ancestorReports := map[ir.ObjectSource][]error{}
		for _, rpt := range reports {
			ancestorReports[rpt.Ancestor] = rpt.Errors
		}
		policiesToAncestorReports[policyRef] = ancestorReports
	}

	// finally, we group each policy obj with their associated reports
	// by GroupKind; this allows us to give each plugin a chance to process
	// policy reports for its own GroupKind
	seenPolsByGk := map[schema.GroupKind]plug.PolicyReport{}
	for policyRef, reports := range policiesToAncestorReports {
		gk := schema.GroupKind{
			Group: policyRef.Group,
			Kind:  policyRef.Kind,
		}
		gkPolsMap := seenPolsByGk[gk]
		if gkPolsMap == nil {
			gkPolsMap = map[ir.AttachedPolicyRef]plug.AncestorReports{}
		}
		gkPolsMap[policyRef] = reports
		seenPolsByGk[gk] = gkPolsMap
	}
	return &GKPolicyReport{
		SeenPolicies: seenPolsByGk,
	}
}

// TODO: find better location for this
func BuildPolicyCondition(polErrs []error) metav1.Condition {
	if len(polErrs) == 0 {
		return metav1.Condition{
			Type:    string(gwv1a2.PolicyConditionAccepted),
			Status:  metav1.ConditionTrue,
			Reason:  string(gwv1a2.PolicyReasonAccepted),
			Message: "Policy accepted and attached",
		}
	}
	var aggErrs strings.Builder
	var prologue string
	if len(polErrs) == 1 {
		prologue = "Policy error:"
	} else {
		prologue = fmt.Sprintf("Policy has %d errors:", len(polErrs))
	}
	aggErrs.Write([]byte(prologue))
	for _, err := range polErrs {
		aggErrs.Write([]byte(` "`))
		aggErrs.Write([]byte(err.Error()))
		aggErrs.Write([]byte(`"`))
	}
	return metav1.Condition{
		Type:    string(gwv1a2.PolicyConditionAccepted),
		Status:  metav1.ConditionFalse,
		Reason:  string(gwv1a2.PolicyReasonInvalid),
		Message: aggErrs.String(),
	}
}
