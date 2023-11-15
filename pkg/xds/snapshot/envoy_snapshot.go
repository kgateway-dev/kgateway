package snapshot

import (
	"errors"
	"fmt"

	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"

	"github.com/golang/protobuf/proto"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/types"
)

var (
	// Compile-time assertion
	_ cache.Snapshot = new(EnvoySnapshot)
)

// Snapshot is an internally consistent snapshot of xDS resources.
// Consistently is important for the convergence as different resource types
// from the snapshot may be delivered to the proxy in arbitrary order.
type EnvoySnapshot struct {
	// Endpoints are items in the EDS V3 response payload.
	Endpoints cache.Resources

	// Clusters are items in the CDS response payload.
	Clusters cache.Resources

	// Routes are items in the RDS response payload.
	Routes cache.Resources

	// Listeners are items in the LDS response payload.
	Listeners cache.Resources
}

// NewSnapshot creates a snapshot from response types and a version.
func NewSnapshot(
	version string,
	endpoints []cache.Resource,
	clusters []cache.Resource,
	routes []cache.Resource,
	listeners []cache.Resource,
) *EnvoySnapshot {
	// TODO: Copy resources
	return &EnvoySnapshot{
		Endpoints: cache.NewResources(version, endpoints),
		Clusters:  cache.NewResources(version, clusters),
		Routes:    cache.NewResources(version, routes),
		Listeners: cache.NewResources(version, listeners),
	}
}

func NewSnapshotFromResources(
	endpoints cache.Resources,
	clusters cache.Resources,
	routes cache.Resources,
	listeners cache.Resources,
) cache.Snapshot {
	// TODO: Copy resources and downgrade, maybe maintain hash to not do it too many times (https://github.com/solo-io/gloo/issues/4421)
	return &EnvoySnapshot{
		Endpoints: endpoints,
		Clusters:  clusters,
		Routes:    routes,
		Listeners: listeners,
	}
}

func NewEndpointsSnapshotFromResources(
	endpoints cache.Resources,
	clusters cache.Resources,
) cache.Snapshot {
	return &EnvoySnapshot{
		Endpoints: endpoints,
		Clusters:  clusters,
	}
}

// Consistent check verifies that the dependent resources are exactly listed in the
// snapshot:
// - all EDS resources are listed by name in CDS resources
// - all RDS resources are listed by name in LDS resources
//
// Note that clusters and listeners are requested without name references, so
// Envoy will accept the snapshot list of clusters as-is even if it does not match
// all references found in xDS.
func (s *EnvoySnapshot) Consistent() error {
	if s == nil {
		return errors.New("nil snapshot")
	}
	endpoints := resource.GetResourceReferences(s.Clusters.Items)
	if len(endpoints) != len(s.Endpoints.Items) {
		return fmt.Errorf("mismatched endpoint reference and resource lengths: length of %v does not equal length of %v", endpoints, s.Endpoints.Items)
	}
	if err := cache.SupersetWithResource(endpoints, s.Endpoints.Items); err != nil {
		return err
	}

	routes := resource.GetResourceReferences(s.Listeners.Items)
	if len(routes) != len(s.Routes.Items) {
		return fmt.Errorf("mismatched route reference and resource lengths: length of %v does not equal length of %v", routes, s.Routes.Items)
	}
	return cache.SupersetWithResource(routes, s.Routes.Items)
}

// MakeConsistent removes any items that fail to link to parent resources in the snapshot.
// It will also add placeholder routes for listeners referencing non-existent routes.
func (s *EnvoySnapshot) MakeConsistent() {
	if s == nil {
		s.Listeners = cache.Resources{
			Version: "empty",
			Items:   map[string]cache.Resource{},
		}
		s.Routes = cache.Resources{
			Version: "empty",
			Items:   map[string]cache.Resource{},
		}
		s.Clusters = cache.Resources{
			Version: "empty",
			Items:   map[string]cache.Resource{},
		}
		s.Endpoints = cache.Resources{
			Version: "empty",
			Items:   map[string]cache.Resource{},
		}
		return
	}

	// for each cluster persisted, add placeholder endpoint if referenced endpoint does not exist
	childEndpoints := resource.GetResourceReferences(s.Clusters.Items)
	persistedEndpointNameSet := map[string]struct{}{}
	for _, endpoint := range s.Endpoints.Items {
		persistedEndpointNameSet[endpoint.Self().Name] = struct{}{}
	}
	for childEndpointName, cluster := range childEndpoints {
		if _, exists := persistedEndpointNameSet[childEndpointName]; exists {
			continue
		}
		// add placeholder
		s.Endpoints.Items[childEndpointName] = resource.NewEnvoyResource(
			&envoy_config_endpoint_v3.ClusterLoadAssignment{
				ClusterName: cluster.Self().Name,
				Endpoints:   []*envoy_config_endpoint_v3.LocalityLbEndpoints{},
			})
	}

	// remove each endpoint not referenced by a cluster
	// it is safe to delete from a map you are iterating over, example in effective go https://go.dev/doc/effective_go#for
	for name, _ := range s.Endpoints.Items {
		if _, exists := childEndpoints[name]; !exists {
			delete(s.Endpoints.Items, name)
		}
	}

	// for each listener persisted, add placeholder route if referenced route does not exist
	childRoutes := resource.GetResourceReferences(s.Listeners.Items)
	persistedRouteNameSet := map[string]struct{}{}
	for _, route := range s.Routes.Items {
		persistedRouteNameSet[route.Self().Name] = struct{}{}
	}
	for childRouteName, listener := range childRoutes {
		if _, exists := persistedRouteNameSet[childRouteName]; exists {
			continue
		}
		// add placeholder
		s.Routes.Items[childRouteName] = resource.NewEnvoyResource(
			&envoy_config_route_v3.RouteConfiguration{
				Name: fmt.Sprintf("%s-%s", listener.Self().Name, "routes-for-invalid-envoy"),
				VirtualHosts: []*envoy_config_route_v3.VirtualHost{
					{
						Name:    "invalid-envoy-config-vhost",
						Domains: []string{"*"},
						Routes: []*envoy_config_route_v3.Route{
							{
								Match: &envoy_config_route_v3.RouteMatch{
									PathSpecifier: &envoy_config_route_v3.RouteMatch_Prefix{
										Prefix: "/",
									},
								},
								Action: &envoy_config_route_v3.Route_DirectResponse{
									DirectResponse: &envoy_config_route_v3.DirectResponseAction{
										Status: 500,
										Body: &envoy_config_core_v3.DataSource{
											Specifier: &envoy_config_core_v3.DataSource_InlineString{
												InlineString: "Invalid Envoy Configuration. " +
													"This placeholder was generated to localize pain to the misconfigured route",
											},
										},
									},
								},
							},
						},
					},
				},
			})
	}

	// remove each route not referenced by a listener
	// it is safe to delete from a map you are iterating over, example in effective go https://go.dev/doc/effective_go#for
	for name, _ := range s.Routes.Items {
		if _, exists := childRoutes[name]; !exists {
			delete(s.Routes.Items, name)
		}
	}
}

