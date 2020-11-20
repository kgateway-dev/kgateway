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
	envoy_api_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/conversion"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds/internal"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
)

type EnvoyResource struct {
	ProtoMessage cache.ResourceProto
}

var _ cache.Resource = &EnvoyResource{}

func NewEnvoyResource(r cache.ResourceProto) *EnvoyResource {
	return &EnvoyResource{ProtoMessage: r}
}

// Resource types in xDS v3.
const (
	EndpointType = cache.TypePrefix + "/envoy.config.endpoint.v3.ClusterLoadAssignment"
	ClusterType  = cache.TypePrefix + "/envoy.config.cluster.v3.Cluster"
	RouteType    = cache.TypePrefix + "/envoy.config.route.v3.RouteConfiguration"
	ListenerType = cache.TypePrefix + "/envoy.config.listener.v3.Listener"
	SecretType   = cache.TypePrefix + "/envoy.extensions.transport_sockets.tls.v3.Secret"
	RuntimeType  = cache.TypePrefix + "/envoy.service.runtime.v3.Runtime"
)

// Fetch urls in xDS v3.
const (
	FetchEndpoints = "/v3/discovery:endpoints"
	FetchClusters  = "/v3/discovery:clusters"
	FetchListeners = "/v3/discovery:listeners"
	FetchRoutes    = "/v3/discovery:routes"
	FetchSecrets   = "/v3/discovery:secrets"
	FetchRuntimes  = "/v3/discovery:runtime"
)

// Resource types in xDS v2.
const (
	apiTypePrefix       = cache.TypePrefix + "/envoy.api.v2."
	discoveryTypePrefix = cache.TypePrefix + "/envoy.service.discovery.v2."
	EndpointTypeV2      = apiTypePrefix + "ClusterLoadAssignment"
	ClusterTypeV2       = apiTypePrefix + "Cluster"
	RouteTypeV2         = apiTypePrefix + "RouteConfiguration"
	ListenerTypeV2      = apiTypePrefix + "Listener"
	SecretTypeV2        = apiTypePrefix + "auth.Secret"
	RuntimeTypeV2       = discoveryTypePrefix + "Runtime"

	// AnyType is used only by ADS
	AnyType = ""
)

// Fetch urls in xDS v2.
const (
	FetchEndpointsV2 = "/v2/discovery:endpoints"
	FetchClustersV2  = "/v2/discovery:clusters"
	FetchListenersV2 = "/v2/discovery:listeners"
	FetchRoutesV2    = "/v2/discovery:routes"
	FetchSecretsV2   = "/v2/discovery:secrets"
	FetchRuntimesV2  = "/v2/discovery:runtime"
)

var (
	// ResponseTypes are supported response types.
	ResponseTypes = []string{
		EndpointTypeV2,
		ClusterTypeV2,
		RouteTypeV2,
		ListenerTypeV2,
	}
)

func (e *EnvoyResource) Self() cache.XdsResourceReference {
	return cache.XdsResourceReference{
		Name: e.Name(),
		Type: e.Type(),
	}
}

// GetResourceName returns the resource name for a valid xDS response type.
func (e *EnvoyResource) Name() string {
	switch v := e.ProtoMessage.(type) {
	case *envoy_api_v2.ClusterLoadAssignment:
		return v.GetClusterName()
	case *envoy_api_v2.Cluster:
		return v.GetName()
	case *envoy_api_v2.RouteConfiguration:
		return v.GetName()
	case *envoy_api_v2.Listener:
		return v.GetName()
	case *envoy_config_endpoint_v3.ClusterLoadAssignment:
		return v.GetClusterName()
	case *envoy_config_cluster_v3.Cluster:
		return v.GetName()
	case *envoy_config_route_v3.RouteConfiguration:
		return v.GetName()
	case *envoy_config_listener_v3.Listener:
		return v.GetName()
	default:
		return ""
	}
}

func (e *EnvoyResource) ResourceProto() cache.ResourceProto {
	return e.ProtoMessage
}

