package helpers

import (
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/golang/protobuf/ptypes/wrappers"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// ScaledSnapshotBuilder enumerates the number of each type of resource that should be included in a snapshot and
// contains a builder for each sub-resource type which is responsible for building instances of that resource
// Additional fields should be added as needed
type ScaledSnapshotBuilder struct {
	injectedSnap *gloosnapshot.ApiSnapshot

	epCount int
	usCount int

	epBuilder *EndpointBuilder
	usBuilder *UpstreamBuilder
}

func NewScaledSnapshotBuilder() *ScaledSnapshotBuilder {
	return &ScaledSnapshotBuilder{
		epBuilder: NewEndpointBuilder(),
		usBuilder: NewUpstreamBuilder(),
	}
}

// NewInjectedSnapshotBuilder takes a snapshot object to be returned directly by Build()
// All other settings on a builder with an InjectedSnapshot will be ignored
func NewInjectedSnapshotBuilder(snap *gloosnapshot.ApiSnapshot) *ScaledSnapshotBuilder {
	return &ScaledSnapshotBuilder{
		injectedSnap: snap,
	}
}

func (b *ScaledSnapshotBuilder) WithUpstreamCount(n int) *ScaledSnapshotBuilder {
	b.usCount = n
	return b
}

func (b *ScaledSnapshotBuilder) WithUpstreamBuilder(ub *UpstreamBuilder) *ScaledSnapshotBuilder {
	b.usBuilder = ub
	return b
}

func (b *ScaledSnapshotBuilder) WithEndpointCount(n int) *ScaledSnapshotBuilder {
	b.epCount = n
	return b
}

func (b *ScaledSnapshotBuilder) WithEndpointBuilder(eb *EndpointBuilder) *ScaledSnapshotBuilder {
	b.epBuilder = eb
	return b
}

/* Getter funcs to be used by the description generator */

func (b *ScaledSnapshotBuilder) HasInjectedSnapshot() bool {
	return b.injectedSnap != nil
}

func (b *ScaledSnapshotBuilder) UpstreamCount() int {
	return b.usCount
}

func (b *ScaledSnapshotBuilder) EndpointCount() int {
	return b.epCount
}

// Build generates a snapshot populated with the specified number of each resource for the builder, using the
// sub-resource builders to build each sub-resource
func (b *ScaledSnapshotBuilder) Build() *gloosnapshot.ApiSnapshot {
	if b.injectedSnap != nil {
		return b.injectedSnap
	}

	endpointList := make(v1.EndpointList, b.epCount)
	for i := 0; i < b.epCount; i++ {
		endpointList[i] = b.epBuilder.Build(i)
	}

	upstreamList := make(v1.UpstreamList, b.usCount)
	for i := 0; i < b.usCount; i++ {
		upstreamList[i] = b.usBuilder.Build(i)
	}

	return &gloosnapshot.ApiSnapshot{
		// The proxy should contain a route for each upstream
		Proxies: []*v1.Proxy{Proxy(b.usCount)},

		Endpoints: endpointList,
		Upstreams: upstreamList,
	}
}

func upMeta(i int) *core.Metadata {
	return &core.Metadata{
		Name:      fmt.Sprintf("test-%06d", i),
		Namespace: defaults.GlooSystem,
	}
}

// Upstream returns a generic upstream included in snapshots generated from ScaledSnapshot
// The integer argument is used to create a uniquely-named resource
func Upstream(i int) *v1.Upstream {
	return &v1.Upstream{
		Metadata: upMeta(i),
		UpstreamType: &v1.Upstream_Static{
			Static: &v1static.UpstreamSpec{
				Hosts: []*v1static.Host{
					{
						Addr: "Test",
						Port: 124,
					},
				},
			},
		},
	}
}

// Endpoint returns a generic endpoint included in snapshots generated from ScaledSnapshot
// The integer argument is used to create a uniquely-named resource which references a corresponding Upstream
func Endpoint(i int) *v1.Endpoint {
	return &v1.Endpoint{
		Upstreams: []*core.ResourceRef{upMeta(i).Ref()},
		Address:   "1.2.3.4",
		Port:      32,
		Metadata: &core.Metadata{
			Name:      fmt.Sprintf("test-ep-%06d", i),
			Namespace: defaults.GlooSystem,
		},
	}
}

var matcher = &matchers.Matcher{
	PathSpecifier: &matchers.Matcher_Prefix{
		Prefix: "/",
	},
}

func route(i int) *v1.Route {
	return &v1.Route{
		Name:     "testRouteName",
		Matchers: []*matchers.Matcher{matcher},
		Action: &v1.Route_RouteAction{
			RouteAction: &v1.RouteAction{
				Destination: &v1.RouteAction_Single{
					Single: &v1.Destination{
						DestinationType: &v1.Destination_Upstream{
							Upstream: upMeta(i).Ref(),
						},
					},
				},
			},
		},
	}
}

func routes(n int) []*v1.Route {
	routes := make([]*v1.Route, n)
	for i := 0; i < n; i++ {
		routes[i] = route(i)
	}
	return routes
}

var virtualHostName = "virt1"

// HttpListener returns a generic Listener with HttpListener ListenerType and the specified number of routes
func HttpListener(numRoutes int) *v1.Listener {
	return &v1.Listener{
		Name:        "http-listener",
		BindAddress: "127.0.0.1",
		BindPort:    80,
		ListenerType: &v1.Listener_HttpListener{
			HttpListener: &v1.HttpListener{
				VirtualHosts: []*v1.VirtualHost{{
					Name:    virtualHostName,
					Domains: []string{"*"},
					Routes:  routes(numRoutes),
				}},
			},
		},
	}
}

// tcpListener invokes functions that contain assertions and therefore can only be invoked from within a test block
func tcpListener() *v1.Listener {
	return &v1.Listener{
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
											Name:      upMeta(0).GetName(),
											Namespace: upMeta(0).GetNamespace(),
										},
									},
								},
							},
						},
						SslConfig: &ssl.SslConfig{
							SslSecrets: &ssl.SslConfig_SslFiles{
								SslFiles: &ssl.SSLFiles{
									TlsCert: Certificate(),
									TlsKey:  PrivateKey(),
								},
							},
							SniDomains: []string{
								"sni1",
							},
						},
					},
				},
			},
		},
	}
}

