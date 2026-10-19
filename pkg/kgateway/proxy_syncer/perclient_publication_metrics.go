package proxy_syncer

import (
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

// Per-client publication metrics. They show in the field when publish-time
// resolution runs and how it ends, which otherwise has to be inferred from
// debug logs. devel/architecture/metrics.md describes each one; keep the help
// strings to a line.
const (
	publicationReasonLabel = "reason"
	publicationModeLabel   = "mode"

	deferReasonMissingClusters  = "missing_clusters"
	deferReasonMissingEndpoints = "missing_endpoints"

	boundedPublishFirstPublish = "first_publish"
	// boundedPublishWarmTruth marks a bounded publish to a warm
	// (prior-xDS-version) client whose only remaining gaps were referenced
	// clusters with no derived CLA — their steady-state truth, not
	// propagation lag. Warm clients with clusters missing from CDS stay
	// withheld.
	boundedPublishWarmTruth = "warm_truth"
	// boundedPublishFlipRelease marks a held route flip published at budget
	// expiry: the newly-referenced cluster never became ready, and holding
	// longer would keep the client's route/listener/secret updates pinned.
	boundedPublishFlipRelease = "flip_release"
	// boundedPublishWarmRelease marks a warm client published after staying
	// withheld, with referenced clusters missing from CDS, for the whole warm
	// withhold bound (KGW_PER_CLIENT_WARM_WITHHOLD_BOUND): the missing clusters
	// were not translation catching up, and holding longer would freeze the
	// client's config until its proxy restarts.
	boundedPublishWarmRelease = "warm_release"
	// boundedPublishCarryExpired marks a published client stopped being
	// carried a referenced cluster missing from CDS after the warm withhold
	// bound: the cluster's last config is no longer served and routes to it
	// fail until it reappears.
	boundedPublishCarryExpired = "carry_expired"

	withheldStateLabel = "state"
	// withheldWithinBudget is a client whose first publish on its connection
	// waits for referenced clusters, inside the first-publish budget.
	withheldWithinBudget = "within_budget"
	// withheldWarm is a warm client still withheld after the budget because
	// referenced clusters are missing from CDS, up to the warm withhold bound.
	withheldWarm = "warm_withheld"
)

var (
	snapshotPerClientDefersTotal = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: snapshotSubsystem,
			Name:      "perclient_defers_total",
			Help:      "Per-client xDS snapshots built while a referenced cluster was not ready, by reason",
		},
		[]string{gatewayLabel, namespaceLabel, publicationReasonLabel},
	)
	snapshotPerClientWithheldClients = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: snapshotSubsystem,
			Name:      "perclient_withheld_clients",
			Help:      "Clients whose first publish on their connection is withheld until referenced clusters are ready, by state",
		},
		[]string{gatewayLabel, namespaceLabel, withheldStateLabel},
	)
	snapshotPerClientBoundedPublishesTotal = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: snapshotSubsystem,
			Name:      "perclient_bounded_publishes_total",
			Help:      "Withheld or held per-client publications released by a time bound before every referenced cluster was ready, by mode",
		},
		[]string{gatewayLabel, namespaceLabel, publicationModeLabel},
	)
	snapshotPerClientInconsistentTotal = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: snapshotSubsystem,
			Name:      "perclient_inconsistent_snapshots_total",
			Help:      "Per-client xDS snapshots that failed the pre-publication consistency check; any increment is a bug",
		},
		nil,
	)
)

// recordSnapshotDefer counts one deferred build for the client's gateway, per
// recorded gap kind.
func recordSnapshotDefer(proxyKey string, missing, missingEndpoints []string) {
	if !metrics.Active() {
		return
	}
	cd := getDetailsFromXDSClientResourceName(proxyKey)
	if len(missing) > 0 {
		snapshotPerClientDefersTotal.Inc(
			metrics.Label{Name: gatewayLabel, Value: cd.Gateway},
			metrics.Label{Name: namespaceLabel, Value: cd.Namespace},
			metrics.Label{Name: publicationReasonLabel, Value: deferReasonMissingClusters},
		)
	}
	if len(missingEndpoints) > 0 {
		snapshotPerClientDefersTotal.Inc(
			metrics.Label{Name: gatewayLabel, Value: cd.Gateway},
			metrics.Label{Name: namespaceLabel, Value: cd.Namespace},
			metrics.Label{Name: publicationReasonLabel, Value: deferReasonMissingEndpoints},
		)
	}
}

// recordBoundedPublish counts a deferred snapshot published at budget expiry,
// by mode (first_publish for cold clients, warm_truth for warm clients whose
// only gaps were endpoint-less clusters).
func recordBoundedPublish(proxyKey string, mode string) {
	if !metrics.Active() {
		return
	}
	cd := getDetailsFromXDSClientResourceName(proxyKey)
	snapshotPerClientBoundedPublishesTotal.Inc(
		metrics.Label{Name: gatewayLabel, Value: cd.Gateway},
		metrics.Label{Name: namespaceLabel, Value: cd.Namespace},
		metrics.Label{Name: publicationModeLabel, Value: mode},
	)
}

// recordWithheldClients publishes how many of one gateway's clients are
// withheld in each state, from the gate's counts of clients pending a first
// publish and of the warm ones among them withheld past the budget. A state
// with no clients has no series, so a gateway that is gone leaves none.
func recordWithheldClients(labels resourceNameDetails, pending, warm int) {
	for _, s := range []struct {
		state string
		n     int
	}{
		{withheldWithinBudget, pending - warm},
		{withheldWarm, warm},
	} {
		series := []metrics.Label{
			{Name: gatewayLabel, Value: labels.Gateway},
			{Name: namespaceLabel, Value: labels.Namespace},
			{Name: withheldStateLabel, Value: s.state},
		}
		if s.n > 0 {
			snapshotPerClientWithheldClients.Set(float64(s.n), series...)
		} else {
			snapshotPerClientWithheldClients.DeletePartialMatch(series...)
		}
	}
}

// recordInconsistentSnapshot counts a pre-publication consistency violation
// (see snapshotConsistencyError). It carries no gateway labels: any increment
// is a bug, and the error logged with it names the client.
func recordInconsistentSnapshot() {
	if !metrics.Active() {
		return
	}
	snapshotPerClientInconsistentTotal.Inc()
}
