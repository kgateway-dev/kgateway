package proxy_syncer

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

// collectSeries reads every series a metric vector currently exports, keyed by
// its label set, without creating any. Counters and gauges both land here.
func collectSeries(t *testing.T, m any) map[string]float64 {
	t.Helper()
	collector := metrics.GetPromCollector(m)
	require.NotNil(t, collector)
	ch := make(chan prometheus.Metric, 64)
	go func() {
		collector.Collect(ch)
		close(ch)
	}()
	out := make(map[string]float64)
	for pm := range ch {
		var d dto.Metric
		require.NoError(t, pm.Write(&d))
		var key strings.Builder
		for _, lp := range d.GetLabel() {
			key.WriteString(lp.GetName())
			key.WriteString("=")
			key.WriteString(lp.GetValue())
			key.WriteString(",")
		}
		switch {
		case d.GetGauge() != nil:
			out[key.String()] = d.GetGauge().GetValue()
		case d.GetCounter() != nil:
			out[key.String()] = d.GetCounter().GetValue()
		}
	}
	return out
}

func deferredClientsKey(gateway, namespace string) string {
	return "gateway=" + gateway + ",namespace=" + namespace + ","
}

func eventuallyDeferredClients(t *testing.T, gateway, namespace string, want float64) {
	t.Helper()
	key := deferredClientsKey(gateway, namespace)
	var got map[string]float64
	require.Eventually(t, func() bool {
		got = collectSeries(t, snapshotDeferredClients)
		v, ok := got[key]
		return ok && v == want
	}, 2*time.Second, 10*time.Millisecond, "deferred clients for %s/%s never reached %v; series: %v", namespace, gateway, want, got)
}

// The deferred-clients gauge is derived from collection state: a connected
// client counts while it has no cluster row. That is what lets it show a client
// that is stuck, which a counter cannot (the transform runs only on events).
// It must also follow clients out: a deferred client that disconnects stops
// counting, and a gateway's last client leaving removes the series rather than
// leaving a zero behind.
func TestDeferredClientsTracker_FollowsCollectionState(t *testing.T) {
	ResetMetrics()
	const gateway, namespace = "gw-tracker", "ns"
	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, namespace, gateway)
	a := ir.NewUniquelyConnectedClient(role, namespace, map[string]string{"pod": "a"}, ir.PodLocality{})
	b := ir.NewUniquelyConnectedClient(role, namespace, map[string]string{"pod": "b"}, ir.PodLocality{})
	require.NotEqual(t, a.ResourceName(), b.ResourceName(), "fixture needs two distinct clients of one gateway")

	clients := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{a, b})
	rows := krt.NewStaticCollection(nil, []clustersWithErrors{{resourceName: a.ResourceName()}})
	trackDeferredClients(clients, rows)

	// b is connected with no cluster row.
	eventuallyDeferredClients(t, gateway, namespace, 1)

	// b's row lands: healthy gateway reads zero, not absent.
	rows.UpdateObject(clustersWithErrors{resourceName: b.ResourceName()})
	eventuallyDeferredClients(t, gateway, namespace, 0)

	// a's row is withdrawn (a fence failed for it).
	rows.DeleteObject(a.ResourceName())
	eventuallyDeferredClients(t, gateway, namespace, 1)

	// a disconnects while deferred: it must stop counting with no delete hook.
	clients.DeleteObject(a.ResourceName())
	eventuallyDeferredClients(t, gateway, namespace, 0)

	// The gateway's last client leaves: the series goes away.
	clients.DeleteObject(b.ResourceName())
	require.Eventually(t, func() bool {
		_, ok := collectSeries(t, snapshotDeferredClients)[deferredClientsKey(gateway, namespace)]
		return !ok
	}, 2*time.Second, 10*time.Millisecond, "series for a gateway with no clients must be deleted")
}

// A per-client cluster transform that returns nothing increments the deferral
// counter under the fence that failed, and the client counts as deferred until
// the collections converge and its snapshot publishes.
func TestSnapshotPerClient_CountsClusterDeferralsByReason(t *testing.T) {
	ResetMetrics()
	const gateway, namespace = "gw-deferral", "ns"
	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, namespace, gateway)
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection(nil, []ir.UniquelyConnectedClient{ucc})
	mostXdsSnapshots := krt.NewStaticCollection(nil, []GatewayXdsResources{{
		NamespacedName: types.NamespacedName{Namespace: namespace, Name: gateway},
	}})

	// The delta set was evaluated before this client connected: the fence a
	// connecting client hits once per backend.
	baseRow := baseEnvoyCluster{
		Name: "c", Cluster: sharedproto.Wrap(clusterNamed("c")), ClusterVersion: 1, Fingerprint: fingerprintBase(1),
	}
	unresolved := newClientInputSnapshot(nil)
	baseCol := krt.NewStaticCollection(nil, []baseEnvoyCluster{baseRow})
	deltaCol := krt.NewStaticCollection(nil, []backendClusterDeltaSet{{
		Name:               "c",
		BaseFingerprint:    baseRow.Fingerprint,
		ClientsFingerprint: unresolved.Fingerprint,
		ResolvedClients:    unresolved,
	}})
	pcc := PerClientEnvoyClusters{base: baseCol, deltas: deltaCol}
	endpointCol := krt.NewStaticCollection[UccWithEndpoints](nil, nil)

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
		pcc,
	)

	// Wait for the transform itself: the gauge alone reads 1 from the moment the
	// client exists with no row, which is before the transform has run at all.
	want := "gateway=" + gateway + ",namespace=" + namespace + ",reason=" + string(deferralUnresolvedClient) + ","
	var counted map[string]float64
	require.Eventually(t, func() bool {
		counted = collectSeries(t, snapshotClusterDeferralsTotal)
		return counted[want] >= 1
	}, 2*time.Second, 10*time.Millisecond, "deferral must be counted under its reason; series: %v", counted)
	for key := range counted {
		if key != want {
			require.NotContains(t, key, "gateway="+gateway+",", "no other reason may be counted for this client; series: %v", counted)
		}
	}
	eventuallyDeferredClients(t, gateway, namespace, 1)
	require.Empty(t, snapshots.List(), "an unresolved client must not have a snapshot")

	// The delta set catches up with the connected client: the snapshot
	// publishes and the client stops counting as deferred.
	resolved := newClientInputSnapshot([]ir.UniquelyConnectedClient{ucc})
	deltaCol.UpdateObject(backendClusterDeltaSet{
		Name:               "c",
		BaseFingerprint:    baseRow.Fingerprint,
		ClientsFingerprint: resolved.Fingerprint,
		ResolvedClients:    resolved,
	})
	require.Eventually(t, func() bool { return len(snapshots.List()) == 1 }, 2*time.Second, 10*time.Millisecond)
	eventuallyDeferredClients(t, gateway, namespace, 0)
}
