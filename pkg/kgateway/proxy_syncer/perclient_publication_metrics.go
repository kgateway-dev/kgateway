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
	snapshotPerClientWarmWithheldClients = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: snapshotSubsystem,
			Name:      "perclient_warm_withheld_clients",
			Help: "Warm clients (they reported a prior accepted xDS version, so " +
				"they may be serving config from an earlier stream) currently " +
				"withheld because referenced clusters are missing from CDS. " +
				"While withheld a client receives no updates at all, including " +
				"certificate rotations. A client that stays here is released at " +
				"KGW_PER_CLIENT_WARM_WITHHOLD_BOUND, or never if that is 0.",
		},
		[]string{gatewayLabel, namespaceLabel},
	)
	snapshotPerClientFirstPublishPendingClients = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: snapshotSubsystem,
			Name:      "perclient_first_publish_pending_clients",
			Help: "Clients whose first publish on their current connection is " +
				"being withheld because referenced clusters are not ready, cold " +
				"or warm. Until it goes out such a client has nothing from this " +
				"connection: a new proxy has no listeners and is not Ready. A cold " +
				"client is published at KGW_PER_CLIENT_PUBLISH_BUDGET; a warm one " +
				"can stay withheld up to KGW_PER_CLIENT_WARM_WITHHOLD_BOUND (see " +
				"perclient_warm_withheld_clients).",
		},
		[]string{gatewayLabel, namespaceLabel},
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
		[]string{gatewayLabel, namespaceLabel},
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

// recordWarmWithheldClients publishes the number of warm clients currently
// withheld for one gateway.
// recordFirstPublishPendingClients publishes the number of clients of one
// gateway whose first publish on their connection is withheld.
func recordFirstPublishPendingClients(labels resourceNameDetails, n int) {
	snapshotPerClientFirstPublishPendingClients.Set(float64(n),
		metrics.Label{Name: gatewayLabel, Value: labels.Gateway},
		metrics.Label{Name: namespaceLabel, Value: labels.Namespace},
	)
}

func recordWarmWithheldClients(labels resourceNameDetails, n int) {
	snapshotPerClientWarmWithheldClients.Set(float64(n),
		metrics.Label{Name: gatewayLabel, Value: labels.Gateway},
		metrics.Label{Name: namespaceLabel, Value: labels.Namespace},
	)
}

// recordInconsistentSnapshot counts a pre-publication consistency violation
// (see snapshotConsistencyError).
func recordInconsistentSnapshot(proxyKey string) {
	if !metrics.Active() {
		return
	}
	cd := getDetailsFromXDSClientResourceName(proxyKey)
	snapshotPerClientInconsistentTotal.Inc(
		metrics.Label{Name: gatewayLabel, Value: cd.Gateway},
		metrics.Label{Name: namespaceLabel, Value: cd.Namespace},
	)
}
