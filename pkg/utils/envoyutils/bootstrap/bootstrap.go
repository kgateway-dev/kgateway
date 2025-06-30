package bootstrap

import (
	"context"
	"errors"
	"fmt"

	envoy_config_bootstrap_v3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_extensions_filters_network_http_connection_manager_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
)

var (
	// errNoHcm represents a situation where a filter chain does not have an HttpConnectionManager.
	// Because this could occur for valid reasons, such as a TCP proxy, we use
	// this as a sentinel error to inform us it's ok to ignore it and continue.
	errNoHcm = errors.New("no HttpConnectionManager found")
)

func FromEnvoyResources(resources *EnvoyResources) (string, error) {
	bootstrap := &envoy_config_bootstrap_v3.Bootstrap{
		Node: &envoy_config_core_v3.Node{
			Id:      "validation-node-id",
			Cluster: "validation-cluster",
		},
		StaticResources: &envoy_config_bootstrap_v3.Bootstrap_StaticResources{
			Listeners: resources.Listeners,
			Clusters:  resources.Clusters,
			Secrets:   resources.Secrets,
		},
	}

	marshaler := &protojson.MarshalOptions{
		UseProtoNames: true,
	}
	j, err := marshaler.Marshal(bootstrap)
	return string(j), err // returns a json, but json is valid yaml
}

// FromFilter accepts a filter name and typed config for that filter,
// contructs a static bootstrap config containing a single vhost with typed
// per-filter config matching the arguments, marshals it to json, and returns
// the stringified json or any error if it occurred.
func FromFilter(filterName string, msg proto.Message) (string, error) {
	typedFilter, err := utils.MessageToAny(msg)
	if err != nil {
		return "", err
	}

	// Construct a vhost that contains our filter config as TypedPerFilterConfig.
	vhosts := []*envoy_config_route_v3.VirtualHost{
		{
			Name:    "placeholder_host",
			Domains: []string{"*"},
			TypedPerFilterConfig: map[string]*anypb.Any{
				filterName: {
					TypeUrl: typedFilter.GetTypeUrl(),
					Value:   typedFilter.GetValue(),
				},
			},
		},
	}

	// Use our vhost with tpfc in an HttpConnectionManager to be placed in a
	// FilterChain on our listener.
	hcm := &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager{
		StatPrefix: "placeholder",
		RouteSpecifier: &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager_RouteConfig{
			RouteConfig: &envoy_config_route_v3.RouteConfiguration{
				VirtualHosts: vhosts,
			},
		},
	}

	hcmAny, err := utils.MessageToAny(hcm)
	if err != nil {
		return "", err
	}
	listener := &envoy_config_listener_v3.Listener{
		Name: "placeholder_listener",
		Address: &envoy_config_core_v3.Address{
			Address: &envoy_config_core_v3.Address_SocketAddress{SocketAddress: &envoy_config_core_v3.SocketAddress{
				Address:       "0.0.0.0",
				PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{PortValue: 8081},
			}},
		},
		FilterChains: []*envoy_config_listener_v3.FilterChain{
			{
				Name: "placeholder_filter_chain",
				Filters: []*envoy_config_listener_v3.Filter{
					{
						Name: wellknown.HTTPConnectionManager,
						ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{
							TypedConfig: hcmAny,
						},
					},
				},
			},
		},
	}

	return FromEnvoyResources(&EnvoyResources{Listeners: []*envoy_config_listener_v3.Listener{listener}})
}

