package proxy_syncer

import (
	"log/slog"
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
	// every backend because their configuration, or a kept backend's own
	// listeners, pick a destination at request time. Alert on it: without the
	// revert the failure is silent, since the selecting plugin falls back
	// instead of returning 503. A nonzero value means scoping is off for at
	// least one of the gateway's connected proxy groups.
	clusterScopingDisabledGateways = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: clusterScopingSubsystem,
			Name:      "disabled_gateways",
			Help:      "Gateways with a connected proxy group whose CDS is unscoped because its configuration selects a destination at request time",
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

// clusterScopingDisabledState is each gateway's scoping state per connected
// client group. Groups of one gateway can differ, since a backend errored for
// one client keeps or drops its listeners for that client alone, so a gateway
// reads as unscoped while any group is, and one group's healthy build cannot
// clear another's fallback.
var clusterScopingDisabledState = struct {
	sync.Mutex
	gateways map[resourceNameDetails]*gatewayScopingState
}{gateways: make(map[resourceNameDetails]*gatewayScopingState)}

type gatewayScopingState struct {
	// clients is each connected group's state, by client key.
	clients map[string]bool
	// disabled is the gateway's state as last reported.
	disabled bool
}

// update recomputes the gateway's state from its groups and reports whether
// it just turned unscoped.
func (g *gatewayScopingState) update() (turnedOn bool) {
	disabled := false
	for _, clientDisabled := range g.clients {
		disabled = disabled || clientDisabled
	}
	turnedOn = disabled && !g.disabled
	g.disabled = disabled
	return turnedOn
}

// reportClusterScopingDisabled records why clientKey's CDS is unscoped (no
// reasons: it is scoped), sets its gateway's disabled gauge, and warns when the
// gateway turns unscoped rather than on every build. Recording 0 keeps the
// series present, so an alert fires on a change to 1 rather than on a new
// label.
func reportClusterScopingDisabled(log *slog.Logger, clientKey string, reasons []string) {
	cd := getDetailsFromXDSClientResourceName(clientKey)
	gateway := resourceNameDetails{Namespace: cd.Namespace, Gateway: cd.Gateway}
	clusterScopingDisabledState.Lock()
	defer clusterScopingDisabledState.Unlock()
	state := clusterScopingDisabledState.gateways[gateway]
	if state == nil {
		state = &gatewayScopingState{clients: make(map[string]bool)}
		clusterScopingDisabledState.gateways[gateway] = state
	}
	state.clients[clientKey] = len(reasons) > 0
	if state.update() {
		log.Warn("cluster scoping disabled for this gateway: its configuration or a kept backend's listeners select a destination at request time",
			"client", clientKey,
			"reasons", reasons)
	}
	setClusterScopingDisabledGauge(cd, state.disabled)
}

func setClusterScopingDisabledGauge(cd resourceNameDetails, disabled bool) {
	if !metrics.Active() {
		return
	}
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

// forgetClusterScopingClient drops a departed client group's scoping state.
// While the gateway still has connected clients, its disabled gauge is
// recomputed without the group. Once none is connected, the gateway's gauges
// and state go: nothing else removes them, so a deleted gateway would keep
// exporting disabled_gateways=1 and firing alerts. A returning client
// re-creates them on its first build.
func forgetClusterScopingClient(clientKey string, gatewayConnected bool) {
	cd := getDetailsFromXDSClientResourceName(clientKey)
	gateway := resourceNameDetails{Namespace: cd.Namespace, Gateway: cd.Gateway}
	clusterScopingDisabledState.Lock()
	defer clusterScopingDisabledState.Unlock()
	state := clusterScopingDisabledState.gateways[gateway]
	if gatewayConnected {
		if state != nil {
			delete(state.clients, clientKey)
			state.update()
			setClusterScopingDisabledGauge(cd, state.disabled)
		}
		return
	}
	labels := []metrics.Label{
		{Name: gatewayLabel, Value: cd.Gateway},
		{Name: namespaceLabel, Value: cd.Namespace},
	}
	clusterScopingDroppedClusters.DeletePartialMatch(labels...)
	clusterScopingDisabledGateways.DeletePartialMatch(labels...)
	delete(clusterScopingDisabledState.gateways, gateway)
}
