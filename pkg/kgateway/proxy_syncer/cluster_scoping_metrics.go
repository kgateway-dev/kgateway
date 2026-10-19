package proxy_syncer

import (
	"sync"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	clusterScopingSubsystem = "xds_cluster_scoping"
	transitionLabel         = "transition"
)

var (
	// clusterScopingDroppedClusters is what scoping saves a client: clusters
	// translated for it and withheld because nothing references them.
	// xds_snapshot_resources{resource="Cluster"} already counts what it
	// receives. Zero with scoping on means the deployment has nothing to gain
	// from the feature.
	clusterScopingDroppedClusters = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: clusterScopingSubsystem,
			Name:      "dropped_clusters",
			Help:      "Backend clusters withheld from a client because its configuration references none of them",
		},
		[]string{gatewayLabel, namespaceLabel},
	)
	// clusterScopingDisabledGateways counts gateways whose CDS reverted to
	// every backend because a route picks its destination at request time.
	// Alert on it: without the revert the failure is silent, since the
	// selecting plugin falls back instead of returning 503. A nonzero value
	// means scoping is off for that gateway.
	clusterScopingDisabledGateways = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: clusterScopingSubsystem,
			Name:      "disabled_gateways",
			Help:      "Gateways whose CDS is unscoped because a route selects its destination at request time",
		},
		[]string{gatewayLabel, namespaceLabel},
	)
	// clusterScopingTransitionsTotal counts clusters whose emitted-set change
	// needed a window, by window. Each cluster counts once per window it
	// enters, not per publish. Windows are kept per client key (a gateway's
	// proxies sharing labels and locality), so a gateway spanning three zones
	// counts each change three times.
	//
	// These are expected during route churn. Watch the rate: route edits that
	// keep landing on never-sent clusters pay reference-ahead latency, a reason
	// to shorten the window.
	clusterScopingTransitionsTotal = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: clusterScopingSubsystem,
			Name:      "transitions_total",
			Help:      "Clusters whose emitted-set change needed a grace window, per proxy group, by transition",
		},
		[]string{gatewayLabel, namespaceLabel, transitionLabel},
	)
)

const (
	// transitionDereferenceGraced is a cluster kept past its last reference.
	transitionDereferenceGraced = "dereference_graced"
	// transitionDereferencePruned is one actually removed once its window closed.
	transitionDereferencePruned = "dereference_pruned"
	// transitionReferenceAheadHeld is a newly-emitted cluster sent ahead of the
	// route update naming it; a retarget onto n new clusters counts n.
	transitionReferenceAheadHeld = "reference_ahead_held"
)

// recordClusterScopingDropped reports how many clusters scoping kept out of a
// client's CDS. Called on every scoped build, so the gauge tracks the current
// state.
func recordClusterScopingDropped(clientKey string, dropped int) {
	if !metrics.Active() {
		return
	}
	cd := getDetailsFromXDSClientResourceName(clientKey)
	clusterScopingDroppedClusters.Set(float64(dropped),
		metrics.Label{Name: gatewayLabel, Value: cd.Gateway},
		metrics.Label{Name: namespaceLabel, Value: cd.Namespace},
	)
}

// recordClusterScopingDisabled sets this gateway's disabled gauge to 1 or 0.
// Recording 0 keeps the series present, so an alert fires on a change to 1
// rather than on a new label.
func recordClusterScopingDisabled(clientKey string, disabled bool) {
	if !metrics.Active() {
		return
	}
	cd := getDetailsFromXDSClientResourceName(clientKey)
	value := 0.0
	if disabled {
		value = 1.0
	}
	clusterScopingDisabledGateways.Set(value,
		metrics.Label{Name: gatewayLabel, Value: cd.Gateway},
		metrics.Label{Name: namespaceLabel, Value: cd.Namespace},
	)
}

// clusterScopingDisabledState remembers each gateway's last reported state, so
// the warning fires when a gateway turns unscoped, not on every client rebuild.
var clusterScopingDisabledState = struct {
	sync.Mutex
	gateways map[resourceNameDetails]bool
}{gateways: make(map[resourceNameDetails]bool)}

// noteClusterScopingDisabled records clientKey's gateway state and reports
// whether it just turned unscoped.
func noteClusterScopingDisabled(clientKey string, disabled bool) bool {
	cd := getDetailsFromXDSClientResourceName(clientKey)
	gateway := resourceNameDetails{Namespace: cd.Namespace, Gateway: cd.Gateway}
	clusterScopingDisabledState.Lock()
	defer clusterScopingDisabledState.Unlock()
	was := clusterScopingDisabledState.gateways[gateway]
	clusterScopingDisabledState.gateways[gateway] = disabled
	return disabled && !was
}

func recordClusterScopingTransition(clientKey, transition string, count int) {
	if !metrics.Active() || count <= 0 {
		return
	}
	cd := getDetailsFromXDSClientResourceName(clientKey)
	clusterScopingTransitionsTotal.Add(float64(count),
		metrics.Label{Name: gatewayLabel, Value: cd.Gateway},
		metrics.Label{Name: namespaceLabel, Value: cd.Namespace},
		metrics.Label{Name: transitionLabel, Value: transition},
	)
}

// gatewayHasClients reports whether any of clients belongs to clientKey's
// gateway.
func gatewayHasClients(clientKey string, clients []ir.UniquelyConnectedClient) bool {
	departed := getDetailsFromXDSClientResourceName(clientKey)
	for _, ucc := range clients {
		cd := getDetailsFromXDSClientResourceName(ucc.ResourceName())
		if cd.Gateway == departed.Gateway && cd.Namespace == departed.Namespace {
			return true
		}
	}
	return false
}

// forgetClusterScopingGateway drops a gateway's scoping gauges and warning
// state once none of its clients is connected. Nothing else removes them, so a
// deleted gateway would keep exporting disabled_gateways=1 and firing alerts. A
// returning client re-creates them on its first build.
func forgetClusterScopingGateway(clientKey string) {
	cd := getDetailsFromXDSClientResourceName(clientKey)
	labels := []metrics.Label{
		{Name: gatewayLabel, Value: cd.Gateway},
		{Name: namespaceLabel, Value: cd.Namespace},
	}
	clusterScopingDroppedClusters.DeletePartialMatch(labels...)
	clusterScopingDisabledGateways.DeletePartialMatch(labels...)
	clusterScopingDisabledState.Lock()
	defer clusterScopingDisabledState.Unlock()
	delete(clusterScopingDisabledState.gateways, resourceNameDetails{Namespace: cd.Namespace, Gateway: cd.Gateway})
}
