package waypointquery

import (
	"context"

	authcr "istio.io/client-go/pkg/apis/security/v1"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/util/sets"
	"k8s.io/apimachinery/pkg/types"
	gwapi "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

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

func buildAuthzTargetIndex(policies krt.Collection[*authcr.AuthorizationPolicy], rootNamespace string) krt.Index[ir.ObjectSource, *authcr.AuthorizationPolicy] {
	return krtpkg.UnnamedIndex(policies, func(p *authcr.AuthorizationPolicy) []ir.ObjectSource {
		var keys []ir.ObjectSource
		for _, targetRef := range p.Spec.GetTargetRefs() {
			if (targetRef.GetKind() == "Service" && (targetRef.GetGroup() == "" || targetRef.GetGroup() == "core")) ||
				(targetRef.GetKind() == "ServiceEntry" && targetRef.GetGroup() == "networking.istio.io") {
				gk := wellknown.ServiceGVK
				if targetRef.GetKind() == "ServiceEntry" {
					gk = wellknown.ServiceEntryGVK
				}
				keys = append(keys, ir.ObjectSource{
					Name:      targetRef.GetName(),
					Namespace: getEffectiveNamespace(targetRef.GetNamespace(), p.GetNamespace()),
					Group:     gk.Group,
					Kind:      gk.Kind,
				})
			} else if targetRef.GetKind() == "Gateway" && targetRef.GetGroup() == "gateway.networking.k8s.io" {
				keys = append(keys, ir.ObjectSource{
					Name:      targetRef.GetName(),
					Namespace: getEffectiveNamespace(targetRef.GetNamespace(), p.GetNamespace()),
					Group:     targetRef.GetGroup(),
					Kind:      targetRef.GetKind(),
				})
			} else if targetRef.GetKind() == "GatewayClass" && targetRef.GetGroup() == "gateway.networking.k8s.io" && p.GetNamespace() == getRootNamespace(rootNamespace) {
				keys = append(keys, ir.ObjectSource{
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
	gwKey := ir.ObjectSource{
		Name:      gateway.GetName(),
		Namespace: gateway.GetNamespace(),
		Group:     "gateway.networking.k8s.io",
		Kind:      "Gateway",
	}

	// Get all indexed policies targeting this gateway
	allPolicies := krt.Fetch(kctx, w.authzPolicies, krt.FilterIndex(w.byTargetRefKey, gwKey))

	//GatewayClass policies can be in rootNamespace or the gateway namespace
	if rootNamespace != "" && rootNamespace != gateway.GetNamespace() {
		gwClassKey := ir.ObjectSource{
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
	seen := sets.New[types.NamespacedName]()
	var svcPolicies []*authcr.AuthorizationPolicy
	for _, k := range svc.Keys() {
		for _, res := range krt.Fetch(kctx, w.authzPolicies, krt.FilterIndex(w.byTargetRefKey, k)) {
			if !seen.InsertContains(types.NamespacedName{
				Namespace: res.GetNamespace(),
				Name:      res.GetName(),
			}) {
				svcPolicies = append(svcPolicies, res)
			}
		}
	}

	return svcPolicies
}