func (e *EnvoyResource) Type() string {
	switch e.ProtoMessage.(type) {
	case *envoy_api_v2.ClusterLoadAssignment:
		return EndpointTypeV2
	case *envoy_api_v2.Cluster:
		return ClusterTypeV2
	case *envoy_api_v2.RouteConfiguration:
		return RouteTypeV2
	case *envoy_api_v2.Listener:
		return ListenerTypeV2
	case *envoy_config_endpoint_v3.ClusterLoadAssignment:
		return EndpointType
	case *envoy_config_cluster_v3.Cluster:
		return ClusterType
	case *envoy_config_route_v3.RouteConfiguration:
		return RouteType
	case *envoy_config_listener_v3.Listener:
		return ListenerType
	default:
		return ""
	}
}

func (e *EnvoyResource) References() []cache.XdsResourceReference {
	out := make(map[cache.XdsResourceReference]bool)
	res := e.ProtoMessage
	if res == nil {
		return nil
	}
	switch v := res.(type) {
	case *envoy_api_v2.ClusterLoadAssignment:
		// no dependencies
	case *envoy_api_v2.Cluster:
		// for EDS type, use cluster name or ServiceName override
		if v.GetType() == envoy_api_v2.Cluster_EDS {
			rr := cache.XdsResourceReference{
				Type: EndpointTypeV2,
			}
			if v.EdsClusterConfig != nil && v.EdsClusterConfig.ServiceName != "" {
				rr.Name = v.EdsClusterConfig.ServiceName
			} else {
				rr.Name = v.Name
			}
			out[rr] = true
		}
	case *envoy_api_v2.RouteConfiguration:
		// References to clusters in both routes (and listeners) are not included
		// in the result, because the clusters are retrieved in bulk currently,
		// and not by name.
	case *envoy_api_v2.Listener:
		// extract route configuration names from HTTP connection manager
		for _, chain := range v.FilterChains {
			for _, filter := range chain.Filters {
				if filter.Name != wellknown.HTTPConnectionManager {
					continue
				}

				config := &hcm.HttpConnectionManager{}

				switch filterConfig := filter.ConfigType.(type) {
				case *listener.Filter_Config:
					if conversion.StructToMessage(filterConfig.Config, config) != nil {
						continue

					}
				case *listener.Filter_TypedConfig:
					if ptypes.UnmarshalAny(filterConfig.TypedConfig, config) != nil {
						continue
					}
				}

				if rds, ok := config.RouteSpecifier.(*hcm.HttpConnectionManager_Rds); ok && rds != nil && rds.Rds != nil {
					rr := cache.XdsResourceReference{
						Type: RouteTypeV2,
						Name: rds.Rds.RouteConfigName,
					}
					out[rr] = true
				}
			}
		}

	case *envoy_config_endpoint_v3.ClusterLoadAssignment:
		// no dependencies
	case *envoy_config_cluster_v3.Cluster:
		// for EDS type, use cluster name or ServiceName override
		if v.GetType() == envoy_config_cluster_v3.Cluster_EDS {
			rr := cache.XdsResourceReference{
				Type: EndpointType,
			}
			if v.GetEdsClusterConfig().GetServiceName() != "" {
				rr.Name = v.GetEdsClusterConfig().GetServiceName()
			} else {
				rr.Name = v.GetName()
			}
			out[rr] = true
		}
	case *envoy_config_route_v3.RouteConfiguration:
		// References to clusters in both routes (and listeners) are not included
		// in the result, because the clusters are retrieved in bulk currently,
		// and not by name.
	case *envoy_config_listener_v3.Listener:
		// extract route configuration names from HTTP connection manager
		for _, chain := range v.GetFilterChains() {
			for _, filter := range chain.GetFilters() {
				if filter.Name != wellknown.HTTPConnectionManager {
					continue
				}

				config := hcm.HttpConnectionManager{}
				if err := ptypes.UnmarshalAny(filter.GetTypedConfig(), &config); err != nil {
					continue
				}

				if config.GetRds() != nil {
					rr := cache.XdsResourceReference{
						Type: RouteType,
						Name: config.GetRds().GetRouteConfigName(),
					}
					out[rr] = true
				}
			}
		}
	}

	var references []cache.XdsResourceReference
	for k, _ := range out {
		references = append(references, k)
	}
	return references
}

