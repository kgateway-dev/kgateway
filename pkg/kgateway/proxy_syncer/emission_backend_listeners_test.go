package proxy_syncer

import (
	"slices"
	"strconv"
	"testing"
	"time"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoysniclusterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/sni_cluster/v3"
	envoytcpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
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
// among them keeps everything and is reported, even when nothing was dropped.
func TestKeepBackendListenerTargets(t *testing.T) {
	for _, tc := range []struct {
		name       string
		translated []string
		kept       []string
		listeners  map[string]baseResources[*envoylistenerv3.Listener]
		want       []string
		filtered   bool
		fallback   []string
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
			fallback: []string{`backend "tunneled" listener "tunnel_tunneled": sni_cluster`},
		},
		{
			name:       "a kept target's selector keeps every cluster",
			translated: []string{"tunneled", "proxy", "upstream-proxy", "unreferenced"},
			kept:       []string{"tunneled"},
			listeners: map[string]baseResources[*envoylistenerv3.Listener]{
				"tunneled": backendListenersOf(tunnelListener("tunnel_tunneled", "proxy")),
				"proxy":    backendListenersOf(tunnelListener("tunnel_proxy", "upstream-proxy", &envoysniclusterv3.SniCluster{})),
			},
			want:     []string{"tunneled", "proxy", "upstream-proxy", "unreferenced"},
			filtered: false,
			fallback: []string{`backend "proxy" listener "tunnel_proxy": sni_cluster`},
		},
		{
			name:       "a dropped backend's selector changes nothing",
			translated: []string{"routed", "tunneled", "proxy"},
			kept:       []string{"routed"},
			listeners: map[string]baseResources[*envoylistenerv3.Listener]{
				"tunneled": backendListenersOf(tunnelListener("tunnel_tunneled", "proxy", &envoysniclusterv3.SniCluster{})),
			},
			want:     []string{"routed"},
			filtered: true,
		},
		{
			name:       "a selector is reported when every cluster is already kept",
			translated: []string{"tunneled", "proxy"},
			kept:       []string{"tunneled", "proxy"},
			listeners: map[string]baseResources[*envoylistenerv3.Listener]{
				"tunneled": backendListenersOf(tunnelListener("tunnel_tunneled", "proxy", &envoysniclusterv3.SniCluster{})),
			},
			want:     []string{"tunneled", "proxy"},
			filtered: false,
			fallback: []string{`backend "tunneled" listener "tunnel_tunneled": sni_cluster`},
		},
		{
			name:       "everything kept with no selector is not a fallback",
			translated: []string{"tunneled", "proxy"},
			kept:       []string{"tunneled", "proxy"},
			listeners: map[string]baseResources[*envoylistenerv3.Listener]{
				"tunneled": backendListenersOf(tunnelListener("tunnel_tunneled", "proxy")),
			},
			want:     []string{"tunneled", "proxy"},
			filtered: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			translated, translatedVersions := clusterResourcesFor(tc.translated...)
			kept, keptVersions, _ := filterClustersToEmitted(
				scopedClusters(), emissionSet(tc.kept...), translated, translatedVersions)

			got, gotVersions, filtered, fallback := keepBackendListenerTargets(
				translated, translatedVersions, kept, keptVersions, tc.listeners)

			assert.Equal(t, tc.filtered, filtered)
			assert.Equal(t, tc.fallback, fallback)
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
// tunnel keeps the proxy cluster its listener names. Its EDS assignments, LDS
// listeners and SDS secrets follow CDS. An unrouted tunnel's listener and
// secret do not ship.
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

			wrap := tunnelSnapshot(t, "gw-keeps-proxy", tc.scoping, []string{"tunneled"})
			g.Expect(mapKeys(wrap.snap.Resources[envoycachetypes.Cluster].Items)).To(gomega.ConsistOf(toAny(tc.expected)...))
			g.Expect(mapKeys(wrap.snap.Resources[envoycachetypes.Endpoint].Items)).To(gomega.ConsistOf(toAny(tc.expected)...),
				"EDS must follow CDS")
			g.Expect(mapKeys(wrap.snap.Resources[envoycachetypes.Listener].Items)).To(gomega.ConsistOf(toAny(tc.listeners)...),
				"backend-owned LDS listeners ship with their cluster, and only then")
			g.Expect(mapKeys(wrap.snap.Resources[envoycachetypes.Secret].Items)).To(gomega.ConsistOf(toAny(tc.secrets)...),
				"backend-owned SDS secrets ship with their cluster, and only then")
		})
	}
}

