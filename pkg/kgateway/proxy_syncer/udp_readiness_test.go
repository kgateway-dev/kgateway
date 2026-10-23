package proxy_syncer

import (
	"path/filepath"
	"testing"
	"time"

	xdscorev3 "github.com/cncf/xds/go/xds/core/v3"
	xdsmatcherv3 "github.com/cncf/xds/go/xds/type/matcher/v3"
	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	udpproxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/udp/udp_proxy/v3"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

// UDPRoute targets gate readiness like TCPRoute ones. udp_proxy names its
// cluster from a listener filter, not a filter chain.
func TestCollectReferencedClustersReadsUdpProxy(t *testing.T) {
	for golden, target := range map[string]string{
		"udp-routing/basic-proxy.yaml":         "kube_default_example-udp-svc_8080",
		"udp-routing/multi-backend-proxy.yaml": ir.UdpAggregateClusterName("default", "example-udp-route"),
	} {
		result, err := readGoldenTranslation(filepath.Join(goldenCorpusDir, golden))
		require.NoError(t, err, golden)
		routes, listeners := goldenResources(result)
		assert.Equal(t, map[string]struct{}{target: {}}, collectReferencedClusters(routes, listeners), golden)
	}
}

func TestCollectReferencedClustersReadsEveryUdpMatcherAction(t *testing.T) {
	route := func(cluster string) *xdsmatcherv3.Matcher_OnMatch {
		return &xdsmatcherv3.Matcher_OnMatch{OnMatch: &xdsmatcherv3.Matcher_OnMatch_Action{
			Action: &xdscorev3.TypedExtensionConfig{Name: "route", TypedConfig: utils.MustMessageToAny(&udpproxyv3.Route{Cluster: cluster})},
		}}
	}
	nested := &xdsmatcherv3.Matcher{
		MatcherType: &xdsmatcherv3.Matcher_MatcherTree_{MatcherTree: &xdsmatcherv3.Matcher_MatcherTree{
			TreeType: &xdsmatcherv3.Matcher_MatcherTree_ExactMatchMap{ExactMatchMap: &xdsmatcherv3.Matcher_MatcherTree_MatchMap{
				Map: map[string]*xdsmatcherv3.Matcher_OnMatch{"10.0.0.1": route("exact")},
			}},
		}},
		OnNoMatch: route("nested-default"),
	}
	matcher := &xdsmatcherv3.Matcher{
		MatcherType: &xdsmatcherv3.Matcher_MatcherList_{MatcherList: &xdsmatcherv3.Matcher_MatcherList{
			Matchers: []*xdsmatcherv3.Matcher_MatcherList_FieldMatcher{{OnMatch: route("listed")}},
		}},
		OnNoMatch: &xdsmatcherv3.Matcher_OnMatch{OnMatch: &xdsmatcherv3.Matcher_OnMatch_Matcher{Matcher: nested}},
	}
	listeners := sliceToResources([]*envoylistenerv3.Listener{udpProxyListener(matcher)})

	assert.Equal(t, map[string]struct{}{"listed": {}, "exact": {}, "nested-default": {}},
		collectReferencedClusters(envoycache.Resources{}, listeners))
}

// A client whose CDS has a UDP aggregate cluster before the aggregate's CLA
// row lands waits for that row: the route's target gates the snapshot, and the
// placeholder is settled instead of published empty over held endpoints.
func TestUdpAggregatePlaceholderWaitsForItsRow(t *testing.T) {
	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{ucc})
	aggregate := ir.UdpAggregateClusterName("ns", "route")

	listeners := sliceToResources([]*envoylistenerv3.Listener{udpProxyListener(&xdsmatcherv3.Matcher{
		OnNoMatch: &xdsmatcherv3.Matcher_OnMatch{OnMatch: &xdsmatcherv3.Matcher_OnMatch_Action{
			Action: &xdscorev3.TypedExtensionConfig{Name: "route", TypedConfig: utils.MustMessageToAny(&udpproxyv3.Route{Cluster: aggregate})},
		}},
	})})
	gateway := GatewayXdsResources{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: "gw"},
		Listeners:      listeners,
		Clusters: []envoycachetypes.ResourceWithTTL{{Resource: &envoyclusterv3.Cluster{
			Name:                 aggregate,
			ClusterDiscoveryType: &envoyclusterv3.Cluster_Type{Type: envoyclusterv3.Cluster_EDS},
		}}},
		ReferencedClusters: collectReferencedClusters(envoycache.Resources{}, listeners),
	}
	gateways := krt.NewStaticCollection(nil, []GatewayXdsResources{gateway})
	// The aggregate's rows follow a gateway snapshot that has not caught up.
	lagging := krt.NewStaticCollection(nil, []GatewayXdsResources{{NamespacedName: gateway.NamespacedName}})
	backendEndpoints := krt.NewStaticCollection(nil, []ir.EndpointsForBackend{
		{UpstreamResourceName: "be1", LbEps: ir.LocalityLbMap{ir.PodLocality{}: {udpTestEndpoint("10.0.0.1")}}},
	})
	aggregates := krt.NewStaticCollection(nil, []udpAggregate{
		{clusterName: aggregate, members: []udpAggregateMember{{backendResourceName: "be1", weight: 1}}},
	})
	noRows := krt.NewStaticCollection[UccWithEndpoints](nil, nil)
	backendRows := PerClientEnvoyEndpoints{
		endpoints: noRows,
		index:     krtpkg.UnnamedIndex(noRows, func(ep UccWithEndpoints) []string { return []string{ep.Client.ResourceName()} }),
		hasSource: backendEndpointsSource(krt.NewStaticCollection[ir.EndpointsForBackend](nil, nil)),
	}
	snapshots := snapshotPerClient(krtutil.KrtOptions{}, uccs, gateways, backendRows,
		newTestPerClientClustersFromCol(krt.NewStaticCollection[uccWithCluster](nil, nil), uccs), clusterScoping{},
		NewPerClientUdpAggregateEndpoints(krtutil.KrtOptions{}, uccs, aggregates, lagging, backendEndpoints))

	require.Eventually(t, func() bool { return len(snapshots.List()) == 1 }, 2*time.Second, 5*time.Millisecond)
	wrap := snapshots.List()[0]
	assert.Equal(t, []string{aggregate}, wrap.missingEndpointsReferenced, "the route's aggregate gates the snapshot")
	assert.Equal(t, []string{aggregate}, wrap.synthesizedEndpoints, "the aggregate's row is on its way")

	lagging.UpdateObject(gateway)
	require.Eventually(t, func() bool {
		wraps := snapshots.List()
		return len(wraps) == 1 && len(wraps[0].missingEndpointsReferenced) == 0 && len(wraps[0].synthesizedEndpoints) == 0
	}, 2*time.Second, 5*time.Millisecond, "the derived CLA completes the snapshot")
}

func udpProxyListener(matcher *xdsmatcherv3.Matcher) *envoylistenerv3.Listener {
	return &envoylistenerv3.Listener{
		Name: "udp",
		ListenerFilters: []*envoylistenerv3.ListenerFilter{{
			Name: "envoy.filters.udp_listener.udp_proxy",
			ConfigType: &envoylistenerv3.ListenerFilter_TypedConfig{
				TypedConfig: utils.MustMessageToAny(&udpproxyv3.UdpProxyConfig{
					StatPrefix:     "udp",
					RouteSpecifier: &udpproxyv3.UdpProxyConfig_Matcher{Matcher: matcher},
				}),
			},
		}},
	}
}
