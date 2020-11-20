// Copyright 2018 Envoyproxy Authors
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.

package xds

import (
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
)

// Snapshot is an internally consistent snapshot of xDS resources.
// Consistently is important for the convergence as different resource types
// from the snapshot may be delivered to the proxy in arbitrary order.
type EnvoySnapshot struct {
	// Endpoints are items in the EDS V3 response payload.
	Endpoints cache.Resources

	// hiddenDeprecatedEndpoints are items in the EDS V2 response payload.
	hiddenDeprecatedEndpoints cache.Resources

	// Clusters are items in the CDS response payload.
	Clusters cache.Resources

	// hiddenDeprecatedClusters are items in the EDS V2 response payload.
	hiddenDeprecatedClusters cache.Resources

	// Routes are items in the RDS response payload.
	Routes cache.Resources

	// hiddenDeprecatedRoutes are items in the EDS V2 response payload.
	hiddenDeprecatedRoutes cache.Resources

	// Listeners are items in the LDS response payload.
	Listeners cache.Resources
	// hiddenDeprecatedListeners are items in the EDS V2 response payload.
	hiddenDeprecatedListeners cache.Resources
}

var _ cache.Snapshot = &EnvoySnapshot{}

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
		Endpoints:                 cache.NewResources(version, endpoints),
		hiddenDeprecatedEndpoints: cache.NewResources(version, nil),
		Clusters:                  cache.NewResources(version, clusters),
		hiddenDeprecatedClusters:  downgradeCacheResourceList(version, clusters),
		Routes:                    cache.NewResources(version, routes),
		hiddenDeprecatedRoutes:    cache.NewResources(version, nil),
		Listeners:                 cache.NewResources(version, listeners),
		hiddenDeprecatedListeners: downgradeCacheResourceList(version, clusters),
	}
}

func NewSnapshotFromResources(
	endpoints cache.Resources,
	clusters cache.Resources,
	routes cache.Resources,
	listeners cache.Resources,
) cache.Snapshot {
	// TODO: Copy resources and downgrade, maybe maintain hash to not do it too many times
	return &EnvoySnapshot{
		Endpoints:                 endpoints,
		Clusters:                  clusters,
		hiddenDeprecatedClusters:  downgradeCacheResources(clusters),
		Routes:                    routes,
		Listeners:                 listeners,
		hiddenDeprecatedListeners: downgradeCacheResources(listeners),
	}
}

func downgradeCacheResources(resources cache.Resources) cache.Resources {
	newResources := make([]cache.Resource, 0, len(resources.Items))
	for _, v := range resources.Items {
		downgradedResource := downgradeResource(v)
		if downgradedResource != nil {
			newResources = append(newResources, downgradedResource)
		}
	}
	return cache.NewResources(resources.Version, newResources)
}

func downgradeCacheResourceList(version string, resources []cache.Resource) cache.Resources {
	newResources := make([]cache.Resource, 0, len(resources))
	for _, v := range resources {
		downgradedResource := downgradeResource(v)
		if downgradedResource != nil {
			newResources = append(newResources, downgradedResource)
		}
	}
	return cache.NewResources(version, newResources)
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
	endpoints := GetResourceReferences(s.Clusters.Items)
	if len(endpoints) != len(s.Endpoints.Items) {
		return fmt.Errorf("mismatched endpoint reference and resource lengths: length of %v does not equal length of %v", endpoints, s.Endpoints.Items)
	}
	if err := cache.Superset(endpoints, s.Endpoints.Items); err != nil {
		return err
	}

	routes := GetResourceReferences(s.Listeners.Items)
	if len(routes) != len(s.Routes.Items) {
		return fmt.Errorf("mismatched route reference and resource lengths: length of %v does not equal length of %v", routes, s.Routes.Items)
	}
	return cache.Superset(routes, s.Routes.Items)
}

// GetResources selects snapshot resources by type.
func (s *EnvoySnapshot) GetResources(typ string) cache.Resources {
	if s == nil {
		return cache.Resources{}
	}
	switch typ {
	case EndpointType:
		return s.Endpoints
	case ClusterType:
		return s.Clusters
	case RouteType:
		return s.Routes
	case ListenerType:
		return s.Listeners
	case EndpointTypeV2:
		return s.hiddenDeprecatedEndpoints
	case ClusterTypeV2:
		return s.hiddenDeprecatedClusters
	case RouteTypeV2:
		return s.hiddenDeprecatedRoutes
	case ListenerTypeV2:
		return s.hiddenDeprecatedListeners
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

	snapshotClone.hiddenDeprecatedEndpoints = cache.Resources{
		Version: s.hiddenDeprecatedEndpoints.Version,
		Items:   cloneItems(s.hiddenDeprecatedEndpoints.Items),
	}

	snapshotClone.hiddenDeprecatedClusters = cache.Resources{
		Version: s.hiddenDeprecatedClusters.Version,
		Items:   cloneItems(s.hiddenDeprecatedClusters.Items),
	}

	snapshotClone.hiddenDeprecatedRoutes = cache.Resources{
		Version: s.hiddenDeprecatedRoutes.Version,
		Items:   cloneItems(s.hiddenDeprecatedRoutes.Items),
	}

	snapshotClone.hiddenDeprecatedListeners = cache.Resources{
		Version: s.hiddenDeprecatedListeners.Version,
		Items:   cloneItems(s.hiddenDeprecatedListeners.Items),
	}

	return snapshotClone
}

func cloneItems(items map[string]cache.Resource) map[string]cache.Resource {
	clonedItems := make(map[string]cache.Resource, len(items))
	for k, v := range items {
		resProto := v.ResourceProto()
		// NOTE(marco): we have to use `github.com/golang/protobuf/proto.Clone()` to clone here,
		// `github.com/gogo/protobuf/proto.Clone()` will panic!
		resClone := proto.Clone(resProto)
		clonedItems[k] = NewEnvoyResource(resClone)
	}
	return clonedItems
}