// TestSnapshotPerClientReportsABackendListenerFallback: a selector in a kept
// backend's listener reverts the client to every cluster and reports the
// gateway unscoped. Without one, the gateway reads scoped, including when its
// routes already name every cluster.
func TestSnapshotPerClientReportsABackendListenerFallback(t *testing.T) {
	metrics.SetRegistry(false, nil)
	metrics.SetActive(true)
	for _, tc := range []struct {
		name     string
		routed   []string
		filters  []proto.Message
		expected []string
		dropped  float64
		disabled float64
	}{
		{
			name:     "a selector in the tunnel's listener",
			routed:   []string{"tunneled"},
			filters:  []proto.Message{&envoysniclusterv3.SniCluster{}},
			expected: []string{"tunneled", "proxy", "unreferenced"},
			dropped:  0,
			disabled: 1,
		},
		{
			name:     "a selector when the routes already name every cluster",
			routed:   []string{"tunneled", "proxy", "unreferenced"},
			filters:  []proto.Message{&envoysniclusterv3.SniCluster{}},
			expected: []string{"tunneled", "proxy", "unreferenced"},
			dropped:  0,
			disabled: 1,
		},
		{
			name:     "no selector",
			routed:   []string{"tunneled"},
			expected: []string{"tunneled", "proxy"},
			dropped:  1,
			disabled: 0,
		},
		{
			name:     "no selector, every cluster named",
			routed:   []string{"tunneled", "proxy", "unreferenced"},
			expected: []string{"tunneled", "proxy", "unreferenced"},
			dropped:  0,
			disabled: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway := "gw-fallback-" + strconv.Itoa(len(tc.routed)) + "-" + strconv.Itoa(len(tc.filters))
			wrap := tunnelSnapshot(t, gateway, scopedClusters(), tc.routed, tc.filters...)
			assert.ElementsMatch(t, tc.expected, mapKeys(wrap.snap.Resources[envoycachetypes.Cluster].Items))

			labels := []metrics.Label{
				{Name: gatewayLabel, Value: gateway},
				{Name: namespaceLabel, Value: "ns"},
			}
			gathered := metricstest.MustGatherMetrics(t)
			assert.Equal(t, tc.dropped, gathered.MustGetMetricValueByLabels("kgateway_xds_cluster_scoping_dropped_clusters", labels))
			assert.Equal(t, tc.disabled, gathered.MustGetMetricValueByLabels("kgateway_xds_cluster_scoping_disabled_gateways", labels))
		})
	}
}

// TestClaimsDoNotCoverABackendListenersSelector: a claim on a cluster
// specifier plugin accounts for the gateway's routes' uses of it, not for one
// inside a kept backend's own listener. That one still reverts the client to
// every cluster and reports the gateway unscoped.
func TestClaimsDoNotCoverABackendListenersSelector(t *testing.T) {
	metrics.SetRegistry(false, nil)
	metrics.SetActive(true)
	scoping := scopedWithClaims(collectEmissionClaims(claimingPlugins(sdk.ClusterEmissionClaim{
		Selectors: []string{"picker"},
		Names:     []string{"picker-fallback"},
	})))
	picker := &envoyhcmv3.HttpConnectionManager{
		StatPrefix: "tunnel",
		RouteSpecifier: &envoyhcmv3.HttpConnectionManager_RouteConfig{RouteConfig: &envoyroutev3.RouteConfiguration{
			VirtualHosts: []*envoyroutev3.VirtualHost{{Name: "vh", Domains: []string{"*"}, Routes: []*envoyroutev3.Route{{
				Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{
					ClusterSpecifier: &envoyroutev3.RouteAction_ClusterSpecifierPlugin{ClusterSpecifierPlugin: "picker"},
				}},
			}}}},
		}},
	}

	const gateway = "gw-claimed-backend-selector"
	wrap := tunnelSnapshot(t, gateway, scoping, []string{"tunneled"}, picker)

	assert.ElementsMatch(t, []string{"tunneled", "proxy", "unreferenced"}, mapKeys(wrap.snap.Resources[envoycachetypes.Cluster].Items))
	assert.Equal(t, float64(1), metricstest.MustGatherMetrics(t).MustGetMetricValueByLabels(
		"kgateway_xds_cluster_scoping_disabled_gateways", []metrics.Label{
			{Name: gatewayLabel, Value: gateway},
			{Name: namespaceLabel, Value: "ns"},
		}))
}

