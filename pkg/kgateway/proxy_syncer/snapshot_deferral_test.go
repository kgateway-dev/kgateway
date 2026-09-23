package proxy_syncer

import (
	"context"
	"testing"
	"time"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

const deferredClientsMetric = "kgateway_xds_snapshot_deferred_clients"

// deferredGauge reads the gauge for one gateway. A series that has not been
// written since the last reset reads as 0 rather than failing, so the helper
// can be polled from Eventually before the tracker has published anything.
func deferredGauge(t *testing.T, gateway, namespace string) float64 {
	t.Helper()
	gathered := metricstest.MustGatherMetrics(t)
	if gathered.MetricLength(deferredClientsMetric) == 0 {
		return 0
	}
	return gathered.MustGetMetricValueByLabels(deferredClientsMetric, []metrics.Label{
		{Name: gatewayLabel, Value: gateway},
		{Name: namespaceLabel, Value: namespace},
	})
}

// The tracker must count a client that has never had a snapshot at all. That
// is the case a counter incremented on the deferral branch cannot see: the
// snapshot transform returning nil for a row that never existed emits no event.
func TestSnapshotDeferralTrackerCountsClientsWithoutAnySnapshot(t *testing.T) {
	setupTest()
	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	// Distinct label sets give the two clients distinct keys under one gateway.
	cold := ir.NewUniquelyConnectedClient(role, "ns", map[string]string{"pod": "cold"}, ir.PodLocality{})
	warm := ir.NewUniquelyConnectedClient(role, "ns", map[string]string{"pod": "warm"}, ir.PodLocality{})

	d := newSnapshotDeferralTracker()
	d.clientEvents([]krt.Event[ir.UniquelyConnectedClient]{
		{New: &cold, Event: controllers.EventAdd},
		{New: &warm, Event: controllers.EventAdd},
	})
	require.ElementsMatch(t, []string{cold.ResourceName(), warm.ResourceName()}, d.deferredClients(),
		"a connected client with no snapshot row is deferred")
	require.Equal(t, float64(2), deferredGauge(t, "gw", "ns"))

	// warm gets a snapshot; cold never does.
	warmSnap := XdsSnapWrapper{proxyKey: warm.ResourceName()}
	d.snapshotEvents([]krt.Event[XdsSnapWrapper]{{New: &warmSnap, Event: controllers.EventAdd}})
	require.Equal(t, []string{cold.ResourceName()}, d.deferredClients())
	require.Equal(t, float64(1), deferredGauge(t, "gw", "ns"))

	// warm's transform returns nil (its inputs went away): the row is deleted
	// while the client is still connected, so warm is deferred again.
	d.snapshotEvents([]krt.Event[XdsSnapWrapper]{{Old: &warmSnap, Event: controllers.EventDelete}})
	require.ElementsMatch(t, []string{cold.ResourceName(), warm.ResourceName()}, d.deferredClients())
	require.Equal(t, float64(2), deferredGauge(t, "gw", "ns"))

	// A disconnect is not a deferral: the client leaves the count whether or
	// not a snapshot row for it lingers, and a late row delete is then inert.
	d.clientEvents([]krt.Event[ir.UniquelyConnectedClient]{{Old: &cold, Event: controllers.EventDelete}})
	d.snapshotEvents([]krt.Event[XdsSnapWrapper]{{New: &warmSnap, Event: controllers.EventAdd}})
	d.clientEvents([]krt.Event[ir.UniquelyConnectedClient]{{Old: &warm, Event: controllers.EventDelete}})
	d.snapshotEvents([]krt.Event[XdsSnapWrapper]{{Old: &warmSnap, Event: controllers.EventDelete}})
	require.Empty(t, d.deferredClients())
	require.Equal(t, float64(0), deferredGauge(t, "gw", "ns"))

	// Repeated events for the same key are idempotent.
	d.clientEvents([]krt.Event[ir.UniquelyConnectedClient]{
		{New: &cold, Event: controllers.EventAdd},
		{New: &cold, Event: controllers.EventUpdate},
	})
	require.Equal(t, float64(1), deferredGauge(t, "gw", "ns"))
}

// Through the real snapshot collection: a client whose per-client clusters have
// not landed is counted as deferred, leaves the count once its snapshot
// publishes, and leaves it again when it disconnects.
func TestSnapshotPerClientReportsDeferredClients(t *testing.T) {
	setupTest()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	role := xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, "ns", "gw")
	ucc := ir.NewUniquelyConnectedClient(role, "", nil, ir.PodLocality{})
	uccs := krt.NewStaticCollection[ir.UniquelyConnectedClient](nil, []ir.UniquelyConnectedClient{ucc}, krtopts.ToOptions("UCCs")...)
	mostXdsSnapshots := krt.NewStaticCollection[GatewayXdsResources](nil, []GatewayXdsResources{{
		NamespacedName: types.NamespacedName{Namespace: "ns", Name: "gw"},
		Listeners:      sliceToResources([]*envoylistenerv3.Listener{{Name: "listener"}}),
	}}, krtopts.ToOptions("GatewayXds")...)

	// No base clusters yet: the per-client cluster row does not exist, so the
	// snapshot transform defers.
	bases := krt.NewStaticCollection[baseEnvoyCluster](nil, nil, krtopts.ToOptions("Bases")...)
	pcc := newPerClientEnvoyClusters(ctx, krtopts, nil, bases, uccs)
	endpointCol := krt.NewStaticCollection[UccWithEndpoints](nil, nil, krtopts.ToOptions("Endpoints")...)
	snapshots := snapshotPerClient(
		krtopts,
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

	require.Eventually(t, func() bool { return deferredGauge(t, "gw", "ns") == 1 }, 2*time.Second, 20*time.Millisecond,
		"a connected client with no per-client clusters must be reported as deferred")
	require.Empty(t, snapshots.List(), "and it must have no published snapshot")

	// The base lands, the client's payload is assembled, the snapshot publishes.
	bases.UpdateObject(baseFromCluster(uccWithCluster{
		Client:         ucc,
		Name:           "cluster-a",
		Cluster:        sharedproto.Wrap(&envoyclusterv3.Cluster{Name: "cluster-a"}),
		ClusterVersion: 1,
	}))
	require.Eventually(t, func() bool { return len(snapshots.List()) == 1 }, 2*time.Second, 20*time.Millisecond)
	require.Eventually(t, func() bool { return deferredGauge(t, "gw", "ns") == 0 }, 2*time.Second, 20*time.Millisecond,
		"a client with a published snapshot is not deferred")

	// The base goes away again: the transform returns nil, the row is deleted,
	// Envoy keeps the last snapshot, and the client is deferred once more.
	bases.DeleteObject("cluster-a")
	require.Eventually(t, func() bool { return deferredGauge(t, "gw", "ns") == 1 }, 2*time.Second, 20*time.Millisecond,
		"losing the per-client clusters must show up as a deferral, not silence")

	// Disconnecting clears it: a departed client is not being starved.
	uccs.DeleteObject(ucc.ResourceName())
	require.Eventually(t, func() bool { return deferredGauge(t, "gw", "ns") == 0 }, 2*time.Second, 20*time.Millisecond)
}
