package pluginsdk

import (
	"cmp"
	"maps"
	"slices"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// ResolveBackendPolicies decides, once, what each policy attached to a backend contributes:
//   - Each kind with MergePolicies is merged once; translation applies the merged policy.
//   - A kind that Supersedes fields of another kind takes them whenever any policy of it is
//     attached. The fields are cleared from the other kind's merged policy (ClearFields),
//     and each policy that set them is recorded as dropped in favor of the superseding
//     kind's winner.
//   - Each remaining field a policy sets (SetFields) that the merge credits to another
//     policy of the same kind is recorded as dropped in favor of that policy.
//
// A kind whose merged policy has errors applies nothing: translation blackholes the
// cluster. Merge losses are then not reported, since no policy's field is in effect.
//
// It runs for every backend, and most have no policies: those share one empty resolution
// and allocate nothing.
func ResolveBackendPolicies(attached ir.AttachedPolicies, plugins ContributesPolicies) *ir.BackendPolicyResolution {
	kinds := 0
	for _, pols := range attached.Policies {
		if len(pols) > 0 {
			kinds++
		}
	}
	if kinds == 0 {
		return ir.EmptyBackendPolicyResolution()
	}

	gks := make([]schema.GroupKind, 0, kinds)
	for gk, pols := range attached.Policies {
		if len(pols) > 0 {
			gks = append(gks, gk)
		}
	}
	slices.SortFunc(gks, compareGroupKind)

	// Merge each kind once. The winner of a kind that supersedes others comes from here too.
	merged := make([]mergedKind, len(gks))
	for i, gk := range gks {
		merged[i].gk = gk
		if plugin, ok := plugins[gk]; ok && plugin.MergePolicies != nil {
			merged[i].att = plugin.MergePolicies(attached.Policies[gk])
			merged[i].ok = true
		}
	}

	res := ir.NewBackendPolicyResolution(len(gks))

	// Claim superseded fields before reporting merge losses: a field another kind owns is
	// dropped for every policy that sets it, whichever of them won the merge.
	for i, owner := range gks {
		plugin := plugins[owner]
		if len(plugin.Supersedes) == 0 {
			continue
		}
		winner := kindWinner(attached.Policies[owner], merged[i])
		by := ir.SupersedingPolicy{Kind: owner, By: winner.PolicyRef, ByValid: len(winner.Errors) == 0}
		for _, target := range slices.SortedFunc(maps.Keys(plugin.Supersedes), compareGroupKind) {
			if len(attached.Policies[target]) == 0 || plugins[target].ClearFields == nil {
				// Nothing to supersede, or the target kind cannot drop the field, so translation
				// would still apply it; reporting it as superseded would contradict the cluster.
				continue
			}
			for _, field := range plugin.Supersedes[target] {
				res.SetSuperseded(target, field, by)
			}
		}
	}

	for i, gk := range gks {
		pols := attached.Policies[gk]
		plugin := plugins[gk]
		if !merged[i].ok {
			res.SetAll(gk, pols)
			continue
		}
		m := merged[i].att
		hasSuperseded := res.HasSuperseded(gk)
		applied := m
		if hasSuperseded && len(m.Errors) == 0 {
			if fields := supersededFields(res, gk, plugin, pols); len(fields) > 0 {
				applied.PolicyIr = plugin.ClearFields(m.PolicyIr, fields)
				applied.MergeOrigins = withoutFields(m.MergeOrigins, fields)
			}
		}
		res.SetMerged(gk, applied)

		// A single policy cannot lose a merge, so without a superseded field there is nothing
		// to report and its fields need not be listed.
		if plugin.SetFields == nil || (!hasSuperseded && len(pols) < 2) {
			continue
		}
		for _, att := range pols {
			if att.PolicyRef == nil || len(att.Errors) > 0 {
				continue
			}
			for _, field := range plugin.SetFields(att.PolicyIr) {
				if owner, ok := res.SupersededBy(gk, field); ok {
					res.AddDropped(att.PolicyRef, ir.DroppedField{
						Field: field, Kind: owner.Kind, By: owner.By, ByValid: owner.ByValid, Reason: ir.DroppedBySupersede,
					})
					continue
				}
				if len(m.Errors) > 0 {
					continue
				}
				owners := m.MergeOrigins.Get(field)
				if len(owners) == 0 || slices.Contains(owners, att.PolicyRef.ID()) {
					continue
				}
				res.AddDropped(att.PolicyRef, ir.DroppedField{
					Field: field, Kind: gk, By: refByID(pols, owners), ByValid: true, Reason: ir.DroppedByMerge,
				})
			}
		}
	}
	return res
}

// mergedKind is one attached kind's merged policy; ok is false for a kind that does not merge.
type mergedKind struct {
	gk  schema.GroupKind
	att ir.PolicyAtt
	ok  bool
}

// supersededFields returns the fields of kind gk that another kind owns and some attached
// policy sets, in the order SetFields lists them.
func supersededFields(res *ir.BackendPolicyResolution, gk schema.GroupKind, plugin PolicyPlugin, pols []ir.PolicyAtt) []string {
	if plugin.SetFields == nil {
		return nil
	}
	var fields []string
	for _, att := range pols {
		for _, f := range plugin.SetFields(att.PolicyIr) {
			if !slices.Contains(fields, f) && res.IsSuperseded(gk, f) {
				fields = append(fields, f)
			}
		}
	}
	return fields
}

// kindWinner returns the policy that represents a kind on the backend: its merged policy
// (which names a single policy for kinds whose merge picks one winner, such as
// BackendTLSPolicy) or, for a kind that does not merge, its oldest attachment.
func kindWinner(pols []ir.PolicyAtt, merged mergedKind) ir.PolicyAtt {
	if merged.ok {
		return merged.att
	}
	return pols[ir.WinnerPolicyIndexByCreationTimeAndRef(pols)]
}

// refByID returns the attachment ref named by a single-owner merge origin, or nil.
func refByID(pols []ir.PolicyAtt, owners []string) *ir.AttachedPolicyRef {
	if len(owners) != 1 {
		return nil
	}
	for _, p := range pols {
		if p.PolicyRef != nil && p.PolicyRef.ID() == owners[0] {
			return p.PolicyRef
		}
	}
	return nil
}

// withoutFields returns a copy of origins without the given fields.
func withoutFields(origins ir.MergeOrigins, fields []string) ir.MergeOrigins {
	if origins == nil {
		return nil
	}
	out := maps.Clone(origins)
	for _, f := range fields {
		delete(out, f)
	}
	return out
}

func compareGroupKind(a, b schema.GroupKind) int {
	if c := cmp.Compare(a.Group, b.Group); c != 0 {
		return c
	}
	return cmp.Compare(a.Kind, b.Kind)
}
