package waypointquery

import (
	"context"

	istiosecurity "istio.io/client-go/pkg/apis/security/v1"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/log"
)

func (w *waypointQueries) GetAuthorizationPolicies(kctx krt.HandlerContext, ctx context.Context, targetNamespace, rootNamespace string) []*istiosecurity.AuthorizationPolicy {
	log.Infof("Fetching authorization policies for target namespace %q and root namespace %q", targetNamespace, rootNamespace)

	// Get all policies in the target namespace
	policies := krt.Fetch(kctx, w.authzPolicies, krt.FilterIndex(w.byNamespace, targetNamespace))
	log.Infof("Found %d authorization policies in target namespace %q", len(policies), targetNamespace)
	for _, p := range policies {
		log.Infof("Policy in target namespace: %s/%s, spec: %+v", p.Namespace, p.Name, p.Spec)
	}

	// Get all policies in the root namespace
	if rootNamespace != "" && rootNamespace != targetNamespace {
		rootPolicies := krt.Fetch(kctx, w.authzPolicies, krt.FilterIndex(w.byNamespace, rootNamespace))
		log.Infof("Found %d authorization policies in root namespace %q", len(rootPolicies), rootNamespace)
		for _, p := range rootPolicies {
			log.Infof("Policy in root namespace: %s/%s, spec: %+v", p.Namespace, p.Name, p.Spec)
		}
		policies = append(policies, rootPolicies...)
	}

	// Filter policies to only include those targeting services in the target namespace
	filteredPolicies := make([]*istiosecurity.AuthorizationPolicy, 0, len(policies))
	for _, policy := range policies {
		log.Infof("Policy %s/%s annotations: %+v", policy.Namespace, policy.Name, policy.Annotations)
		for _, targetRef := range policy.Spec.TargetRefs {
			log.Infof("  Target ref: kind=%s, group=%s, name=%s, namespace=%s", targetRef.Kind, targetRef.Group, targetRef.Name, targetRef.Namespace)
			if targetRef.Kind == "Service" && targetRef.Group == "" {
				// If the policy targets a service in the target namespace, include it
				targetNamespaceMatches := targetRef.Namespace == "" || targetRef.Namespace == targetNamespace
				if targetNamespaceMatches {
					log.Infof("  Policy %s/%s matches service in target namespace %s", policy.Namespace, policy.Name, targetNamespace)
					filteredPolicies = append(filteredPolicies, policy)
					break
				}
			}
		}
	}

	log.Infof("Returning %d filtered authorization policies", len(filteredPolicies))
	for _, p := range filteredPolicies {
		log.Infof("Filtered policy: %s/%s, spec: %+v", p.Namespace, p.Name, p.Spec)
	}
	return filteredPolicies
}
