package proxy_syncer

import (
	"strings"
	"sync"
	"time"

	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
)

const (
	snapshotSubsystem = "xds_snapshot"
	gatewayLabel      = "gateway"
	nameLabel         = "name"
	namespaceLabel    = "namespace"
	reasonLabel       = "reason"
	resultLabel       = "result"
	resourceLabel     = "resource"
)

var (
	transformsHistogramBuckets = []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}
	snapshotTransformsTotal    = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: snapshotSubsystem,
			Name:      "transforms_total",
			Help:      "Total number of XDS snapshot transforms",
		},
		[]string{gatewayLabel, namespaceLabel, resultLabel},
	)
	snapshotTransformDuration = metrics.NewHistogram(
		metrics.HistogramOpts{
			Subsystem:                       snapshotSubsystem,
			Name:                            "transform_duration_seconds",
			Help:                            "XDS snapshot transform duration",
			Buckets:                         transformsHistogramBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		},
		[]string{gatewayLabel, namespaceLabel},
	)
	snapshotResources = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: snapshotSubsystem,
			Name:      "resources",
			Help:      "Current number of resources in XDS snapshot",
		},
		[]string{gatewayLabel, namespaceLabel, resourceLabel},
	)
	// snapshotClusterDeferralsTotal counts per-client cluster transforms that
	// returned nothing, by the fence that failed (see clusterDeferral). A steady
	// trickle is normal: one per connected client per base change. It cannot
	// show a client that is stuck, because a transform only runs on events; that
	// is what snapshotDeferredClients is for.
	snapshotClusterDeferralsTotal = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: snapshotSubsystem,
			Name:      "cluster_deferrals_total",
			Help:      "Total number of per-client cluster snapshot transforms that published nothing because the cluster collections had not converged, by reason",
		},
		[]string{gatewayLabel, namespaceLabel, reasonLabel},
	)
	// snapshotDeferredClients is the number of connected clients of a gateway
	// whose cluster snapshot is currently withheld. It is zero in steady state
	// and briefly positive while collections converge; a value that stays
	// positive is a client that is not receiving CDS updates, and the thing to
	// alert on.
	snapshotDeferredClients = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: snapshotSubsystem,
			Name:      "deferred_clients",
			Help:      "Number of connected xDS clients whose cluster snapshot is currently withheld pending cluster collection convergence",
		},
		[]string{gatewayLabel, namespaceLabel},
	)
)

// recordClusterDeferral counts one per-client cluster transform that returned
// nothing, attributed to the client's gateway and the fence that failed.
func recordClusterDeferral(clientKey string, reason clusterDeferral) {
	if !metrics.Active() {
		return
	}
	cd := getDetailsFromXDSClientResourceName(clientKey)
	snapshotClusterDeferralsTotal.Inc(
		metrics.Label{Name: gatewayLabel, Value: cd.Gateway},
		metrics.Label{Name: namespaceLabel, Value: cd.Namespace},
		metrics.Label{Name: reasonLabel, Value: string(reason)},
	)
}

// gatewayNamespace keys snapshotDeferredClients. The gauge aggregates over
// every client of a gateway rather than labelling by client: several UCCs share
// one gateway when pod-locality xDS is on, and a per-client series would need
// deleting on every disconnect.
type gatewayNamespace struct {
	Gateway   string
	Namespace string
}

// deferredClientsTracker keeps snapshotDeferredClients in step with the
// collections it is derived from: a connected client (a row in clients) is
// deferred while it has no row in clusters. Deriving the gauge from collection
// state rather than from transform side effects means a client that disconnects
// stops counting without a delete hook, and two handlers racing cannot leave a
// stale value behind: each recompute reads current state under the lock, so the
// last writer also read the newest state. Construct with
// [trackDeferredClients].
type deferredClientsTracker struct {
	clients  krt.Collection[ir.UniquelyConnectedClient]
	clusters krt.Collection[clustersWithErrors]

	mu sync.Mutex
	// exported is the set of series currently published, so a gateway whose
	// last client disconnects has its series removed rather than left at zero
	// forever.
	exported map[gatewayNamespace]struct{}
}

