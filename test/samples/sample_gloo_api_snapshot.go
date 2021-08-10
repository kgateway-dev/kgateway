package samples

import (
	"github.com/golang/protobuf/ptypes/wrappers"
	gwv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func SimpleUpstream() *v1.Upstream {
	return &v1.Upstream{
		Metadata: &core.Metadata{
			Name:      "test",
			Namespace: "gloo-system",
		},
		UpstreamType: &v1.Upstream_Static{
			Static: &static.UpstreamSpec{
				Hosts: []*static.Host{
					{
						Addr: "Test",
						Port: 124,
					},
				},
			},
		},
	}
}

func SimpleGlooSnapshot() *v1.ApiSnapshot {
	us := SimpleUpstream()
	routes := []*v1.Route{{
		Action: &v1.Route_RouteAction{
			RouteAction: &v1.RouteAction{
				Destination: &v1.RouteAction_Single{
					Single: &v1.Destination{
						DestinationType: &v1.Destination_Upstream{
							Upstream: us.GetMetadata().Ref(),
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
						Destination: &v1.TcpHost_TcpAction{
							Destination: &v1.TcpHost_TcpAction_Single{
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
		Metadata: &core.Metadata{
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

func SimpleRoute(us *core.ResourceRef) []*gwv1.Route {
	return []*gwv1.Route{{
		Matchers: []*matchers.Matcher{{
			PathSpecifier: &matchers.Matcher_Prefix{
				Prefix: "/",
			},
		}},
		Action: &gwv1.Route_RouteAction{
			RouteAction: &v1.RouteAction{
				Destination: &v1.RouteAction_Single{
					Single: &v1.Destination{
						DestinationType: &v1.Destination_Upstream{
							Upstream: us,
						},
					},
				},
			},
		},
	}}
}

func SimpleGatewaySnapshot(us *core.ResourceRef, namespace string) *gwv1.ApiSnapshot {
	routes := SimpleRoute(us)
	return &gwv1.ApiSnapshot{
		Gateways: []*gwv1.Gateway{
			defaults.DefaultGateway(namespace),
			defaults.DefaultSslGateway(namespace),
			{
				Metadata: &core.Metadata{
					Name:      "tcp-gateway",
					Namespace: namespace,
				},
				ProxyNames: []string{defaults.GatewayProxyName},
				GatewayType: &gwv1.Gateway_TcpGateway{
					TcpGateway: &gwv1.TcpGateway{
						TcpHosts: []*v1.TcpHost{
							{
								Name: "tcp-dest",
								Destination: &v1.TcpHost_TcpAction{
									Destination: &v1.TcpHost_TcpAction_Single{
										Single: &v1.Destination{
											DestinationType: &v1.Destination_Upstream{
												Upstream: us,
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
				UseProxyProto: &wrappers.BoolValue{Value: false},
			},
		},
		VirtualServices: []*gwv1.VirtualService{
			{
				Metadata: &core.Metadata{Namespace: namespace, Name: "virtualservice"},
				VirtualHost: &gwv1.VirtualHost{
					Domains: []string{"*"},
					Routes:  routes,
				},
			},
		},
	}
}

func AddVsToSnap(snap *gwv1.ApiSnapshot, us *core.ResourceRef, namespace string) *gwv1.ApiSnapshot {
	snap.VirtualServices = append(snap.VirtualServices, &gwv1.VirtualService{
		Metadata: &core.Metadata{Namespace: namespace, Name: "secondary-vs"},
		VirtualHost: &gwv1.VirtualHost{
			Domains: []string{"secondary-vs.com"},
			Routes:  SimpleRoute(us),
		},
	})
	return snap
}

func GatewaySnapshotWithDelegates(us *core.ResourceRef, namespace string) *gwv1.ApiSnapshot {
	rtRoutes := []*gwv1.Route{
		{
			Action: &gwv1.Route_RouteAction{
				RouteAction: &v1.RouteAction{
					Destination: &v1.RouteAction_Single{
						Single: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: us,
							},
						},
					},
				},
			},
		},
	}

	rt := &gwv1.RouteTable{
		Metadata: &core.Metadata{Namespace: namespace, Name: "delegated-routes"},
		Routes:   rtRoutes,
	}

	vsRoutes := []*gwv1.Route{
		{
			Action: &gwv1.Route_DelegateAction{
				DelegateAction: &gwv1.DelegateAction{
					DelegationType: &gwv1.DelegateAction_Ref{
						Ref: rt.GetMetadata().Ref(),
					},
				},
			},
		},
	}
	snap := SimpleGatewaySnapshot(us, namespace)
	snap.VirtualServices.Each(func(element *gwv1.VirtualService) {
		element.GetVirtualHost().Routes = append(element.GetVirtualHost().GetRoutes(), vsRoutes...)
	})
	snap.RouteTables = []*gwv1.RouteTable{rt}
	return snap
}

func GatewaySnapshotWithMultiDelegates(us *core.ResourceRef, namespace string) *gwv1.ApiSnapshot {
	rtLeafRoutes := []*gwv1.Route{
		{
			Action: &gwv1.Route_RouteAction{
				RouteAction: &v1.RouteAction{
					Destination: &v1.RouteAction_Single{
						Single: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: us,
							},
						},
					},
				},
			},
		},
	}

	rtLeaf := &gwv1.RouteTable{
		Metadata: &core.Metadata{Namespace: namespace, Name: "delegated-leaf-routes"},
		Routes:   rtLeafRoutes,
	}

	rtRoutes := []*gwv1.Route{
		{
			Action: &gwv1.Route_DelegateAction{
				DelegateAction: &gwv1.DelegateAction{
					DelegationType: &gwv1.DelegateAction_Ref{
						Ref: rtLeaf.GetMetadata().Ref(),
					},
				},
			},
		},
	}

	rt := &gwv1.RouteTable{
		Metadata: &core.Metadata{Namespace: namespace, Name: "delegated-routes"},
		Routes:   rtRoutes,
	}

	vsRoutes := []*gwv1.Route{
		{
			Action: &gwv1.Route_DelegateAction{
				DelegateAction: &gwv1.DelegateAction{
					DelegationType: &gwv1.DelegateAction_Ref{
						Ref: rt.GetMetadata().Ref(),
					},
				},
			},
		},
	}
	snap := SimpleGatewaySnapshot(us, namespace)
	snap.VirtualServices.Each(func(element *gwv1.VirtualService) {
		element.GetVirtualHost().Routes = append(element.GetVirtualHost().GetRoutes(), vsRoutes...)
	})
	snap.RouteTables = []*gwv1.RouteTable{rt, rtLeaf}
	return snap
}

func GatewaySnapshotWithDelegateChain(us *core.ResourceRef, namespace string) *gwv1.ApiSnapshot {
	vs, rtList := LinkedRouteTablesWithVirtualService("vs", namespace)

	snap := SimpleGatewaySnapshot(us, namespace)
	snap.VirtualServices = gwv1.VirtualServiceList{vs}
	snap.RouteTables = rtList
	return snap
}
