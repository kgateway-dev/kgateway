package proxy_syncer

import (
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

const (
	clusterScopingSubsystem = "xds_cluster_scoping"
	transitionLabel         = "transition"
)

var (
	// clusterScopingEmittedClusters is how many of a client's translated
	// backend clusters actually reached its CDS. Reading it against
	// xds_snapshot_resources{resource="cluster"} is the whole point of the
	// feature: the gap between them is the inventory each proxy no longer
	// carries.
	clusterScopingEmittedClusters = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: clusterScopingSubsystem,
			Name:      "emitted_clusters",
			Help:      "Backend clusters published to a client after scoping CDS to what its configuration references",
		},
		[]string{gatewayLabel, namespaceLabel},
	)
	// clusterScopingDroppedClusters is the complement: clusters translated for
	// this client and withheld because nothing references them. Zero here with
	// scoping on means the deployment routes to everything it discovers and has
	// nothing to gain from the feature.
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
	// clusterScopingTransitionsTotal counts the emitted-set changes that needed
	// a window to be safe, by which window.
	//
	// These are expected during route churn, not faults. What is worth watching
	// is the ratio to route updates: a fleet where nearly every publish holds is
	// a fleet paying reference-ahead latency on most route edits, which is a
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
	// transitionReferenceAheadHeld is a route update held so a newly-emitted
	// cluster could be delivered first.
	transitionReferenceAheadHeld = "reference_ahead_held"
)

// recordClusterScopingEmission reports how a client's CDS was scoped. Called on
// every publish that filtered, so the gauges track the current state rather
// than a high-water mark.
func recordClusterScopingEmission(clientKey string, emitted, dropped int) {
	if !metrics.Active() {
		return
	}
	cd := getDetailsFromXDSClientResourceName(clientKey)
	labels := []metrics.Label{
		{Name: gatewayLabel, Value: cd.Gateway},
		{Name: namespaceLabel, Value: cd.Namespace},
	}
	clusterScopingEmittedClusters.Set(float64(emitted), labels...)
	clusterScopingDroppedClusters.Set(float64(dropped), labels...)
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
