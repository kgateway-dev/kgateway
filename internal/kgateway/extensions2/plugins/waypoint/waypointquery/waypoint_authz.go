package waypointquery

import (
	"context"
	"fmt"

	authcr "istio.io/client-go/pkg/apis/security/v1"
	"istio.io/istio/pilot/pkg/serviceregistry/provider"
	"istio.io/istio/pkg/kube/krt"
	gwapi "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

// targetRefKey identifies a service or gateway that policies target
type targetRefKey struct {
	Name      string
	Namespace string
	Group     string
	Kind      string
}

// the key needs to be a string to be used as a map key
func (k targetRefKey) String() string {
	return fmt.Sprintf("%s/%s/%s/%s", k.Group, k.Kind, k.Namespace, k.Name)
}

// getRootNamespace sets the RootNamespace from settings.
// This should be called during initialization.
// Instead of a package-level variable, create a function that gets the namespace
// This can be placed in the same package where you currently have the RootNamespace variable
func getRootNamespace(settingRootNamespace string) string {
	defaultRootNamespace := "istio-system"
	if settingRootNamespace != "" {
		return settingRootNamespace
	}
	return defaultRootNamespace
}

// This is based on the current Istio AuthorizationPolicy docs (TargetRef section)
// https://istio.io/latest/docs/reference/config/security/authorization-policy/#TargetRef

// Currently, the following resource attachment types are supported:
// kind: Gateway with group: gateway.networking.k8s.io in the same namespace.
// kind: GatewayClass with group: gateway.networking.k8s.io in the root namespace.
// kind: Service with group: "" or group: "core" in the same namespace. This type is only supported for waypoints.
// kind: ServiceEntry with group: networking.istio.io in the same namespace.

func buildAuthzTargetIndex(policies krt.Collection[*authcr.AuthorizationPolicy], rootNamespace string) krt.Index[targetRefKey, *authcr.AuthorizationPolicy] {
	return krt.NewIndex(policies, func(p *authcr.AuthorizationPolicy) []targetRefKey {
		var keys []targetRefKey
		for _, targetRef := range p.Spec.GetTargetRefs() {
			if (targetRef.GetKind() == "Service" && (targetRef.GetGroup() == "" || targetRef.GetGroup() == "core")) ||
				(targetRef.GetKind() == "ServiceEntry" && targetRef.GetGroup() == "networking.istio.io") {
				gk := wellknown.ServiceGVK
				if targetRef.GetKind() == "ServiceEntry" {
					gk = wellknown.ServiceEntryGVK
				}
				keys = append(keys, targetRefKey{
					Name:      targetRef.GetName(),
					Namespace: getEffectiveNamespace(targetRef.GetNamespace(), p.GetNamespace()),
					Group:     gk.Group,
					Kind:      gk.Kind,
				})
			} else if targetRef.GetKind() == "Gateway" && targetRef.GetGroup() == "gateway.networking.k8s.io" {
				keys = append(keys, targetRefKey{
					Name:      targetRef.GetName(),
					Namespace: getEffectiveNamespace(targetRef.GetNamespace(), p.GetNamespace()),
					Group:     targetRef.GetGroup(),
					Kind:      targetRef.GetKind(),
				})
			} else if targetRef.GetKind() == "GatewayClass" && targetRef.GetGroup() == "gateway.networking.k8s.io" && p.GetNamespace() == getRootNamespace(rootNamespace) {
				keys = append(keys, targetRefKey{
					Name:      targetRef.GetName(),
					Namespace: getEffectiveNamespace(targetRef.GetNamespace(), p.GetNamespace()),
					Group:     targetRef.GetGroup(),
					Kind:      targetRef.GetKind(),
				})
			}
		}
		return keys
	})
}

// GetAuthorizationPoliciesForGateway returns policies targeting a specific gateway
func (w *waypointQueries) GetAuthorizationPoliciesForGateway(
	kctx krt.HandlerContext,
	ctx context.Context,
	gateway *gwapi.Gateway,
	settingRootNamespace string) []*authcr.AuthorizationPolicy {
	rootNamespace := getRootNamespace(settingRootNamespace)
	// Get policies targeting this gateway directly using the index
	gwKey := targetRefKey{
		Name:      gateway.GetName(),
		Namespace: gateway.GetNamespace(),
		Group:     "gateway.networking.k8s.io",
		Kind:      "Gateway",
	}

	// Get all indexed policies targeting this gateway
	allPolicies := krt.Fetch(kctx, w.authzPolicies, krt.FilterIndex(w.byTargetRefKey, gwKey))

	//GatewayClass policies can be in rootNamespace or the gateway namespace
	if rootNamespace != "" && rootNamespace != gateway.GetNamespace() {
		gwClassKey := targetRefKey{
			Name:      "kgateway-waypoint",
			Namespace: rootNamespace,
			Group:     "gateway.networking.k8s.io",
			Kind:      "GatewayClass",
		}
		rootPolicies := krt.Fetch(kctx, w.authzPolicies, krt.FilterIndex(w.byTargetRefKey, gwClassKey))
		allPolicies = append(allPolicies, rootPolicies...)
	}
	return allPolicies
}

// GetAuthorizationPoliciesForService returns policies targeting a specific service
func (w *waypointQueries) GetAuthorizationPoliciesForService(
	kctx krt.HandlerContext,
	ctx context.Context,
	svc *Service) []*authcr.AuthorizationPolicy {
	providerID := svc.Provider()

	gk := wellknown.ServiceGVK.GroupKind()
	if providerID == provider.External {
		gk = wellknown.ServiceEntryGVK.GroupKind()
	}

	svcKey := targetRefKey{
		Name:      svc.GetName(),
		Namespace: svc.GetNamespace(),
		Group:     gk.Group,
		Kind:      gk.Kind,
	}

	svcPolicies := krt.Fetch(kctx, w.authzPolicies, krt.FilterIndex(w.byTargetRefKey, svcKey))
	return svcPolicies
}
