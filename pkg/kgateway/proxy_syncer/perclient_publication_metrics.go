package proxy_syncer

import (
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

// Per-client publication counters. These make the publish-time resolution
// legible in the field: the #14184 triage had to infer all of this from debug
// log volume.
const (
	publicationReasonLabel = "reason"
	publicationModeLabel   = "mode"

	deferReasonMissingClusters  = "missing_clusters"
	deferReasonMissingEndpoints = "missing_endpoints"

	boundedPublishFirstPublish = "first_publish"
	// boundedPublishWarmTruth marks a bounded publish to a warm
	// (prior-xDS-version) client whose only remaining gaps were referenced
	// clusters with no derived CLA — their steady-state truth, not
	// propagation lag (#14352). Warm clients with clusters missing from CDS
	// stay withheld.
	boundedPublishWarmTruth = "warm_truth"
	// boundedPublishFlipRelease marks a held route flip published at budget
	// expiry: the newly-referenced cluster never became ready, and holding
	// longer would keep the client's route/listener/secret updates pinned
	// (#14352).
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
			Help: "Total per-client XDS snapshots built with unready referenced " +
				"clusters, by reason (missing_clusters: absent from CDS; " +
				"missing_endpoints: no derived CLA — a derived-but-empty CLA is " +
				"the backend's truth and never counts, #14352). Each increment " +
				"means publish-time resolution ran for the client " +
				"(carry-forward, vanished-slices truth, or a held route flip). " +
				"A sustained rate for a gateway means some referenced cluster " +
				"is not becoming ready (#14184).",
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
			Help: "Total withheld or held publications released because the publish " +
				"budget (KGW_PER_CLIENT_PUBLISH_BUDGET) expired. " +
				"mode=first_publish: a never-published client started on " +
				"incomplete-but-consistent config instead of waiting indefinitely " +
				"(it would otherwise have crash-looped, #14184). mode=warm_truth: " +
				"a warm client's only gaps were referenced clusters with no " +
				"derived CLA, and withholding would have frozen its config " +
				"indefinitely (#14352); those CLAs were left out, so the client " +
				"kept its endpoints for them. mode=flip_release: a " +
				"held route flip published because its newly-referenced cluster " +
				"never became ready; the affected routes fail until it does, and " +
				"pinned route/listener/secret updates resume. mode=warm_release: a " +
				"warm client withheld for the whole warm withhold bound " +
				"(KGW_PER_CLIENT_WARM_WITHHOLD_BOUND) because referenced clusters " +
				"were missing from CDS; routes to them fail until they appear. " +
				"mode=carry_expired: a published client stopped being carried a " +
				"referenced cluster that stayed missing from CDS for the warm " +
				"withhold bound; routes to it fail until it reappears.",
		},
		[]string{gatewayLabel, namespaceLabel, publicationModeLabel},
	)
	snapshotPerClientInconsistentTotal = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: snapshotSubsystem,
			Name:      "perclient_inconsistent_snapshots_total",
			Help: "Total per-client snapshots that, immediately before " +
				"publication, carried an EDS or RDS resource that no CDS or LDS " +
				"resource in the same snapshot references (go-control-plane " +
				"withholds an ADS response carrying an unrequested resource), " +
				"or a listener naming a RouteConfiguration the snapshot lacks. " +
				"Only recorded when KGW_XDS_SNAPSHOT_CONSISTENCY_CHECK is " +
				"enabled (test/CI environments); the snapshot is still " +
				"published. The publication paths maintain consistency by " +
				"construction, so any increment is a kgateway bug — please " +
				"report it with the accompanying error log line.",
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
