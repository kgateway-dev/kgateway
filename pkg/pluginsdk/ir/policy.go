package ir

import "strings"

// ComparePoliciesByCreationTimeAndRef returns the Gateway API conflict ordering for
// two attached policies: older policies win, with a deterministic ref string tie-breaker.
func ComparePoliciesByCreationTimeAndRef(a, b PolicyAtt) int {
	if cmp := a.PolicyIr.CreationTime().Compare(b.PolicyIr.CreationTime()); cmp != 0 {
		return cmp
	}
	return strings.Compare(PolicyRefString(a.PolicyRef), PolicyRefString(b.PolicyRef))
}

// WinnerPolicyIndexByCreationTimeAndRef returns the index of the winning policy
// according to ComparePoliciesByCreationTimeAndRef.
func WinnerPolicyIndexByCreationTimeAndRef(policies []PolicyAtt) int {
	winnerIdx := 0
	for i := 1; i < len(policies); i++ {
		if ComparePoliciesByCreationTimeAndRef(policies[i], policies[winnerIdx]) < 0 {
			winnerIdx = i
		}
	}
	return winnerIdx
}

// PolicyRefString returns a deterministic string form of an attached policy ref
// suitable for tie-breaking and hashing.
func PolicyRefString(ref *AttachedPolicyRef) string {
	if ref == nil {
		return ""
	}
	return ref.Group + "/" + ref.Kind + "/" + ref.Namespace + "/" + ref.Name + "/" + ref.SectionName
}