// FromSnapshot accepts an xds Snapshot and converts it into valid bootstrap json.
func FromSnapshot(
	ctx context.Context,
	snap envoycache.ResourceSnapshot,
) (string, error) {
	// Get the resources we're going to need as concrete types.
	resources, err := resourcesFromSnapshot(snap)
	if err != nil {
		return "", err
	}

	// This map will hold the aggregate of all cluster names that are routed to
	// by a FilterChain.
	routedCluster := map[string]struct{}{}

	// Gather up all of the clusters that we target with RouteConfigs that are associated with a FilterChain.
	if err := extractRoutedClustersFromListeners(routedCluster, resources.Listeners, resources.routes); err != nil {
		return "", err
	}

	// Next, we will look through our Snapshot's clusters and delete the ones which are
	// already routed to.
	convertToStaticClusters(routedCluster, resources.Clusters, resources.endpoints)

	// We now need to find clusters which do not exist, even though they are targeted by
	// a route. In static mode, envoy won't start without these. At this point in the
	// processing, routedClusters holds this list, so we use it to create blackhole
	// clusters for these routes to target. It is important to have unique clusters
	// for the targets since some envoy functionality relies on such setup, like
	// weighted destinations.
	resources.Clusters = addBlackholeClusters(routedCluster, resources.Clusters)

	return FromEnvoyResources(resources)
}

