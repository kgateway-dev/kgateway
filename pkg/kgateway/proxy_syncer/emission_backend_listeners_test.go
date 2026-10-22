package proxy_syncer

import (
	"testing"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoysniclusterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/sni_cluster/v3"
	envoytcpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

// tunnelListener is a CONNECT tunnel's listener shape: a TCP proxy to the proxy
// backend's cluster, optionally behind other network filters.
func tunnelListener(name, proxyCluster string, before ...proto.Message) *envoylistenerv3.Listener {
	var filters []*envoylistenerv3.Filter
	for _, msg := range append(before, &envoytcpv3.TcpProxy{
		StatPrefix:       name,
		ClusterSpecifier: &envoytcpv3.TcpProxy_Cluster{Cluster: proxyCluster},
	}) {
		filters = append(filters, &envoylistenerv3.Filter{
			Name:       string(msg.ProtoReflect().Descriptor().FullName()),
			ConfigType: &envoylistenerv3.Filter_TypedConfig{TypedConfig: utils.MustMessageToAny(msg)},
		})
	}
	return &envoylistenerv3.Listener{
		Name:         name,
		FilterChains: []*envoylistenerv3.FilterChain{{Filters: filters}},
	}
}

func backendListenersOf(listeners ...*envoylistenerv3.Listener) baseResources[*envoylistenerv3.Listener] {
	return wrapBaseResources(listeners)
}

// TestKeepBackendListenerTargets: a kept backend's own listeners keep the
// clusters they name, a dropped backend's do not, and a request-time selector
// among them keeps everything.
func TestKeepBackendListenerTargets(t *testing.T) {
	for _, tc := range []struct {
		name       string
		translated []string
		kept       []string
		listeners  map[string]baseResources[*envoylistenerv3.Listener]
		want       []string
		filtered   bool
	}{
		{
			name:       "a kept tunnel keeps its proxy",
			translated: []string{"tunneled", "proxy", "unreferenced"},
			kept:       []string{"tunneled"},
			listeners: map[string]baseResources[*envoylistenerv3.Listener]{
				"tunneled": backendListenersOf(tunnelListener("tunnel_tunneled", "proxy")),
			},
			want:     []string{"tunneled", "proxy"},
			filtered: true,
		},
		{
			name:       "a dropped tunnel keeps nothing",
			translated: []string{"routed", "tunneled", "proxy"},
			kept:       []string{"routed"},
			listeners: map[string]baseResources[*envoylistenerv3.Listener]{
				"tunneled": backendListenersOf(tunnelListener("tunnel_tunneled", "proxy")),
			},
			want:     []string{"routed"},
			filtered: true,
		},
		{
			name:       "a kept target's own listeners are followed",
			translated: []string{"tunneled", "proxy", "upstream-proxy", "unreferenced"},
			kept:       []string{"tunneled"},
			listeners: map[string]baseResources[*envoylistenerv3.Listener]{
				"tunneled": backendListenersOf(tunnelListener("tunnel_tunneled", "proxy")),
				"proxy":    backendListenersOf(tunnelListener("tunnel_proxy", "upstream-proxy")),
			},
			want:     []string{"tunneled", "proxy", "upstream-proxy"},
			filtered: true,
		},
		{
			name:       "a target that is not translated is skipped",
			translated: []string{"tunneled", "unreferenced"},
			kept:       []string{"tunneled"},
			listeners: map[string]baseResources[*envoylistenerv3.Listener]{
				"tunneled": backendListenersOf(tunnelListener("tunnel_tunneled", "proxy")),
			},
			want:     []string{"tunneled"},
			filtered: true,
		},
		{
			name:       "keeping the targets can leave nothing dropped",
			translated: []string{"tunneled", "proxy"},
			kept:       []string{"tunneled"},
			listeners: map[string]baseResources[*envoylistenerv3.Listener]{
				"tunneled": backendListenersOf(tunnelListener("tunnel_tunneled", "proxy")),
			},
			want:     []string{"tunneled", "proxy"},
			filtered: false,
		},
		{
			name:       "a request-time selector keeps every cluster",
			translated: []string{"tunneled", "proxy", "unreferenced"},
			kept:       []string{"tunneled"},
			listeners: map[string]baseResources[*envoylistenerv3.Listener]{
				"tunneled": backendListenersOf(tunnelListener("tunnel_tunneled", "proxy", &envoysniclusterv3.SniCluster{})),
			},
			want:     []string{"tunneled", "proxy", "unreferenced"},
			filtered: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			translated, translatedVersions := clusterResourcesFor(tc.translated...)
			kept, keptVersions, filtered := filterClustersToEmitted(
				scopedClusters(), emissionSet(tc.kept...), translated, translatedVersions)
			require.True(t, filtered, "the fixture must drop something before the targets are kept")

			got, gotVersions, filtered := keepBackendListenerTargets(
				translated, translatedVersions, kept, keptVersions, tc.listeners)

			assert.Equal(t, tc.filtered, filtered)
			assert.ElementsMatch(t, tc.want, resourceNames(got))
			wantVersions := make(map[string]uint64, len(tc.want))
			for _, name := range tc.want {
				wantVersions[name] = translatedVersions[name]
			}
			assert.Equal(t, wantVersions, gotVersions, "versions must cover exactly the emitted clusters")
		})
	}
}

