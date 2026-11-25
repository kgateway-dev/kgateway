package pluginsdk

import (
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// fieldsIR is a fake policy whose fields are named values.
type fieldsIR struct {
	ct     time.Time
	fields map[string]string
}

func (f *fieldsIR) CreationTime() time.Time { return f.ct }
func (f *fieldsIR) Equals(in any) bool {
	o, ok := in.(*fieldsIR)
	return ok && f.ct.Equal(o.ct) && maps.Equal(f.fields, o.fields)
}

var (
	kindA = schema.GroupKind{Group: "a.example", Kind: "PolicyA"}
	kindB = schema.GroupKind{Group: "b.example", Kind: "PolicyB"}
	t2020 = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	t2021 = time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	t2022 = time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
)

// mergeOlderWins merges fieldsIR policies field by field, the older policy winning, and
// records which policy supplied each field, like the real per-field merges.
func mergeOlderWins(pols []ir.PolicyAtt) ir.PolicyAtt {
	sorted := slices.Clone(pols)
	slices.SortFunc(sorted, ir.ComparePoliciesByCreationTimeAndRef)
	out := ir.PolicyAtt{GroupKind: kindA, PolicyIr: &fieldsIR{fields: map[string]string{}}, MergeOrigins: ir.MergeOrigins{}}
	merged := out.PolicyIr.(*fieldsIR)
	for _, p := range sorted {
		out.Errors = append(out.Errors, p.Errors...)
		if len(p.Errors) > 0 {
			continue
		}
		for f, v := range p.PolicyIr.(*fieldsIR).fields {
			if _, ok := merged.fields[f]; ok {
				continue
			}
			merged.fields[f] = v
			out.MergeOrigins.SetOne(f, p.PolicyRef, nil)
		}
	}
	return out
}

func pluginA(clearable bool) PolicyPlugin {
	p := PolicyPlugin{
		MergePolicies: mergeOlderWins,
		SetFields: func(pol ir.PolicyIR) []string {
			return slices.Sorted(maps.Keys(pol.(*fieldsIR).fields))
		},
	}
	if clearable {
		p.ClearFields = func(pol ir.PolicyIR, fields []string) ir.PolicyIR {
			c := &fieldsIR{ct: pol.(*fieldsIR).ct, fields: maps.Clone(pol.(*fieldsIR).fields)}
			for _, f := range fields {
				delete(c.fields, f)
			}
			return c
		}
	}
	return p
}

// pluginB picks its oldest policy and owns field "x" of kind A.
var pluginB = PolicyPlugin{
	MergePolicies: func(pols []ir.PolicyAtt) ir.PolicyAtt {
		return pols[ir.WinnerPolicyIndexByCreationTimeAndRef(pols)]
	},
	Supersedes: map[schema.GroupKind][]string{kindA: {"x"}},
}

func att(gk schema.GroupKind, name string, ct time.Time, fields map[string]string, errs ...error) ir.PolicyAtt {
	return ir.PolicyAtt{
		GroupKind: gk,
		PolicyRef: &ir.AttachedPolicyRef{Group: gk.Group, Kind: gk.Kind, Namespace: "ns", Name: name},
		PolicyIr:  &fieldsIR{ct: ct, fields: fields},
		Errors:    errs,
	}
}

func attached(byKind map[schema.GroupKind][]ir.PolicyAtt) ir.AttachedPolicies {
	return ir.AttachedPolicies{Policies: byKind}
}

func appliedFields(g Gomega, res *ir.BackendPolicyResolution, gk schema.GroupKind) map[string]string {
	eff, ok := res.Effective(gk)
	g.Expect(ok).To(BeTrue(), "kind %s should resolve to one policy", gk.Kind)
	return eff.PolicyIr.(*fieldsIR).fields
}

func TestResolveBackendPoliciesMergeLoss(t *testing.T) {
	g := NewWithT(t)
	older := att(kindA, "older", t2020, map[string]string{"x": "older", "y": "older"})
	newer := att(kindA, "newer", t2021, map[string]string{"x": "newer", "z": "newer"})

	res := ResolveBackendPolicies(attached(map[schema.GroupKind][]ir.PolicyAtt{kindA: {newer, older}}),
		ContributesPolicies{kindA: pluginA(true)})

	g.Expect(appliedFields(g, res, kindA)).To(Equal(map[string]string{"x": "older", "y": "older", "z": "newer"}))
	g.Expect(res.Dropped(older.PolicyRef)).To(BeEmpty(), "every field of the older policy applies")
	g.Expect(res.Dropped(newer.PolicyRef)).To(Equal([]ir.DroppedField{
		{Field: "x", Kind: kindA, By: older.PolicyRef, ByValid: true, Reason: ir.DroppedByMerge},
	}), "the newer policy loses x to the older one, and keeps z")
}

func TestResolveBackendPoliciesSupersede(t *testing.T) {
	older := att(kindA, "older", t2020, map[string]string{"x": "older", "y": "older"})
	newer := att(kindA, "newer", t2021, map[string]string{"x": "newer", "z": "newer"})
	tests := []struct {
		name      string
		btps      []ir.PolicyAtt
		wantBy    string
		wantValid bool
	}{
		{name: "valid owner", btps: []ir.PolicyAtt{att(kindB, "b", t2022, nil)}, wantBy: "b", wantValid: true},
		{
			// The oldest owner wins even when invalid; translation blackholes the cluster.
			name:   "older invalid owner wins over newer valid one",
			btps:   []ir.PolicyAtt{att(kindB, "newer-b", t2022, nil), att(kindB, "older-b", t2021, nil, errors.New("bad"))},
			wantBy: "older-b",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			res := ResolveBackendPolicies(attached(map[schema.GroupKind][]ir.PolicyAtt{kindA: {newer, older}, kindB: tt.btps}),
				ContributesPolicies{kindA: pluginA(true), kindB: pluginB})

			g.Expect(appliedFields(g, res, kindA)).To(Equal(map[string]string{"y": "older", "z": "newer"}),
				"x is cleared from kind A's merged policy")
			g.Expect(res.IsSuperseded(kindA, "x")).To(BeTrue())
			eff, _ := res.Effective(kindA)
			g.Expect(eff.MergeOrigins.Get("x")).To(BeEmpty(), "merge origins should not credit a cleared field")
			for _, p := range []ir.PolicyAtt{older, newer} {
				dropped := res.Dropped(p.PolicyRef)
				g.Expect(dropped).To(HaveLen(1), "%s should lose only x", p.PolicyRef.Name)
				g.Expect(dropped[0].Field).To(Equal("x"))
				g.Expect(dropped[0].Kind).To(Equal(kindB))
				g.Expect(dropped[0].By.Name).To(Equal(tt.wantBy))
				g.Expect(dropped[0].ByValid).To(Equal(tt.wantValid))
				g.Expect(dropped[0].Reason).To(Equal(ir.DroppedBySupersede))
			}
			g.Expect(older.PolicyIr.(*fieldsIR).fields).To(HaveKey("x"), "attached policies are shared and must not change")
		})
	}
}

