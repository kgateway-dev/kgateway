package httproute

import (
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
	delegationutils "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils/delegation"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// filterDelegatedChildren takes a parent route matcher and a list of children
// referenced by the parent's backendRefs, and filters the children based on
// the following criteria, returning only the valid child delegatee routes:
//   - If a child sets parentRefs, the parentRefs must include the parent (if
//     parentRefs is not set, then any parent may delegate to that child)
//   - If route matcher inheritance is used (via annotation on the child), the
//     child matcher does not need to match the parent matcher. The parent and
//     child matchers are merged according to the rules specified by
//     `delegationutils.MergeParentChildRouteMatch`.
//   - If route matcher inheritance is not used (the default), then the parent
//     and child matchers must match according to the requirements specified by
//     `delegationutils.IsDelegatedRouteMatch`. If they don't match, the child
//     matcher will be discarded from the results.
//
// The per-rule filtering and merging is done by delegationutils.FilterDelegatedRuleMatches.
// After that processing, if a child route rule does not have any valid
// matches with respect to the parent, the rule is discarded. If the child route
// does not have any remaining valid route rules, the whole route is discarded.
func filterDelegatedChildren(
	parentRef types.NamespacedName,
	parentMatch gwv1.HTTPRouteMatch,
	children []*query.RouteInfo,
) []*query.RouteInfo {
	// Select the child routes that match the parent
	var selected []*query.RouteInfo
	for _, c := range children {
		// Check if the child route is allowed to be delegated to by the parent
		if !delegationutils.ChildRouteCanAttachToParentRef(c.Object.GetNamespace(), c.Object.GetParentRefs(), parentRef) {
			continue
		}

		// make a copy; multiple parents can delegate to the same child so we can't modify a shared reference
		clone := c.Clone()
		origChild, ok := clone.Object.(*ir.HttpRouteIR)
		if !ok {
			continue
		}
		cloneChild := *origChild
		child := &cloneChild
		// make sure we don't overwite the original rules
		child.Rules = make([]ir.HttpRouteRuleIR, len(origChild.Rules))
		copy(child.Rules, origChild.Rules)

		inheritMatcher := child.DelegationInheritParentMatcher

		// We use validRules to store the rules in the child route that are valid
		// (matches in the rule match the parent route matcher). If a specific rule
		// in the child is not valid, then we discard it in the final child route
		// returned by this function.
		var validRules []ir.HttpRouteRuleIR
		for i, rule := range child.Rules {
			validMatches := delegationutils.FilterDelegatedRuleMatches(parentMatch, rule.Matches, inheritMatcher)

			// if there were any valid matches, store this rule as a valid rule
			if len(validMatches) > 0 {
				validRule := child.Rules[i]
				validRule.Matches = validMatches
				validRules = append(validRules, validRule)
			}
		}
		// if there were any valid rules, then add this child route as a valid delegatee
		if len(validRules) > 0 {
			child.Rules = validRules
			clone.Object = child
			selected = append(selected, clone)
		}
	}

	return selected
}
