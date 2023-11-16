package snapshot

import (
	"context"
	"fmt"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/solo-io/go-utils/contextutils"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
)

type ResourceHasher func(resources []envoycache.Resource) (uint64, error)

func GenerateXDSSnapshot(
	ctx context.Context,
	hasher ResourceHasher,
	clusters []*envoy_config_cluster_v3.Cluster,
	endpoints []*envoy_config_endpoint_v3.ClusterLoadAssignment,
	routeConfigs []*envoy_config_route_v3.RouteConfiguration,
	listeners []*envoy_config_listener_v3.Listener,
) envoycache.Snapshot {
	var endpointsProto, clustersProto, listenersProto []envoycache.Resource

	for _, ep := range endpoints {
		endpointsProto = append(endpointsProto, resource.NewEnvoyResource(ep))
	}
	for _, cluster := range clusters {
		clustersProto = append(clustersProto, resource.NewEnvoyResource(cluster))
	}
	for _, listener := range listeners {
		// don't add empty listeners, envoy will complain
		if len(listener.GetFilterChains()) < 1 {
			continue
		}
		listenersProto = append(listenersProto, resource.NewEnvoyResource(listener))
	}
	// construct version
	// TODO: investigate whether we need a more sophisticated versioning algorithm
	endpointsVersion, endpointsErr := hasher(endpointsProto)
	if endpointsErr != nil {
		contextutils.LoggerFrom(ctx).DPanic(fmt.Sprintf("error trying to hash endpointsProto: %v", endpointsErr))
	}
	clustersVersion, clustersErr := hasher(clustersProto)
	if clustersErr != nil {
		contextutils.LoggerFrom(ctx).DPanic(fmt.Sprintf("error trying to hash clustersProto: %v", clustersErr))
	}
	listenersVersion, listenersErr := hasher(listenersProto)
	if listenersErr != nil {
		contextutils.LoggerFrom(ctx).DPanic(fmt.Sprintf("error trying to hash listenersProto: %v", listenersErr))
	}

	// if clusters are updated, provider a new version of the endpoints,
	// so the clusters are warm
	endpointsNew := envoycache.NewResources(fmt.Sprintf("%v-%v", clustersVersion, endpointsVersion), endpointsProto)
	if endpointsErr != nil || clustersErr != nil {
		endpointsNew = envoycache.NewResources("endpoints-hashErr", endpointsProto)
	}
	clustersNew := envoycache.NewResources(fmt.Sprintf("%v", clustersVersion), clustersProto)
	if clustersErr != nil {
		clustersNew = envoycache.NewResources("clusters-hashErr", endpointsProto)
	}
	listenersNew := envoycache.NewResources(fmt.Sprintf("%v", listenersVersion), listenersProto)
	if listenersErr != nil {
		listenersNew = envoycache.NewResources("listeners-hashErr", listenersProto)
	}
	return NewSnapshotFromResources(
		endpointsNew,
		clustersNew,
		MakeRdsResources(hasher, routeConfigs),
		listenersNew)
}

func MakeRdsResources(hasher ResourceHasher, routeConfigs []*envoy_config_route_v3.RouteConfiguration) envoycache.Resources {
	var routesProto []envoycache.Resource

	for _, routeCfg := range routeConfigs {
		// don't add empty route configs, envoy will complain
		if len(routeCfg.GetVirtualHosts()) < 1 {
			continue
		}
		routesProto = append(routesProto, resource.NewEnvoyResource(routeCfg))

	}

	routesVersion, err := hasher(routesProto)
	if err != nil {
		contextutils.LoggerFrom(context.Background()).DPanic(fmt.Sprintf("error trying to hash routesProto: %v", err))
		return envoycache.NewResources("routes-hashErr", routesProto)
	}
	return envoycache.NewResources(fmt.Sprintf("%v", routesVersion), routesProto)
}