// tunnelSnapshot builds one client's snapshot of gateway ns/gateway, whose
// routes name routed, over three backends: "tunneled", whose listener is a
// tunnel through "proxy" behind filters, "proxy", and "unreferenced".
func tunnelSnapshot(t *testing.T, gateway string, scoping clusterScoping, routed []string, filters ...proto.Message) XdsSnapWrapper {
	t.Helper()
	return eventuallyCoherentWrapper(t, tunnelSnapshots(t, gateway, scoping, routed, nil, filters...))
}

// tunnelSnapshots is tunnelSnapshot's collection; the clusters in
// underived have no endpoint row.
func tunnelSnapshots(t *testing.T, gateway string, scoping clusterScoping, routed, underived []string, filters ...proto.Message) krt.Collection[XdsSnapWrapper] {
	t.Helper()
	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", gateway)
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	t.Cleanup(func() { forgetClusterScopingClient(ucc.ResourceName(), false) })
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc})

	var vhRoutes []*envoyroutev3.Route
	for _, cluster := range routed {
		vhRoutes = append(vhRoutes, &envoyroutev3.Route{
			Name: cluster,
			Action: &envoyroutev3.Route_Route{Route: &envoyroutev3.RouteAction{
				ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: cluster},
			}},
		})
	}
	routes := sliceToResources([]*envoyroutev3.RouteConfiguration{{
		Name:         "route-config",
		VirtualHosts: []*envoyroutev3.VirtualHost{{Name: "vhost", Domains: []string{"*"}, Routes: vhRoutes}},
	}})
	listeners := sliceToResources([]*envoylistenerv3.Listener{httpListenerWithRDS(t, "listener", "route-config")})
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
		NamespacedName:     types.NamespacedName{Namespace: "ns", Name: gateway},
		Routes:             routes,
		Listeners:          listeners,
		ReferencedClusters: collectReferencedClusters(routes, listeners),
		EmittedClusters:    emittedClustersFor(scoping, routes, listeners),
	}})

	tunnel := tunnelListener("tunnel_tunneled", "proxy", filters...)
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
		if slices.Contains(underived, name) {
			continue
		}
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
		scoping,
	)
	return snapshots
}

// The publication gate follows a referenced backend's own listeners: a route to
// a tunneled backend is not ready while the proxy cluster its tunnel listener
// sends to has no endpoints, in either discovery mode.
func TestRouteToATunneledBackendWaitsForItsProxyCluster(t *testing.T) {
	for name, scoping := range map[string]clusterScoping{"all": {}, "referenced": scopedClusters()} {
		t.Run(name, func(t *testing.T) {
			snapshots := tunnelSnapshots(t, "gw-proxy-unready-"+name, scoping, []string{"tunneled"}, []string{"proxy"})
			gomega.NewWithT(t).Eventually(func() []string {
				list := snapshots.List()
				if len(list) != 1 {
					return nil
				}
				return list[0].missingEndpointsReferenced
			}, time.Second, 20*time.Millisecond).Should(gomega.ConsistOf("proxy"),
				"the route to the tunneled backend waits for the proxy's endpoints")
		})
	}
}
