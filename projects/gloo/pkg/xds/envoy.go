package xds

import (
	"fmt"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"

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

func (h *ProxyKeyHasher) ID(node *core.Node) string {
	if node.Metadata != nil {
		roleValue := node.Metadata.Fields["role"]
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
func GetKeysFromProxies(proxies v1.ProxyList) []string {
	var validKeys []string
	// This is where we correlate Node ID with proxy namespace~name
	for _, proxy := range proxies {
		validKeys = append(validKeys, SnapshotKey(proxy))
	}
	return validKeys
}

// register xDS methods with GRPC server
func SetupEnvoyXds(grpcServer *grpc.Server, xdsServer envoyserver.Server, envoyCache envoycache.SnapshotCache) {

	// check if we need to register
	if _, ok := grpcServer.GetServiceInfo()["envoy.api.v2.EndpointDiscoveryService"]; ok {
		return
	}
	envoyServer := NewEnvoyServer(xdsServer)

	v2.RegisterEndpointDiscoveryServiceServer(grpcServer, envoyServer)
	v2.RegisterClusterDiscoveryServiceServer(grpcServer, envoyServer)
	v2.RegisterRouteDiscoveryServiceServer(grpcServer, envoyServer)
	v2.RegisterListenerDiscoveryServiceServer(grpcServer, envoyServer)
	_ = envoyCache.SetSnapshot(FallbackNodeKey, fallbackSnapshot(fallbackBindAddr, fallbackBindPort, fallbackStatusCode))

}