func TestResolveBackendPoliciesSupersedeNeedsClearFields(t *testing.T) {
	g := NewWithT(t)
	a := att(kindA, "a", t2020, map[string]string{"x": "a"})

	res := ResolveBackendPolicies(attached(map[schema.GroupKind][]ir.PolicyAtt{kindA: {a}, kindB: {att(kindB, "b", t2021, nil)}}),
		ContributesPolicies{kindA: pluginA(false), kindB: pluginB})

	g.Expect(res.IsSuperseded(kindA, "x")).To(BeFalse(), "a field the target cannot clear would still be applied")
	g.Expect(res.Dropped(a.PolicyRef)).To(BeEmpty(), "so it must not be reported as dropped")
	g.Expect(appliedFields(g, res, kindA)).To(HaveKey("x"))
}

func TestResolveBackendPoliciesErrors(t *testing.T) {
	g := NewWithT(t)
	valid := att(kindA, "valid", t2020, map[string]string{"x": "valid", "y": "valid"})
	broken := att(kindA, "broken", t2021, map[string]string{"y": "broken"}, errors.New("bad"))

	res := ResolveBackendPolicies(attached(map[schema.GroupKind][]ir.PolicyAtt{kindA: {valid, broken}, kindB: {att(kindB, "b", t2022, nil)}}),
		ContributesPolicies{kindA: pluginA(true), kindB: pluginB})

	eff, ok := res.Effective(kindA)
	g.Expect(ok).To(BeTrue())
	g.Expect(eff.Errors).NotTo(BeEmpty(), "the merged policy carries the error, so translation blackholes the cluster")
	g.Expect(res.Dropped(valid.PolicyRef)).To(Equal([]ir.DroppedField{
		{Field: "x", Kind: kindB, By: &ir.AttachedPolicyRef{Group: kindB.Group, Kind: kindB.Kind, Namespace: "ns", Name: "b"}, ByValid: true, Reason: ir.DroppedBySupersede},
	}), "a superseded field is still reported; merge losses are not, since nothing of kind A applies")
	g.Expect(res.Dropped(broken.PolicyRef)).To(BeEmpty(), "an invalid policy reports its errors, not drops")
}

func TestResolveBackendPoliciesKindWithoutMerge(t *testing.T) {
	g := NewWithT(t)
	first := att(kindA, "first", t2020, map[string]string{"x": "first"})
	second := att(kindA, "second", t2021, map[string]string{"x": "second"})

	res := ResolveBackendPolicies(attached(map[schema.GroupKind][]ir.PolicyAtt{kindA: {first, second}}),
		ContributesPolicies{kindA: {}})

	g.Expect(res.Applied(kindA)).To(HaveLen(2), "a kind without MergePolicies applies every attachment")
	_, ok := res.Effective(kindA)
	g.Expect(ok).To(BeFalse(), "and has no single effective policy")
}
