package proxy_syncer

import (
	"maps"
	"slices"

	"k8s.io/apimachinery/pkg/runtime/schema"

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
	// SeenPolicies maps from schema.GroupKind.String() to a PolicyReport for
	// all encountered policies of that GroupKind
	SeenPolicies map[string]plug.PolicyReport
}

type ObjWithAttachedPolicies interface {
	GetAttachedPolicies() ir.AttachedPolicies
	GetObjectSource() ir.ObjectSource
}

func convertBackends(backends []ir.BackendObjectIR) []ObjWithAttachedPolicies {
	objs := make([]ObjWithAttachedPolicies, 0, len(backends))
	for _, backend := range backends {
		objs = append(objs, backend)
	}
	return objs
}

var _ ObjWithAttachedPolicies = ir.BackendObjectIR{}

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

func generatePolicyReport(in []ObjWithAttachedPolicies) *GKPolicyReport {
	seenPolicyResources := policyObjsWithReports{}
	// iterate all backends and aggregate all policies attached to them
	// we track each attachment point of the policy to be tracked as an
	// ancestor for reporting status
	for _, obj := range in {
		for _, polAtts := range obj.GetAttachedPolicies().Policies {
			for _, polAtt := range polAtts {
				if polAtt.PolicyRef == nil {
					// the policyRef may be nil in the case of virtual plugins (e.g. istio settings)
					// since there's no real policy object, we don't need to generate status for it
					continue
				}
				ar := attachmentReport{
					Ancestor: obj.GetObjectSource(),
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
	seenPolsByGk := map[string]plug.PolicyReport{}
	for policyRef, reports := range policiesToAncestorReports {
		gk := schema.GroupKind{
			Group: policyRef.Group,
			Kind:  policyRef.Kind,
		}
		gkStr := gk.String()
		gkPolsMap := seenPolsByGk[gkStr]
		if gkPolsMap == nil {
			gkPolsMap = map[ir.AttachedPolicyRef]plug.AncestorReports{}
		}
		gkPolsMap[policyRef] = reports
		seenPolsByGk[gkStr] = gkPolsMap
	}
	return &GKPolicyReport{
		SeenPolicies: seenPolsByGk,
	}
}
