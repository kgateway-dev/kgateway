package xds

import (
	"fmt"

	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	envoy_service_discovery_v2 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	envoy_service_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	envoy_service_route_v3 "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	envoyserver "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
	"google.golang.org/grpc"
)

// Returns the node.metadata.role from the envoy bootstrap config
// if not found, it returns a key for the Fallback snapshot
// which alerts the user their Envoy is missing the required role key.
type ProxyKeyHasher struct{}

func NewNodeHasher() *ProxyKeyHasher {
	return &ProxyKeyHasher{}
}

func (h *ProxyKeyHasher) ID(node *envoy_config_core_v3.Node) string {
	if node.GetMetadata() != nil {
		roleValue := node.GetMetadata().GetFields()["role"]
		if roleValue != nil {
			return roleValue.GetStringValue()
		}
	}
	// TODO: use FallbackNodeKey here
	return ""
}

// used to let nodes know they have a bad config
// we assign a "fix me" snapshot for bad nodes
const FallbackNodeKey = "misconfigured-node"

// TODO(ilackarms): expose these as a configuration option (maybe)
var fallbackBindPort = defaults.HttpPort

const (
	fallbackBindAddr   = "::"
	fallbackStatusCode = 500
)

// SnapshotKey of Proxy == Role in Envoy Configmap == "Node" in Envoy semantics
func SnapshotKey(proxy *v1.Proxy) string {
	namespace, name := proxy.GetMetadata().Ref().Strings()
	return fmt.Sprintf("%v~%v", namespace, name)
}

// Called in Syncer when a new set of proxies arrive
// used to trim snapshots whose proxies have been deleted
func GetValidKeys(proxies v1.ProxyList, extensionKeys map[string]struct{}) []string {
	var validKeys []string
	// Get keys from proxies
	for _, proxy := range proxies {
		// This is where we correlate Node ID with proxy namespace~name
		validKeys = append(validKeys, SnapshotKey(proxy))
	}
	for key := range extensionKeys {
		validKeys = append(validKeys, key)
	}
	return validKeys
}

// register xDS methods with GRPC server
func SetupEnvoyXds(grpcServer *grpc.Server, xdsServer envoyserver.Server, envoyCache envoycache.SnapshotCache) {

	// check if we need to register
	if _, ok := grpcServer.GetServiceInfo()["envoy.api.v2.EndpointDiscoveryService"]; ok {
		return
	}

	// The v2 implementation is kept solely to support discovery of ext-auth and rate-limit configuration
	// Relevant GitHub issue to remove this: https://github.com/solo-io/gloo/issues/4369
	// Context: Envoy has deprecated the v2 API and no longer providing support for it. We use the v2 xDS
	//	protocol as a transport mechanism to serve ext-auth and rate-limit with their configuration. Since
	//	Envoy is not directly involved in this connection, we can continue to rely on the v2 go-control-plane
	//  code, which, although deprecated, still works.
	//  A preferred path forward would be for us to maintain the necessary code to support this version of
	//	xDS, and to not rely on the go-control-plane, since that code might get removed in the future.
	serverV2 := NewEnvoyServerV2(xdsServer)
	envoy_api_v2.RegisterEndpointDiscoveryServiceServer(grpcServer, serverV2)
	envoy_api_v2.RegisterClusterDiscoveryServiceServer(grpcServer, serverV2)
	envoy_api_v2.RegisterRouteDiscoveryServiceServer(grpcServer, serverV2)
	envoy_api_v2.RegisterListenerDiscoveryServiceServer(grpcServer, serverV2)
	envoy_service_discovery_v2.RegisterAggregatedDiscoveryServiceServer(grpcServer, serverV2)

	serverV3 := NewEnvoyServerV3(xdsServer)
	envoy_service_endpoint_v3.RegisterEndpointDiscoveryServiceServer(grpcServer, serverV3)
	envoy_service_cluster_v3.RegisterClusterDiscoveryServiceServer(grpcServer, serverV3)
	envoy_service_route_v3.RegisterRouteDiscoveryServiceServer(grpcServer, serverV3)
	envoy_service_listener_v3.RegisterListenerDiscoveryServiceServer(grpcServer, serverV3)
	envoy_service_discovery_v3.RegisterAggregatedDiscoveryServiceServer(grpcServer, serverV3)

	_ = envoyCache.SetSnapshot(FallbackNodeKey, fallbackSnapshot(fallbackBindAddr, fallbackBindPort, fallbackStatusCode))

}