// trackDeferredClients registers handlers on both inputs of the deferred-client
// derivation. Handlers run on their own queues, so a recompute costs O(clients)
// off the transform path.
func trackDeferredClients(
	clients krt.Collection[ir.UniquelyConnectedClient],
	clusters krt.Collection[clustersWithErrors],
) {
	t := &deferredClientsTracker{
		clients:  clients,
		clusters: clusters,
		exported: make(map[gatewayNamespace]struct{}),
	}
	clients.RegisterBatch(func([]krt.Event[ir.UniquelyConnectedClient]) { t.recompute() }, true)
	clusters.RegisterBatch(func([]krt.Event[clustersWithErrors]) { t.recompute() }, true)
}

func (t *deferredClientsTracker) recompute() {
	if !metrics.Active() {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	deferred := make(map[gatewayNamespace]int)
	for _, ucc := range t.clients.List() {
		cd := getDetailsFromXDSClientResourceName(ucc.ResourceName())
		key := gatewayNamespace{Gateway: cd.Gateway, Namespace: cd.Namespace}
		// Every connected gateway gets a series, so a healthy gateway reads 0
		// rather than being absent.
		if _, ok := deferred[key]; !ok {
			deferred[key] = 0
		}
		if t.clusters.GetKey(ucc.ResourceName()) == nil {
			deferred[key]++
		}
	}

	for key := range t.exported {
		if _, ok := deferred[key]; ok {
			continue
		}
		snapshotDeferredClients.DeletePartialMatch(
			metrics.Label{Name: gatewayLabel, Value: key.Gateway},
			metrics.Label{Name: namespaceLabel, Value: key.Namespace},
		)
		delete(t.exported, key)
	}
	for key, n := range deferred {
		snapshotDeferredClients.Set(float64(n),
			metrics.Label{Name: gatewayLabel, Value: key.Gateway},
			metrics.Label{Name: namespaceLabel, Value: key.Namespace},
		)
		t.exported[key] = struct{}{}
	}
}

// snapshotResourcesMetricLabels defines the labels for XDS snapshot resources metrics.
type snapshotResourcesMetricLabels struct {
	Gateway   string
	Namespace string
	Resource  string
}

func (r snapshotResourcesMetricLabels) toMetricsLabels() []metrics.Label {
	return []metrics.Label{
		{Name: gatewayLabel, Value: r.Gateway},
		{Name: namespaceLabel, Value: r.Namespace},
		{Name: resourceLabel, Value: r.Resource},
	}
}

// collectXDSTransformMetrics is called at the start of a transform function to
// begin metrics collection and returns a function called at the end to complete
// metrics recording.
func collectXDSTransformMetrics(clientKey string) func(error) {
	if !metrics.Active() {
		return func(err error) {}
	}

	start := time.Now()

	cd := getDetailsFromXDSClientResourceName(clientKey)
	return func(err error) {
		result := "success"
		if err != nil {
			result = "error"
		}

		snapshotTransformsTotal.Inc(
			metrics.Label{Name: gatewayLabel, Value: cd.Gateway},
			metrics.Label{Name: namespaceLabel, Value: cd.Namespace},
			metrics.Label{Name: resultLabel, Value: result},
		)

		duration := time.Since(start)

		snapshotTransformDuration.Observe(duration.Seconds(),
			metrics.Label{Name: gatewayLabel, Value: cd.Gateway},
			metrics.Label{Name: namespaceLabel, Value: cd.Namespace},
		)
	}
}

type resourceNameDetails struct {
	Role      string
	Namespace string
	Gateway   string
}

// getDetailsFromXDSClientResourceName extracts details from an XDS client resource name.
func getDetailsFromXDSClientResourceName(resourceName string) resourceNameDetails {
	res := resourceNameDetails{
		Role:      "unknown",
		Namespace: "unknown",
		Gateway:   "unknown",
	}

	pks := strings.SplitN(resourceName, "~", 5)

	if len(pks) > 0 {
		res.Role = pks[0]
	}

	if len(pks) > 1 {
		res.Namespace = pks[1]
	}

	if len(pks) > 2 {
		res.Gateway = pks[2]
	}

	return res
}

// ResetMetrics resets the metrics from this package.
// This is provided for testing purposes only.
func ResetMetrics() {
	statussync.ResetMetrics()
	snapshotTransformsTotal.Reset()
	snapshotTransformDuration.Reset()
	snapshotResources.Reset()
	snapshotClusterDeferralsTotal.Reset()
	snapshotDeferredClients.Reset()
}