// hybridListener invokes functions that contain assertions and therefore can only be invoked from within a test block
func hybridListener(numRoutes int) *v1.Listener {
	return &v1.Listener{
		Name:        "hybrid-listener",
		BindAddress: "127.0.0.1",
		BindPort:    8888,
		ListenerType: &v1.Listener_HybridListener{
			HybridListener: &v1.HybridListener{
				MatchedListeners: []*v1.MatchedListener{
					{
						Matcher: &v1.Matcher{
							SslConfig: &ssl.SslConfig{
								SslSecrets: &ssl.SslConfig_SslFiles{
									SslFiles: &ssl.SSLFiles{
										TlsCert: Certificate(),
										TlsKey:  PrivateKey(),
									},
								},
								SniDomains: []string{
									"sni1",
								},
							},
							SourcePrefixRanges: []*v3.CidrRange{
								{
									AddressPrefix: "1.2.3.4",
									PrefixLen: &wrappers.UInt32Value{
										Value: 32,
									},
								},
							},
						},
						ListenerType: &v1.MatchedListener_TcpListener{
							TcpListener: tcpListener().GetTcpListener(),
						},
					},
					{
						Matcher: &v1.Matcher{
							SslConfig: &ssl.SslConfig{
								SslSecrets: &ssl.SslConfig_SslFiles{
									SslFiles: &ssl.SSLFiles{
										TlsCert: Certificate(),
										TlsKey:  PrivateKey(),
									},
								},
								SniDomains: []string{
									"sni2",
								},
							},
							SourcePrefixRanges: []*v3.CidrRange{
								{
									AddressPrefix: "5.6.7.8",
									PrefixLen: &wrappers.UInt32Value{
										Value: 32,
									},
								},
							},
						},
						ListenerType: &v1.MatchedListener_HttpListener{
							HttpListener: HttpListener(numRoutes).GetHttpListener(),
						},
					},
				},
			},
		},
	}
}

// Proxy returns a generic proxy that can be used for translation benchmarking
// Proxy invokes functions that contain assertions and therefore can only be invoked from within a test block
func Proxy(numRoutes int) *v1.Proxy {
	return &v1.Proxy{
		Metadata: &core.Metadata{
			Name:      "test",
			Namespace: defaults.GlooSystem,
		},
		Listeners: []*v1.Listener{
			HttpListener(numRoutes),
			tcpListener(),
			hybridListener(numRoutes),
		},
	}
}