// TestSnapshotPerClientKeepsClustersBackendListenersName: scoped, a routed
// tunnel keeps the proxy cluster its listener names, the listener ships in LDS,
// and EDS follows CDS. An unrouted tunnel's listener and secret do not ship.
func TestSnapshotPerClientKeepsClustersBackendListenersName(t *testing.T) {
	for _, tc := range []struct {
		name      string
		scoping   clusterScoping
		expected  []string
		listeners []string
		secrets   []string
	}{
		{
			name:      "all emits every translated backend",
			scoping:   clusterScoping{},
			expected:  []string{"tunneled", "proxy", "unreferenced"},
			listeners: []string{"listener", "tunnel_tunneled", "tunnel_unreferenced"},
			secrets:   []string{"tunneled-header", "unreferenced-header"},
		},
		{
			name:      "referenced keeps the tunnel's proxy",
			scoping:   scopedClusters(),
			expected:  []string{"tunneled", "proxy"},
			listeners: []string{"listener", "tunnel_tunneled"},
			secrets:   []string{"tunneled-header"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
			ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
			uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})

			routes := sliceToResources([]*envoyroutev3.RouteConfiguration{{
				Name: "route-config",
				VirtualHosts: []*envoyroutev3.VirtualHost{{
					Name:    "vhost",
					Domains: []string{"*"},
					Routes: []*envoyroutev3.Route{{
						Name: "tunneled",
						Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{
							ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: "tunneled"},
						}},
					}},
				}},
			}})
			listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
			mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
				NamespacedName:     types.NamespacedName{Namespace: "ns", Name: "gw"},
				Routes:             routes,
				Listeners:          listeners,
				ReferencedClusters: collectReferencedClusters(routes, listeners),
				EmittedClusters:    collectReferencedClustersForEmission(routes, listeners),
			}})

			tunnel := tunnelListener("tunnel_tunneled", "proxy")
			clusterRows := make([]uccWithCluster, 0, 3)
			endpointRows := make([]UccWithEndpoints, 0, 3)
			var version uint64
			for _, name := range []string{"tunneled", "proxy", "unreferenced"} {
				version++
				row := uccWithCluster{
					Client:         ucc,
					Name:           name,
					Cluster:        sharedproto.Wrap(edsClusterProto(name)),
					ClusterVersion: version,
				}
				switch name {
				case "tunneled":
					row.Listeners = backendListenersOf(tunnel)
					row.Secrets = wrapBaseResources([]*envoytlsv3.Secret{{Name: "tunneled-header"}})
				case "unreferenced":
					row.Listeners = backendListenersOf(tunnelListener("tunnel_unreferenced", "proxy"))
					row.Secrets = wrapBaseResources([]*envoytlsv3.Secret{{Name: "unreferenced-header"}})
				}
				clusterRows = append(clusterRows, row)
				endpointRows = append(endpointRows, UccWithEndpoints{
					Client:        ucc,
					endpointsName: name,
					resourceName:  uccEndpointsResourceName(ucc, name),
					Endpoints:     sharedproto.Wrap(&envoyendpointv3.ClusterLoadAssignment{ClusterName: name}),
					EndpointsHash: version,
				})
			}
			clusterCol := krt.NewStaticCollection[uccWithCluster](nil, clusterRows)
			endpointCol := krt.NewStaticCollection[UccWithEndpoints](nil, endpointRows)

			snapshots := snapshotPerClient(
				krtutil.KrtOptions{},
				uccs,
				mostXdsSnapshots,
				PerClientEnvoyEndpoints{
					endpoints: endpointCol,
					index: krtpkg.UnnamedIndex(endpointCol, func(ep UccWithEndpoints) []string {
						return []string{ep.Client.ResourceName()}
					}),
				},
				newTestPerClientClustersFromCol(clusterCol, uccs),
				tc.scoping,
			)

			wrap := eventuallyCoherentWrapper(t, snapshots)
			g.Expect(mapKeys(wrap.snap.Resources[envoycachetypes.Cluster].Items)).To(gomega.ConsistOf(toAny(tc.expected)...))
			g.Expect(mapKeys(wrap.snap.Resources[envoycachetypes.Endpoint].Items)).To(gomega.ConsistOf(toAny(tc.expected)...),
				"EDS must follow CDS")
			g.Expect(mapKeys(wrap.snap.Resources[envoycachetypes.Listener].Items)).To(gomega.ConsistOf(toAny(tc.listeners)...),
				"a backend's listeners ship with its cluster, and only then")
			g.Expect(mapKeys(wrap.snap.Resources[envoycachetypes.Secret].Items)).To(gomega.ConsistOf(toAny(tc.secrets)...),
				"a backend's secrets ship with its cluster, and only then")
		})
	}
}