func downgradeResource(e cache.Resource) *EnvoyResource {
	var downgradedResource cache.ResourceProto
	res := e.ResourceProto()
	if res == nil {
		return nil
	}
	switch v := res.(type) {
	case *envoy_config_endpoint_v3.ClusterLoadAssignment:
		// No downgrade necessary
	case *envoy_config_cluster_v3.Cluster:
		downgradedResource = internal.DowngradeCluster(v)
	case *envoy_config_route_v3.RouteConfiguration:
		// No downgrade necessary
	case *envoy_config_listener_v3.Listener:
		downgradedResource = internal.DowngradeListener(v)
	}
	return &EnvoyResource{ProtoMessage: downgradedResource}
}

// GetResourceReferences returns the names for dependent resources (EDS cluster
// names for CDS, RDS routes names for LDS).
func GetResourceReferences(resources map[string]cache.Resource) map[string]bool {
	out := make(map[string]bool)
	for _, res := range resources {
		if res == nil {
			continue
		}
		switch v := res.ResourceProto().(type) {
		case *envoy_api_v2.ClusterLoadAssignment:
			// no dependencies
		case *envoy_api_v2.Cluster:
			// for EDS type, use cluster name or ServiceName override
			if v.GetType() == envoy_api_v2.Cluster_EDS {
				if v.EdsClusterConfig != nil && v.EdsClusterConfig.ServiceName != "" {
					out[v.EdsClusterConfig.ServiceName] = true
				} else {
					out[v.Name] = true
				}
			}
		case *envoy_api_v2.RouteConfiguration:
			// References to clusters in both routes (and listeners) are not included
			// in the result, because the clusters are retrieved in bulk currently,
			// and not by name.
		case *envoy_api_v2.Listener:
			// extract route configuration names from HTTP connection manager
			for _, chain := range v.FilterChains {
				for _, filter := range chain.Filters {
					if filter.Name != wellknown.HTTPConnectionManager {
						continue
					}

					config := &hcm.HttpConnectionManager{}

					switch filterConfig := filter.ConfigType.(type) {
					case *listener.Filter_Config:
						if conversion.StructToMessage(filterConfig.Config, config) != nil {
							continue

						}
					case *listener.Filter_TypedConfig:
						if ptypes.UnmarshalAny(filterConfig.TypedConfig, config) != nil {
							continue
						}
					}

					if rds, ok := config.RouteSpecifier.(*hcm.HttpConnectionManager_Rds); ok && rds != nil && rds.Rds != nil {
						out[rds.Rds.GetRouteConfigName()] = true
					}

				}
			}

		case *envoy_config_endpoint_v3.ClusterLoadAssignment:
			// no dependencies
		case *envoy_config_cluster_v3.Cluster:
			// for EDS type, use cluster name or ServiceName override
			if v.GetType() == envoy_config_cluster_v3.Cluster_EDS {
				if v.GetEdsClusterConfig().GetServiceName() != "" {
					out[v.GetEdsClusterConfig().GetServiceName()] = true
				} else {
					out[v.GetName()] = true
				}
			}
		case *envoy_config_route_v3.RouteConfiguration:
			// References to clusters in both routes (and listeners) are not included
			// in the result, because the clusters are retrieved in bulk currently,
			// and not by name.
		case *envoy_config_listener_v3.Listener:
			// extract route configuration names from HTTP connection manager
			for _, chain := range v.GetFilterChains() {
				for _, filter := range chain.GetFilters() {
					if filter.Name != wellknown.HTTPConnectionManager {
						continue
					}

					config := hcm.HttpConnectionManager{}
					if err := ptypes.UnmarshalAny(filter.GetTypedConfig(), &config); err != nil {
						continue
					}

					if config.GetRds() != nil {
						out[config.GetRds().GetRouteConfigName()] = true
					}
				}
			}
		}
	}
	return out
}

// GetResourceName returns the resource name for a valid xDS response type.
func GetResourceName(res cache.ResourceProto) string {
	switch v := res.(type) {
	case *envoy_api_v2.ClusterLoadAssignment:
		return v.GetClusterName()
	case *envoy_api_v2.Cluster:
		return v.GetName()
	case *envoy_api_v2.RouteConfiguration:
		return v.GetName()
	case *envoy_api_v2.Listener:
		return v.GetName()
	case *envoy_config_endpoint_v3.ClusterLoadAssignment:
		return v.GetClusterName()
	case *envoy_config_cluster_v3.Cluster:
		return v.GetName()
	case *envoy_config_route_v3.RouteConfiguration:
		return v.GetName()
	case *envoy_config_listener_v3.Listener:
		return v.GetName()
	default:
		return ""
	}
}
