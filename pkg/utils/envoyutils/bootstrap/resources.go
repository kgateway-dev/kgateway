package bootstrap

import (
	"errors"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyresource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
)

type EnvoyResources struct {
	Clusters  []*envoyclusterv3.Cluster
	Listeners []*envoylistenerv3.Listener
	Secrets   []*envoytlsv3.Secret
	// routes are only used in converting from an xds snapshot.
	routes []*envoyroutev3.RouteConfiguration
	// endpoints are only used in converting from an xds snapshot.
	endpoints []*envoyendpointv3.ClusterLoadAssignment
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
func listenersFromSnapshot(snap envoycache.ResourceSnapshot) ([]*envoylistenerv3.Listener, error) {
	var listeners []*envoylistenerv3.Listener
	for _, v := range snap.GetResources(envoyresource.ListenerType) {
		l, ok := v.(*envoylistenerv3.Listener)
		if !ok {
			return nil, errors.New("invalid listener type found")
		}
		listeners = append(listeners, l)
	}
	return listeners, nil
}

// clustersFromSnapshot accepts a Snapshot and extracts from it a slice of pointers to
// the Cluster structs contained in the Snapshot.
func clustersFromSnapshot(snap envoycache.ResourceSnapshot) ([]*envoyclusterv3.Cluster, error) {
	var clusters []*envoyclusterv3.Cluster
	for _, v := range snap.GetResources(envoyresource.ClusterType) {
		c, ok := v.(*envoyclusterv3.Cluster)
		if !ok {
			return nil, errors.New("invalid cluster type found")
		}
		clusters = append(clusters, c)
	}
	return clusters, nil
}

// routesFromSnapshot accepts a Snapshot and extracts from it a slice of pointers to
// the RouteConfiguration structs contained in the Snapshot.
func routesFromSnapshot(snap envoycache.ResourceSnapshot) ([]*envoyroutev3.RouteConfiguration, error) {
	var routes []*envoyroutev3.RouteConfiguration
	for _, v := range snap.GetResources(envoyresource.RouteType) {
		r, ok := v.(*envoyroutev3.RouteConfiguration)
		if !ok {
			return nil, errors.New("invalid route type found")
		}
		routes = append(routes, r)
	}
	return routes, nil
}

// endpointsFromSnapshot accepts a Snapshot and extracts from it a slice of pointers to
// the ClusterLoadAssignment structs contained in the Snapshot.
func endpointsFromSnapshot(snap envoycache.ResourceSnapshot) ([]*envoyendpointv3.ClusterLoadAssignment, error) {
	var endpoints []*envoyendpointv3.ClusterLoadAssignment
	for _, v := range snap.GetResources(envoyresource.EndpointType) {
		e, ok := v.(*envoyendpointv3.ClusterLoadAssignment)
		if !ok {
			return nil, errors.New("invalid endpoint type found")
		}
		endpoints = append(endpoints, e)
	}
	return endpoints, nil
}
