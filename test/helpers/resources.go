package helpers

import (
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	"github.com/golang/protobuf/ptypes/wrappers"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

// scaledSnapshotBuilder enumerates the number of each type of resource that should be included in a snapshot
// Additional fields should be added as needed
type scaledSnapshotBuilder struct {
	endpoints int
	upstreams int

	eb *endpointBuilder
	ub *upstreamBuilder
}

type upstreamBuilder struct {
	sni string
}

type endpointBuilder struct{}

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
		routes[i] = route(i + 1) // names are 1-indexed
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
											Name:      upMeta(1).GetName(),
											Namespace: upMeta(1).GetNamespace(),
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

// Build generates a snapshot populated with particular numbers of each resource types as determined by the
// scaledSnapshotBuilder
func (b *scaledSnapshotBuilder) Build() *gloosnapshot.ApiSnapshot {
	endpointList := make(v1.EndpointList, b.endpoints)
	for i := 0; i < b.endpoints; i++ {
		endpointList[i] = b.eb.Build(i + 1) // names are 1-indexed
	}

	upstreamList := make(v1.UpstreamList, b.upstreams)
	for i := 0; i < b.upstreams; i++ {
		upstreamList[i] = b.ub.Build(i + 1) // names are 1-indexed
	}

	return &gloosnapshot.ApiSnapshot{
		// The proxy should contain a route for each upstream
		Proxies: []*v1.Proxy{Proxy(b.upstreams)},

		Endpoints: endpointList,
		Upstreams: upstreamList,
	}
}

func NewScaledSnapshotBuilder() *scaledSnapshotBuilder {
	return &scaledSnapshotBuilder{
		eb: NewEndpointBuilder(),
		ub: NewUpstreamBuilder(),
	}
}

func (b *scaledSnapshotBuilder) WithUpstreams(n int) *scaledSnapshotBuilder {
	b.upstreams = n
	return b
}

func (b *scaledSnapshotBuilder) WithUpstreamBuilder(ub *upstreamBuilder) *scaledSnapshotBuilder {
	b.ub = ub
	return b
}

func (b *scaledSnapshotBuilder) WithEndpoints(n int) *scaledSnapshotBuilder {
	b.endpoints = n
	return b
}

func (b *scaledSnapshotBuilder) WithEndpointBuilder(eb *endpointBuilder) *scaledSnapshotBuilder {
	b.eb = eb
	return b
}

func NewEndpointBuilder() *endpointBuilder {
	return &endpointBuilder{}
}

func (b *endpointBuilder) Build(i int) *v1.Endpoint {
	return Endpoint(i)
}

func NewUpstreamBuilder() *upstreamBuilder {
	return &upstreamBuilder{}
}

func (b *upstreamBuilder) WithUniqueSni() *upstreamBuilder {
	b.sni = "unique"
	return b
}

func (b *upstreamBuilder) WithConsistentSni() *upstreamBuilder {
	b.sni = "consistent"
	return b
}

func (b *upstreamBuilder) Build(i int) *v1.Upstream {
	up := Upstream(i)

	switch b.sni {
	case "unique":
		up.SslConfig = &ssl.UpstreamSslConfig{
			Sni: fmt.Sprintf("unique-domain-%d", i),
		}
	case "consistent":
		up.SslConfig = &ssl.UpstreamSslConfig{
			Sni: "consistent-domain",
		}
	}

	return up
}