// extractRoutedClustersFromListeners accepts a hash set of strings containing the names of clusters
// to which routes point, a slice of pointers to Listener structs,
// and a slice of pointers to RouteConfiguration structs from the snapshot. It looks
// through the FilterChains on each Listener for an HttpConnectionManager, gets the
// routes on that hcm, and gets all of the clusters targeted by those routes. It then
// converts the hcm config to use static RouteConfiguration. routedCluster and elements
// of listeners are mutated in this function.
func extractRoutedClustersFromListeners(
	routedCluster map[string]struct{},
	listeners []*envoy_config_listener_v3.Listener,
	routes []*envoy_config_route_v3.RouteConfiguration,
) error {
	for _, l := range listeners {
		for _, fc := range l.GetFilterChains() {
			// Get the HttpConnectionManager for this FilterChain if it exists.
			hcm, f, err := getHcmForFilterChain(fc)
			if err != nil {
				// If we just don't have an hcm on this filter chain, skip to the next one.
				if errors.Is(err, errNoHcm) {
					continue
				}
				// If we encountered any other error, fail loudly.
				return err
			}

			// We use Route Discovery Service (RDS) in lieu of static route table config, so we
			// need to get the RouteConfiguration name to lookup in our Snapshot-provided routes,
			// which contain what we serve over RDS.
			routeConfigName := hcm.GetRds().GetRouteConfigName()
			if routeConfigName == "" {
				continue
			}
			// Find matching route config from snapshot.
			for _, r := range routes {
				if r.GetName() != routeConfigName {
					// These aren't the routes you're looking for.
					continue
				}

				// Add clusters targeted by routes on this config to our aggregate list of all targeted clusters
				findTargetedClusters(r, routedCluster)

				// We need to add our route table as a static config to this hcm instead of
				// relying on RDS, the we pack it back up and set it back on the filter chain.
				if err = setStaticRouteConfig(f, hcm, r); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// convertToStaticClusters accepts a hash set of strings containing the names of clusters
// to which routes point, a slice of pointers to Cluster structs,
// and a slice of pointers to ClusterLoadAssignment structs from the snapshot. It
// deletes all clusters that exist from the routedCluster hash set, then converts
// the cluster's EDS config to static config using the endpoints from the snapshot.
// clusters is mutated in this function.
func convertToStaticClusters(
	routedCluster map[string]struct{},
	clusters []*envoy_config_cluster_v3.Cluster,
	endpoints []*envoy_config_endpoint_v3.ClusterLoadAssignment,
) {
	for _, c := range clusters {
		delete(routedCluster, c.GetName())

		// We use Endpoint Discovery Service (EDS) in lieu of static endpoint config, so we
		// need to get the EDS ServiceName name to lookup in our Snapshot-provided endpoints,
		// which contain what we serve over EDS.
		if c.GetEdsClusterConfig() != nil {
			clusterName := c.GetName()
			if edsServiceName := c.GetEdsClusterConfig().GetServiceName(); edsServiceName != "" {
				clusterName = edsServiceName
			}

			// Find endpoints matching our EDS config and convert the cluster to use
			// static endpoint config matching that which would have been served over EDS.
			for _, e := range endpoints {
				if e.GetClusterName() == clusterName {
					c.LoadAssignment = e
					c.EdsClusterConfig = nil
					c.ClusterDiscoveryType = &envoy_config_cluster_v3.Cluster_Type{
						Type: envoy_config_cluster_v3.Cluster_STRICT_DNS,
					}
				}
			}
		}
	}
}

// addBlackholeClusters accepts a hash set of strings containing the names of clusters
// to which routes point and a slice of pointers to Cluster structs from the snapshot. It
// adds an cluster to clusters for each entry in the routedCluster set. clusters is mutated
// by this function.
func addBlackholeClusters(
	routedCluster map[string]struct{},
	clusters []*envoy_config_cluster_v3.Cluster,
) []*envoy_config_cluster_v3.Cluster {
	for c := range routedCluster {
		clusters = append(clusters, &envoy_config_cluster_v3.Cluster{
			Name: c,
			ClusterDiscoveryType: &envoy_config_cluster_v3.Cluster_Type{
				Type: envoy_config_cluster_v3.Cluster_STATIC,
			},
			LoadAssignment: &envoy_config_endpoint_v3.ClusterLoadAssignment{
				ClusterName: c,
				Endpoints:   []*envoy_config_endpoint_v3.LocalityLbEndpoints{},
			},
		})
	}
	return clusters
}

// getHcmForFilterChain accepts a pointer to a FilterChain and looks for the HttpConnectionManager
// network filter if one exists. It returns a pointer to the HttpConnectionManager struct and
// a pointer to the filter that actually contained it. This function has no side effects.
func getHcmForFilterChain(fc *envoy_config_listener_v3.FilterChain) (
	*envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager,
	*envoy_config_listener_v3.Filter,
	error,
) {
	for _, f := range fc.GetFilters() {
		if f.GetTypedConfig().GetTypeUrl() == "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager" {
			hcmAny, err := utils.AnyToMessage(f.GetTypedConfig())
			if err != nil {
				return nil, nil, err
			}
			// This check can be unreliable if the proto *Any format can be successfully unmarshalled to this concrete type,
			// which is surprisingly easy to do. This codepath is not tested as I was unable to force a failure, but we're
			// leaving the check in to guard against NPE from the concrete cast.
			if hcm, ok := hcmAny.(*envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager); ok {
				return hcm, f, nil
			} else {
				return nil, nil, fmt.Errorf("filter %v has hcm type url but casting to concrete failed", f.GetName())
			}
		}
	}
	return nil, nil, errNoHcm
}

// findTargetedClusters accepts a pointer to a RouteConfiguration and a hash set of strings. It
// finds all clusters and weighted clusters targeted by routes on the virtual hosts in the RouteConfiguration
// and adds their names to the routedCluster hash set. routedCluster is mutated in this function.
func findTargetedClusters(r *envoy_config_route_v3.RouteConfiguration, routedCluster map[string]struct{}) {
	for _, v := range r.GetVirtualHosts() {
		for _, r := range v.GetRoutes() {
			if r.GetRoute() == nil {
				continue
			}

			if c := r.GetRoute().GetCluster(); c != "" {
				routedCluster[c] = struct{}{}
			}
			if wc := r.GetRoute().GetWeightedClusters().GetClusters(); len(wc) != 0 {
				for _, c := range wc {
					routedCluster[c.GetName()] = struct{}{}
				}
			}
		}
	}
}

// setStaticRouteConfig accepts pointers to each of a Filter, HttpConnectionManager, and RouteConfiguration.
// It adds the RouteConfiguration to the HttpConnectionManager as static, marshals the hcm, and sets the filter's
// TypedConfig. f and hcm are mutated in this function.
func setStaticRouteConfig(
	f *envoy_config_listener_v3.Filter,
	hcm *envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager,
	r *envoy_config_route_v3.RouteConfiguration,
) error {
	hcm.RouteSpecifier = &envoy_extensions_filters_network_http_connection_manager_v3.HttpConnectionManager_RouteConfig{
		RouteConfig: r,
	}

	hcmAny, err := utils.MessageToAny(hcm)
	if err != nil {
		return err
	}

	f.ConfigType = &envoy_config_listener_v3.Filter_TypedConfig{
		TypedConfig: hcmAny,
	}
	return nil
}
