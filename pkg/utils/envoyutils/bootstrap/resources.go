package bootstrap

import (
	"errors"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_extensions_transport_sockets_tls_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
)

type EnvoyResources struct {
	Clusters  []*envoy_config_cluster_v3.Cluster
	Listeners []*envoy_config_listener_v3.Listener
	Secrets   []*envoy_extensions_transport_sockets_tls_v3.Secret
	// routes are only used in converting from an xds snapshot.
	routes []*envoy_config_route_v3.RouteConfiguration
	// endpoints are only used in converting from an xds snapshot.
	endpoints []*envoy_config_endpoint_v3.ClusterLoadAssignment
}

func resourcesFromSnapshot(snap envoycache.ResourceSnapshot) (*EnvoyResources, error) {
	listeners, err := listenersFromSnapshot(snap)
	if err != nil {
		return nil, err
	}
	clusters, err := clustersFromSnapshot(snap)
	if err != nil {
		return nil, err
	}
	routes, err := routesFromSnapshot(snap)
	if err != nil {
		return nil, err
	}
	endpoints, err := endpointsFromSnapshot(snap)
	if err != nil {
		return nil, err
	}

	return &EnvoyResources{
		Clusters:  clusters,
		Listeners: listeners,
		routes:    routes,
		endpoints: endpoints,
	}, nil
}

// listenersFromSnapshot accepts a Snapshot and extracts from it a slice of pointers to
// the Listener structs contained in the Snapshot.
func listenersFromSnapshot(snap envoycache.ResourceSnapshot) ([]*envoy_config_listener_v3.Listener, error) {
	var listeners []*envoy_config_listener_v3.Listener
	for _, v := range snap.GetResources(envoyresource.ListenerType) {
		l, ok := v.(*envoy_config_listener_v3.Listener)
		if !ok {
			return nil, errors.New("invalid listener type found")
		}
		listeners = append(listeners, l)
	}
	return listeners, nil
}

// clustersFromSnapshot accepts a Snapshot and extracts from it a slice of pointers to
// the Cluster structs contained in the Snapshot.
func clustersFromSnapshot(snap envoycache.ResourceSnapshot) ([]*envoy_config_cluster_v3.Cluster, error) {
	var clusters []*envoy_config_cluster_v3.Cluster
	for _, v := range snap.GetResources(envoyresource.ClusterType) {
		c, ok := v.(*envoy_config_cluster_v3.Cluster)
		if !ok {
			return nil, errors.New("invalid cluster type found")
		}
		clusters = append(clusters, c)
	}
	return clusters, nil
}

// routesFromSnapshot accepts a Snapshot and extracts from it a slice of pointers to
// the RouteConfiguration structs contained in the Snapshot.
func routesFromSnapshot(snap envoycache.ResourceSnapshot) ([]*envoy_config_route_v3.RouteConfiguration, error) {
	var routes []*envoy_config_route_v3.RouteConfiguration
	for _, v := range snap.GetResources(envoyresource.RouteType) {
		r, ok := v.(*envoy_config_route_v3.RouteConfiguration)
		if !ok {
			return nil, errors.New("invalid route type found")
		}
		routes = append(routes, r)
	}
	return routes, nil
}

// endpointsFromSnapshot accepts a Snapshot and extracts from it a slice of pointers to
// the ClusterLoadAssignment structs contained in the Snapshot.
func endpointsFromSnapshot(snap envoycache.ResourceSnapshot) ([]*envoy_config_endpoint_v3.ClusterLoadAssignment, error) {
	var endpoints []*envoy_config_endpoint_v3.ClusterLoadAssignment
	for _, v := range snap.GetResources(envoyresource.EndpointType) {
		e, ok := v.(*envoy_config_endpoint_v3.ClusterLoadAssignment)
		if !ok {
			return nil, errors.New("invalid endpoint type found")
		}
		endpoints = append(endpoints, e)
	}
	return endpoints, nil
}
