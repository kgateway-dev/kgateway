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

// GetAuthorizationPoliciesForGateway returns policies targeting a specific gateway
func (w *waypointQueries) GetAuthorizationPoliciesForGateway(
	kctx krt.HandlerContext,
	ctx context.Context,
	gateway *gwapi.Gateway,
	rootNamespace string) []*authcr.AuthorizationPolicy {

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

	for i, p := range allPolicies {
		fmt.Printf("  Policy %d: %s/%s (deletion: %v)\n", i,
			p.GetNamespace(), p.GetName(), p.GetDeletionTimestamp() != nil)
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
