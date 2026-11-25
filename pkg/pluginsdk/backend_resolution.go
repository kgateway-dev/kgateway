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
func ResolveBackendPolicies(attached ir.AttachedPolicies, plugins ContributesPolicies) *ir.BackendPolicyResolution {
	res := ir.NewBackendPolicyResolution()
	gks := slices.SortedFunc(maps.Keys(attached.Policies), compareGroupKind)

	// Merge each kind once. The winner of a kind that supersedes others comes from here too.
	merged := make(map[schema.GroupKind]ir.PolicyAtt, len(gks))
	for _, gk := range gks {
		pols := attached.Policies[gk]
		if len(pols) == 0 {
			continue
		}
		if plugin, ok := plugins[gk]; ok && plugin.MergePolicies != nil {
			merged[gk] = plugin.MergePolicies(pols)
		}
	}

	// Claim superseded fields before reporting merge losses: a field another kind owns is
	// dropped for every policy that sets it, whichever of them won the merge.
	for _, owner := range gks {
		plugin, ok := plugins[owner]
		if !ok || len(plugin.Supersedes) == 0 || len(attached.Policies[owner]) == 0 {
			continue
		}
		winner := kindWinner(attached.Policies[owner], merged, owner)
		by := ir.SupersedingPolicy{Kind: owner, By: winner.PolicyRef, ByValid: len(winner.Errors) == 0}
		for _, target := range slices.SortedFunc(maps.Keys(plugin.Supersedes), compareGroupKind) {
			if len(attached.Policies[target]) == 0 {
				continue
			}
			if targetPlugin, ok := plugins[target]; !ok || targetPlugin.ClearFields == nil {
				// The target kind cannot drop the field, so translation would still apply it.
				// Reporting it as superseded would contradict the cluster.
				continue
			}
			for _, field := range plugin.Supersedes[target] {
				if !res.IsSuperseded(target, field) {
					res.SetSuperseded(target, field, by)
				}
			}
		}
	}

	for _, gk := range gks {
		pols := attached.Policies[gk]
		if len(pols) == 0 {
			continue
		}
		plugin := plugins[gk]
		m, merges := merged[gk]
		if !merges {
			res.SetApplied(gk, slices.Clone(pols))
			continue
		}
		superseded := supersededFields(res, gk, plugin, pols)
		applied := m
		if len(superseded) > 0 && len(m.Errors) == 0 {
			applied.PolicyIr = plugin.ClearFields(m.PolicyIr, superseded)
			applied.MergeOrigins = withoutFields(m.MergeOrigins, superseded)
		}
		res.SetApplied(gk, []ir.PolicyAtt{applied})

		if plugin.SetFields == nil {
			continue
		}
		for _, att := range pols {
			if att.PolicyRef == nil || len(att.Errors) > 0 {
				continue
			}
			fields := slices.Clone(plugin.SetFields(att.PolicyIr))
			slices.Sort(fields)
			for _, field := range fields {
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

// supersededFields returns the fields of kind gk that another kind owns, in order.
func supersededFields(res *ir.BackendPolicyResolution, gk schema.GroupKind, plugin PolicyPlugin, pols []ir.PolicyAtt) []string {
	var fields []string
	seen := map[string]bool{}
	for _, att := range pols {
		if plugin.SetFields == nil {
			break
		}
		for _, f := range plugin.SetFields(att.PolicyIr) {
			if !seen[f] && res.IsSuperseded(gk, f) {
				seen[f] = true
				fields = append(fields, f)
			}
		}
	}
	slices.Sort(fields)
	return fields
}

// kindWinner returns the policy that represents a kind on the backend: its merged policy
// (which names a single policy for kinds whose merge picks one winner, such as
// BackendTLSPolicy) or, for a kind that does not merge, its oldest attachment.
func kindWinner(pols []ir.PolicyAtt, merged map[schema.GroupKind]ir.PolicyAtt, gk schema.GroupKind) ir.PolicyAtt {
	if m, ok := merged[gk]; ok {
		return m
	}
	if len(pols) == 0 {
		return ir.PolicyAtt{}
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
