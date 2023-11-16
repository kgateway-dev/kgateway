package server

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/solo-io/gloo/v2/pkg/translator/utils"
	xdssnapshot "github.com/solo-io/gloo/v2/pkg/xds/snapshot"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
)

const fallbackBindPort = 8080

const (
	fallbackBindAddr   = "::"
	fallbackStatusCode = 500
)

// createFallbackSnapshot returns a snapshot that is served to proxies which
// do not contain a node identifier, as a way of signaling that they are invalid
func createFallbackSnapshot() cache.Snapshot {
	return fallbackSnapshot(fallbackBindAddr, fallbackBindPort, fallbackStatusCode)
}

func fallbackSnapshot(bindAddress string, port, invalidConfigStatusCode uint32) cache.Snapshot {
	routeConfigName := "routes-for-invalid-envoy"
	listenerName := "listener-for-invalid-envoy"
	var (
		endpoints []cache.Resource
		clusters  []cache.Resource
	)
	routes := []cache.Resource{
		resource.NewEnvoyResource(&envoy_config_route_v3.RouteConfiguration{
			Name: routeConfigName,
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
									Status: invalidConfigStatusCode,
									Body: &envoy_config_core_v3.DataSource{
										Specifier: &envoy_config_core_v3.DataSource_InlineString{
											InlineString: "Invalid Envoy Bootstrap Configuration. " +
												"Please refer to Gloo documentation https://gloo.solo.io/",
										},
									},
								},
							},
						},
					},
				},
			},
		}),
	}
	adsSource := envoy_config_core_v3.ConfigSource{
		ConfigSourceSpecifier: &envoy_config_core_v3.ConfigSource_Ads{
			Ads: &envoy_config_core_v3.AggregatedConfigSource{},
		},
	}
	manager := &envoyhcm.HttpConnectionManager{
		CodecType:  envoyhcm.HttpConnectionManager_AUTO,
		StatPrefix: "http",
		RouteSpecifier: &envoyhcm.HttpConnectionManager_Rds{
			Rds: &envoyhcm.Rds{
				ConfigSource:    &adsSource,
				RouteConfigName: routeConfigName,
			},
		},
		HttpFilters: []*envoyhcm.HttpFilter{
			{
				Name: wellknown.Router,
				ConfigType: &envoyhcm.HttpFilter_TypedConfig{
					TypedConfig: &any.Any{
						TypeUrl: "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router",
					},
				},
			},
		},
	}
	pbst := utils.ToAny(manager)

	listener := &envoy_config_listener_v3.Listener{
		Name: listenerName,
		Address: &envoy_config_core_v3.Address{
			Address: &envoy_config_core_v3.Address_SocketAddress{
				SocketAddress: &envoy_config_core_v3.SocketAddress{
					Protocol: envoy_config_core_v3.SocketAddress_TCP,
					Address:  bindAddress,
					PortSpecifier: &envoy_config_core_v3.SocketAddress_PortValue{
						PortValue: port,
					},
					Ipv4Compat: true,
				},
			},
		},
		FilterChains: []*envoy_config_listener_v3.FilterChain{{
			Filters: []*envoy_config_listener_v3.Filter{
				{
					Name:       wellknown.HTTPConnectionManager,
					ConfigType: &envoy_config_listener_v3.Filter_TypedConfig{TypedConfig: pbst},
				},
			},
		}},
	}

	listeners := []cache.Resource{
		resource.NewEnvoyResource(listener),
	}
	return xdssnapshot.NewSnapshot("unversioned", endpoints, clusters, routes, listeners)
}
