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
	// translated for it and withheld because nothing references them. How many
	// it does receive is already xds_snapshot_resources{resource="Cluster"},
	// which counts the published snapshot. Zero here with scoping on means the
	// deployment routes to everything it discovers and has nothing to gain from
	// the feature.
	clusterScopingDroppedClusters = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: clusterScopingSubsystem,
			Name:      "dropped_clusters",
			Help:      "Backend clusters withheld from a client because its configuration references none of them",
		},
		[]string{gatewayLabel, namespaceLabel},
	)
	// clusterScopingDisabledGateways counts gateways whose emitted set could not
	// be trusted, so CDS reverted to carrying every backend for them.
	//
	// This is the metric to alert on. A gateway lands here when a route picks
	// its destination at request time, from a set the configuration never
	// enumerates, and the failure mode without the revert is silent: the
	// selecting plugin falls back rather than 503ing, so requests quietly go
	// somewhere else. A nonzero value means the optimization is off for that
	// gateway and someone should look at why.
	clusterScopingDisabledGateways = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: clusterScopingSubsystem,
			Name:      "disabled_gateways",
			Help:      "Gateways whose CDS is unscoped because a route selects its destination at request time",
		},
		[]string{gatewayLabel, namespaceLabel},
	)
	// clusterScopingTransitionsTotal counts the clusters whose emitted-set change
	// needed a window to be safe, by which window. Each cluster counts once per
	// window it enters, not once per publish while the window is open.
	//
	// These are expected during route churn, not faults. What is worth watching
	// is the rate: a fleet whose route edits keep landing on never-sent clusters
	// is a fleet paying reference-ahead latency on most of them, which is a
	// reason to shorten the window rather than a bug.
	clusterScopingTransitionsTotal = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: clusterScopingSubsystem,
			Name:      "transitions_total",
			Help:      "Emitted-set transitions that required a grace window",
		},
		[]string{gatewayLabel, namespaceLabel, transitionLabel},
	)
)

const (
	// transitionDereferenceGraced is a cluster kept past its last reference.
	transitionDereferenceGraced = "dereference_graced"
	// transitionDereferencePruned is one actually removed once its window closed.
	transitionDereferencePruned = "dereference_pruned"
	// transitionReferenceAheadHeld is a newly-emitted cluster delivered ahead of
	// the route update that named it; a retarget onto n new clusters counts n,
	// like the de-reference transitions count clusters.
	transitionReferenceAheadHeld = "reference_ahead_held"
)

// recordClusterScopingDropped reports how many clusters scoping kept out of a
// client's CDS. Called on every publish that filtered, so the gauge tracks the
// current state rather than a high-water mark.
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

// recordClusterScopingDisabled reports whether this gateway's CDS had to revert
// to carrying every backend. Recorded for filterable gateways too, as 0, so the
// series exists and an alert can fire on a transition to 1 rather than on the
// appearance of a label.
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

// clusterScopingDisabledState remembers, per gateway, whether its CDS was last
// reported unscoped, so the warning fires when a gateway turns unscoped rather
// than on every rebuild of every one of its clients.
var clusterScopingDisabledState = struct {
	sync.Mutex
	gateways map[resourceNameDetails]bool
}{gateways: make(map[resourceNameDetails]bool)}

// noteClusterScopingDisabled records whether clientKey's gateway is unscoped
// and reports whether that is a change to unscoped.
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
	for range count {
		clusterScopingTransitionsTotal.Inc(
			metrics.Label{Name: gatewayLabel, Value: cd.Gateway},
			metrics.Label{Name: namespaceLabel, Value: cd.Namespace},
			metrics.Label{Name: transitionLabel, Value: transition},
		)
	}
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

// forgetClusterScopingGateway drops a gateway's scoping gauges, and the state
// behind its unscoped warning, once none of its clients is connected. The
// gauges are written on every publish and nothing else removes them, so a
// deleted gateway would otherwise export its last values forever: a
// disabled_gateways of 1 keeps an alert firing for a gateway that is gone. A
// client that connects again re-creates them on its first publish.
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
