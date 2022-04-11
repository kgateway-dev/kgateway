package xds

import (
	"fmt"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	envoy_service_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	envoy_service_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	envoy_service_route_v3 "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	envoyserver "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/server"
	solo_xds "github.com/solo-io/solo-kit/pkg/api/xds"
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
	if _, ok := grpcServer.GetServiceInfo()["solo.io.xds.SoloDiscoveryService"]; ok {
		return
	}

	// The Gloo Server is an XDS server that accepts v2 Envoy ADS requests. The Envoy v2 API has been
	// deprecated but the ADS api has been preserved internally to support discovery of
	// ext-auth and rate-limit configurations.
	glooServer := NewGlooXdsServer(xdsServer)
	solo_xds.RegisterSoloDiscoveryServiceServer(grpcServer, glooServer)

	envoyServer := NewEnvoyServerV3(xdsServer)
	envoy_service_endpoint_v3.RegisterEndpointDiscoveryServiceServer(grpcServer, envoyServer)
	envoy_service_cluster_v3.RegisterClusterDiscoveryServiceServer(grpcServer, envoyServer)
	envoy_service_route_v3.RegisterRouteDiscoveryServiceServer(grpcServer, envoyServer)
	envoy_service_listener_v3.RegisterListenerDiscoveryServiceServer(grpcServer, envoyServer)
	envoy_service_discovery_v3.RegisterAggregatedDiscoveryServiceServer(grpcServer, envoyServer)

	envoyCache.SetSnapshot(FallbackNodeKey, fallbackSnapshot(fallbackBindAddr, fallbackBindPort, fallbackStatusCode))
}
