package samples

import (
	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/utils"
	gwv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	v2 "github.com/solo-io/gloo/projects/gateway/pkg/api/v2"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/static"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func SimpleUpstream() *v1.Upstream {
	return &v1.Upstream{
		Metadata: core.Metadata{
			Name:      "test",
			Namespace: "gloo-system",
		},
		UpstreamSpec: &v1.UpstreamSpec{
			UpstreamType: &v1.UpstreamSpec_Static{
				Static: &static.UpstreamSpec{
					Hosts: []*static.Host{
						{
							Addr: "Test",
							Port: 124,
						},
					},
				},
			},
		},
	}
}

func SimpleGlooSnapshot() *v1.ApiSnapshot {
	us := SimpleUpstream()
	matcher := &v1.Matcher{
		PathSpecifier: &v1.Matcher_Prefix{
			Prefix: "/",
		},
	}
	routes := []*v1.Route{{
		Matcher: matcher,
		Action: &v1.Route_RouteAction{
			RouteAction: &v1.RouteAction{
				Destination: &v1.RouteAction_Single{
					Single: &v1.Destination{
						DestinationType: &v1.Destination_Upstream{
							Upstream: utils.ResourceRefPtr(us.Metadata.Ref()),
						},
					},
				},
			},
		},
	}}

	httpListener := &v1.Listener{
		Name:        "http-listener",
		BindAddress: "127.0.0.1",
		BindPort:    80,
		ListenerType: &v1.Listener_HttpListener{
			HttpListener: &v1.HttpListener{
				VirtualHosts: []*v1.VirtualHost{{
					Name:    "virt1",
					Domains: []string{"*"},
					Routes:  routes,
				}},
			},
		},
	}
	tcpListener := &v1.Listener{
		Name:        "tcp-listener",
		BindAddress: "127.0.0.1",
		BindPort:    8080,
		ListenerType: &v1.Listener_TcpListener{
			TcpListener: &v1.TcpListener{
				TcpHosts: []*v1.TcpHost{
					{
						Destination: &v1.RouteAction{
							Destination: &v1.RouteAction_Single{
								Single: &v1.Destination{
									DestinationType: &v1.Destination_Upstream{
										Upstream: &core.ResourceRef{
											Name:      "test",
											Namespace: "gloo-system",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	proxy := &v1.Proxy{
		Metadata: core.Metadata{
			Name:      "test",
			Namespace: "gloo-system",
		},
		Listeners: []*v1.Listener{
			httpListener,
			tcpListener,
		},
	}

	return &v1.ApiSnapshot{
		Proxies:   []*v1.Proxy{proxy},
		Upstreams: []*v1.Upstream{us},
	}
}

func SimpleGatewaySnapshot(us core.ResourceRef, namespace string) *v2.ApiSnapshot {
	routes := []*gwv1.Route{{
		Matcher: &v1.Matcher{
			PathSpecifier: &v1.Matcher_Prefix{
				Prefix: "/",
			},
		},
		Action: &gwv1.Route_RouteAction{
			RouteAction: &v1.RouteAction{
				Destination: &v1.RouteAction_Single{
					Single: &v1.Destination{
						DestinationType: &v1.Destination_Upstream{
							Upstream: utils.ResourceRefPtr(us),
						},
					},
				},
			},
		},
	}}
	return &v2.ApiSnapshot{
		Gateways: []*v2.Gateway{
			defaults.DefaultGateway(namespace),
			defaults.DefaultSslGateway(namespace),
			{
				Metadata: core.Metadata{
					Name:      "tcp-gateway",
					Namespace: namespace,
				},
				ProxyNames: []string{defaults.GatewayProxyName},
				GatewayType: &v2.Gateway_TcpGateway{
					TcpGateway: &v2.TcpGateway{
						Destinations: []*v1.TcpHost{
							{
								Name: "tcp-dest",
								Destination: &v1.RouteAction{
									Destination: &v1.RouteAction_Single{
										Single: &v1.Destination{
											DestinationType: &v1.Destination_Upstream{
												Upstream: utils.ResourceRefPtr(us),
											},
										},
									},
								},
							},
						},
					},
				},
				BindAddress:   "::",
				BindPort:      12345,
				UseProxyProto: &types.BoolValue{Value: false},
			},
		},
		VirtualServices: []*gwv1.VirtualService{
			{
				Metadata: core.Metadata{Namespace: namespace, Name: "virtualservice"},
				VirtualHost: &gwv1.VirtualHost{
					Domains: []string{"*"},
					Routes:  routes,
				},
			},
		},
	}
}

func GatewaySnapshotWithDelegates(us core.ResourceRef, namespace string) *v2.ApiSnapshot {
	rtRoutes := []*gwv1.Route{
		{
			Matcher: &v1.Matcher{
				PathSpecifier: &v1.Matcher_Prefix{
					Prefix: "/",
				},
			},
			Action: &gwv1.Route_RouteAction{
				RouteAction: &v1.RouteAction{
					Destination: &v1.RouteAction_Single{
						Single: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: utils.ResourceRefPtr(us),
							},
						},
					},
				},
			},
		},
	}

	rt := &gwv1.RouteTable{
		Metadata: core.Metadata{Namespace: namespace, Name: "delegated-routes"},
		Routes:   rtRoutes,
	}

	vsRoutes := []*gwv1.Route{
		{
			Matcher: &v1.Matcher{
				PathSpecifier: &v1.Matcher_Prefix{
					Prefix: "/",
				},
			},
			Action: &gwv1.Route_DelegateAction{
				DelegateAction: utils.ResourceRefPtr(rt.Metadata.Ref()),
			},
		},
	}
	snap := SimpleGatewaySnapshot(us, namespace)
	snap.VirtualServices.Each(func(element *gwv1.VirtualService) {
		element.VirtualHost.Routes = append(element.VirtualHost.Routes, vsRoutes...)
	})
	snap.RouteTables = []*gwv1.RouteTable{rt}
	return snap
}

func GatewaySnapshotWithMultiDelegates(us core.ResourceRef, namespace string) *v2.ApiSnapshot {
	rtLeafRoutes := []*gwv1.Route{
		{
			Matcher: &v1.Matcher{
				PathSpecifier: &v1.Matcher_Prefix{
					Prefix: "/",
				},
			},
			Action: &gwv1.Route_RouteAction{
				RouteAction: &v1.RouteAction{
					Destination: &v1.RouteAction_Single{
						Single: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: utils.ResourceRefPtr(us),
							},
						},
					},
				},
			},
		},
	}

	rtLeaf := &gwv1.RouteTable{
		Metadata: core.Metadata{Namespace: namespace, Name: "delegated-leaf-routes"},
		Routes:   rtLeafRoutes,
	}

	rtRoutes := []*gwv1.Route{
		{
			Matcher: &v1.Matcher{
				PathSpecifier: &v1.Matcher_Prefix{
					Prefix: "/",
				},
			},
			Action: &gwv1.Route_DelegateAction{
				DelegateAction: utils.ResourceRefPtr(rtLeaf.Metadata.Ref()),
			},
		},
	}

	rt := &gwv1.RouteTable{
		Metadata: core.Metadata{Namespace: namespace, Name: "delegated-routes"},
		Routes:   rtRoutes,
	}

	vsRoutes := []*gwv1.Route{
		{
			Matcher: &v1.Matcher{
				PathSpecifier: &v1.Matcher_Prefix{
					Prefix: "/",
				},
			},
			Action: &gwv1.Route_DelegateAction{
				DelegateAction: utils.ResourceRefPtr(rt.Metadata.Ref()),
			},
		},
	}
	snap := SimpleGatewaySnapshot(us, namespace)
	snap.VirtualServices.Each(func(element *gwv1.VirtualService) {
		element.VirtualHost.Routes = append(element.VirtualHost.Routes, vsRoutes...)
	})
	snap.RouteTables = []*gwv1.RouteTable{rt, rtLeaf}
	return snap
}

func GatewaySnapshotWithDelegateChain(us core.ResourceRef, namespace string) *v2.ApiSnapshot {
	vs, rtList := LinkedRouteTablesWithVirtualService("vs", namespace)

	snap := SimpleGatewaySnapshot(us, namespace)
	snap.VirtualServices = gwv1.VirtualServiceList{vs}
	snap.RouteTables = rtList
	return snap
}
