package reports

import gwv1 "sigs.k8s.io/gateway-api/apis/v1"

// MaxPolicyStatusAncestors is the Gateway API limit for PolicyStatus.ancestors.
const MaxPolicyStatusAncestors = 16

// ParentRefEqual reports whether two Gateway API parent references are identical.
func ParentRefEqual(a, b gwv1.ParentReference) bool {
	return a.Name == b.Name &&
		optionalRefEqual(a.Group, b.Group) &&
		optionalRefEqual(a.Kind, b.Kind) &&
		optionalRefEqual(a.Namespace, b.Namespace) &&
		optionalRefEqual(a.SectionName, b.SectionName) &&
		optionalRefEqual(a.Port, b.Port)
}

func optionalRefEqual[T comparable](a, b *T) bool {
	switch {
	case a == nil || b == nil:
		return a == nil && b == nil
	default:
		return *a == *b
	}
}
