package proxy_syncer

import (
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

// Per-client publication metrics. They show when publish-time resolution runs
// and how it ends. devel/architecture/metrics.md describes each one; keep the
// help strings to a line.
const (
	publicationReasonLabel = "reason"
	publicationModeLabel   = "mode"

	deferReasonMissingClusters  = "missing_clusters"
	deferReasonMissingEndpoints = "missing_endpoints"

	boundedPublishFirstPublish = "first_publish"
	// boundedPublishWarmTruth marks a bounded publish to a warm client whose
	// only gaps were referenced clusters with no derived CLA; withholding would
	// freeze it indefinitely. Warm clients missing CDS clusters stay withheld.
	boundedPublishWarmTruth = "warm_truth"
	// boundedPublishFlipRelease marks a held flip released at budget expiry:
	// the new cluster never became ready, and holding longer would keep the
	// client's route/listener/secret updates pinned.
	boundedPublishFlipRelease = "flip_release"
	// boundedPublishWarmRelease marks a warm client published after being
	// withheld for KGW_PER_CLIENT_WARM_WITHHOLD_BOUND with clusters missing
	// from CDS; holding longer would freeze its config until its proxy restarts.
	boundedPublishWarmRelease = "warm_release"
	// boundedPublishCarryExpired marks a published client that stopped being
	// carried a cluster missing from CDS for the warm withhold bound. Routes to
	// it fail until it reappears.
	boundedPublishCarryExpired = "carry_expired"
	// boundedPublishReferenceAhead marks a hold kept only for reference-ahead
	// windows (KGW_XDS_SCOPING_REFERENCE_AHEAD) that the publish budget released
	// early. Windows closing on their own are not a bounded publish.
	boundedPublishReferenceAhead = "reference_ahead"

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
	snapshotPerClientReclaimedTotal = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: snapshotSubsystem,
			Name:      "perclient_reclaimed_total",
			Help:      "Per-client xDS snapshot cache entries reclaimed after their client left and stayed gone for the grace period",
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

// recordBoundedPublish counts a withheld or held publication that a time
// bound released before every referenced cluster was ready, by mode.
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

// recordWithheldClients sets one gateway's withheld-client gauge per state
// from the gate's pending and warm counts. A state with no clients has no
// series, so a gateway that is gone leaves none.
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
// (see snapshotConsistencyError). It is unlabeled: any increment is a bug,
// and the logged error names the client.
func recordInconsistentSnapshot() {
	if !metrics.Active() {
		return
	}
	snapshotPerClientInconsistentTotal.Inc()
}

// recordSnapshotReclaim counts a reclaimed cache entry. It has no gateway
// labels: the client is gone, often with its gateway, so a labeled series
// would outlive what it names.
func recordSnapshotReclaim() {
	if !metrics.Active() {
		return
	}
	snapshotPerClientReclaimedTotal.Inc()
}
