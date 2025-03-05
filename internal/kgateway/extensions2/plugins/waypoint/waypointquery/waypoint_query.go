package waypointquery

import (
	"context"

	istiosecurity "istio.io/client-go/pkg/apis/security/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/query"
)

const (
	// IstioUseWaypointLabel is the label used to specify which waypoint should be used for a given pod, service, etc...
	// `istio.io/use-waypoint: none` means skipping using any waypoint specified from higher scope, namespace/service, etc...
	IstioUseWaypointLabel = "istio.io/use-waypoint"
	// IstioUseWaypointNamespaceLabel is a label used to indicate the namespace of the waypoint (referred to by AmbientUseWaypointLabel).
	// This allows cross-namespace waypoint references. If unset, the same namespace is assumed.
	IstioUseWaypointNamespaceLabel = "istio.io/use-waypoint-namespace"
)

type WaypointQueries interface {
	// GetWaypointServices returns all Services that are marked as using the Gateway
	// via istio.io/use-waypoint (and possibly istio.io/use-waypoint-namespace).
	GetWaypointServices(ctx context.Context, gw *gwv1.Gateway) ([]Service, error)

	// GetHTTPRoutesForService fetches HTTPRoutes that have the given Service in parentRefs.
	GetHTTPRoutesForService(ctx context.Context, svc *Service) ([]query.RouteInfo, error)

	// GetAuthorizationPolicies gets all AuthorizationPolicy resources in the targetNamespace and rootNamespace.
	// Callers should apply attachment logic themselves for particular Gateways and Services.
	GetAuthorizationPolicies(ctx context.Context, targetNamespace, rootNamespace string) ([]*istiosecurity.AuthorizationPolicy, error)
}

func NewQueries(client client.Client, gwQueries query.GatewayQueries) WaypointQueries {
	return nil // TODO TODO TODO
}
