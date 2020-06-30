package xds

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycorev2 "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyhttpconnectionmanager "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
)

func fallbackSnapshot(bindAddress string, port, invalidConfigStatusCode uint32) cache.Snapshot {
	routeConfigName := "routes-for-invalid-envoy"
	listenerName := "listener-for-invalid-envoy"
	var (
		endpoints []cache.Resource
		clusters  []cache.Resource
	)
	routes := []cache.Resource{
		NewEnvoyResource(&envoyapi.RouteConfiguration{
			Name: routeConfigName,
			VirtualHosts: []*envoyroute.VirtualHost{
				{
					Name:    "invalid-envoy-config-vhost",
					Domains: []string{"*"},
					Routes: []*envoyroute.Route{
						{
							Match: &envoyroute.RouteMatch{
								PathSpecifier: &envoyroute.RouteMatch_Prefix{
									Prefix: "/",
								},
							},
							Action: &envoyroute.Route_DirectResponse{
								DirectResponse: &envoyroute.DirectResponseAction{
									Status: invalidConfigStatusCode,
									Body: &envoycorev2.DataSource{
										Specifier: &envoycorev2.DataSource_InlineString{
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
	adsSource := envoycore.ConfigSource{
		ConfigSourceSpecifier: &envoycore.ConfigSource_Ads{
			Ads: &envoycore.AggregatedConfigSource{},
		},
	}
	manager := &envoyhttpconnectionmanager.HttpConnectionManager{
		CodecType:  envoyhttpconnectionmanager.HttpConnectionManager_AUTO,
		StatPrefix: "http",
		RouteSpecifier: &envoyhttpconnectionmanager.HttpConnectionManager_Rds{
			Rds: &envoyhttpconnectionmanager.Rds{
				ConfigSource:    &adsSource,
				RouteConfigName: routeConfigName,
			},
		},
		HttpFilters: []*envoyhttpconnectionmanager.HttpFilter{
			{
				Name: wellknown.Router,
			},
		},
	}
	pbst, err := utils.MessageToAny(manager)
	if err != nil {
		panic(err)
	}

	listener := &envoyapi.Listener{
		Name: listenerName,
		Address: &envoycorev2.Address{
			Address: &envoycorev2.Address_SocketAddress{
				SocketAddress: &envoycorev2.SocketAddress{
					Protocol: envoycorev2.SocketAddress_TCP,
					Address:  bindAddress,
					PortSpecifier: &envoycorev2.SocketAddress_PortValue{
						PortValue: port,
					},
					Ipv4Compat: true,
				},
			},
		},
		FilterChains: []*envoylistener.FilterChain{{
			Filters: []*envoylistener.Filter{
				{
					Name:       wellknown.HTTPConnectionManager,
					ConfigType: &envoylistener.Filter_TypedConfig{TypedConfig: pbst},
				},
			},
		}},
	}

	listeners := []cache.Resource{
		NewEnvoyResource(listener),
	}
	return NewSnapshot("unversioned", endpoints, clusters, routes, listeners)
}