// GetResources selects snapshot resources by type.
func (s *EnvoySnapshot) GetResources(typ string) cache.Resources {
	if s == nil {
		return cache.Resources{}
	}
	switch typ {
	case types.EndpointTypeV3:
		return s.Endpoints
	case types.ClusterTypeV3:
		return s.Clusters
	case types.RouteTypeV3:
		return s.Routes
	case types.ListenerTypeV3:
		return s.Listeners
	}
	return cache.Resources{}
}

func (s *EnvoySnapshot) Clone() cache.Snapshot {
	snapshotClone := &EnvoySnapshot{}

	snapshotClone.Endpoints = cache.Resources{
		Version: s.Endpoints.Version,
		Items:   cloneItems(s.Endpoints.Items),
	}

	snapshotClone.Clusters = cache.Resources{
		Version: s.Clusters.Version,
		Items:   cloneItems(s.Clusters.Items),
	}

	snapshotClone.Routes = cache.Resources{
		Version: s.Routes.Version,
		Items:   cloneItems(s.Routes.Items),
	}

	snapshotClone.Listeners = cache.Resources{
		Version: s.Listeners.Version,
		Items:   cloneItems(s.Listeners.Items),
	}

	return snapshotClone
}

func cloneItems(items map[string]cache.Resource) map[string]cache.Resource {
	clonedItems := make(map[string]cache.Resource, len(items))
	for k, v := range items {
		resProto := v.ResourceProto()
		resClone := proto.Clone(resProto)
		clonedItems[k] = resource.NewEnvoyResource(resClone)
	}
	return clonedItems
}

// Equal checks is 2 snapshots are equal, important since reflect.DeepEqual no longer works with proto4
func (this *EnvoySnapshot) Equal(that *EnvoySnapshot) bool {
	if len(this.Clusters.Items) != len(that.Clusters.Items) || this.Clusters.Version != that.Clusters.Version {
		return false
	}
	for key, thisVal := range this.Clusters.Items {
		thatVal, ok := that.Clusters.Items[key]
		if !ok {
			return false
		}
		if !proto.Equal(thisVal.ResourceProto(), thatVal.ResourceProto()) {
			return false
		}
	}
	if len(this.Endpoints.Items) != len(that.Endpoints.Items) || this.Endpoints.Version != that.Endpoints.Version {
		return false
	}
	for key, thisVal := range this.Endpoints.Items {
		thatVal, ok := that.Endpoints.Items[key]
		if !ok {
			return false
		}
		if !proto.Equal(thisVal.ResourceProto(), thatVal.ResourceProto()) {
			return false
		}
	}
	if len(this.Routes.Items) != len(that.Routes.Items) || this.Routes.Version != that.Routes.Version {
		return false
	}
	for key, thisVal := range this.Routes.Items {
		thatVal, ok := that.Routes.Items[key]
		if !ok {
			return false
		}
		if !proto.Equal(thisVal.ResourceProto(), thatVal.ResourceProto()) {
			return false
		}
	}
	if len(this.Endpoints.Items) != len(that.Endpoints.Items) || this.Endpoints.Version != that.Endpoints.Version {
		return false
	}
	for key, thisVal := range this.Endpoints.Items {
		thatVal, ok := that.Endpoints.Items[key]
		if !ok {
			return false
		}
		if !proto.Equal(thisVal.ResourceProto(), thatVal.ResourceProto()) {
			return false
		}
	}
	return true
}
