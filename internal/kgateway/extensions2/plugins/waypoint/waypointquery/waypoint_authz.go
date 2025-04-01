package waypointquery

import (
	"context"

	istiosecurity "istio.io/client-go/pkg/apis/security/v1"
	"istio.io/istio/pkg/kube/krt"
)

func (w *waypointQueries) GetAuthorizationPolicies(kctx krt.HandlerContext, ctx context.Context, targetNamespace, rootNamespace string) []*istiosecurity.AuthorizationPolicy {
	// Get all policies in the target namespace
	policies := krt.Fetch(kctx, w.authzPolicies, krt.FilterIndex(w.byNamespace, targetNamespace))

	// Get all policies in the root namespace
	if rootNamespace != "" && rootNamespace != targetNamespace {
		rootPolicies := krt.Fetch(kctx, w.authzPolicies, krt.FilterIndex(w.byNamespace, rootNamespace))
		policies = append(policies, rootPolicies...)
	}

	// Filter policies to only include those targeting services in the target namespace
	filteredPolicies := make([]*istiosecurity.AuthorizationPolicy, 0, len(policies))
	for _, policy := range policies {
		for _, targetRef := range policy.Spec.GetTargetRefs() {
			if targetRef.GetKind() == "Service" && targetRef.GetGroup() == "" {
				// If the policy targets a service in the target namespace, include it
				targetNamespaceMatches := targetRef.GetNamespace() == "" || targetRef.GetNamespace() == targetNamespace
				if targetNamespaceMatches {
					filteredPolicies = append(filteredPolicies, policy)
					break
				}
			}
		}
	}
	return filteredPolicies
}
