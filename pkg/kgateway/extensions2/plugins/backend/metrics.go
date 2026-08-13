package backend

import (
	"time"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

// EC2 endpoint discovery metrics. The exposed names are
// kgateway_ec2_discovery_poll_total, kgateway_ec2_discovery_endpoints_active, and
// kgateway_ec2_discovery_error_state, following the kgateway_<subsystem>_<name>
// convention used elsewhere in the codebase.
const (
	ec2DiscoverySubsystem = "ec2_discovery"

	ec2MetricNamespaceLabel = "namespace"
	ec2MetricNameLabel      = "name"
	ec2MetricResultLabel    = "result"
	ec2MetricReasonLabel    = "reason"

	ec2PollResultSuccess = "success"
	ec2PollResultError   = "error"
)

var (
	// ec2DiscoveryPollTotal counts EC2 discovery refresh attempts per Backend,
	// partitioned by result (success/error) and reason. The reason values match the
	// Reason values used in the Backend's EndpointsDiscovered status condition.
	ec2DiscoveryPollTotal = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: ec2DiscoverySubsystem,
			Name:      "poll_total",
			Help:      "Total number of EC2 endpoint discovery refresh attempts per Backend",
		},
		[]string{ec2MetricNamespaceLabel, ec2MetricNameLabel, ec2MetricResultLabel, ec2MetricReasonLabel},
	)

	// ec2DiscoveryEndpointsActive reports the number of active Envoy endpoints for a
	// Backend after the most recent successful poll. It is not updated on a failed
	// poll, so it retains the last successful value (NFR-3 graceful degradation).
	ec2DiscoveryEndpointsActive = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: ec2DiscoverySubsystem,
			Name:      "endpoints_active",
			Help:      "Current number of active Envoy endpoints discovered for an EC2 Backend",
		},
		[]string{ec2MetricNamespaceLabel, ec2MetricNameLabel},
	)

	// ec2DiscoveryErrorState is 1 when the most recent poll for a Backend failed and 0
	// when it succeeded. It intentionally carries no reason label: reason-specific
	// diagnosis is provided by ec2DiscoveryPollTotal and the Backend status condition.
	ec2DiscoveryErrorState = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: ec2DiscoverySubsystem,
			Name:      "error_state",
			Help:      "Whether the most recent EC2 discovery poll for a Backend failed (1) or succeeded (0)",
		},
		[]string{ec2MetricNamespaceLabel, ec2MetricNameLabel},
	)

	// ec2DiscoveryPollDuration measures the wall-clock time of the AWS
	// DescribeInstances round trip for a poll, attributed to each Backend in the
	// credential scope. Both successful and failed polls are observed, so the
	// distribution includes slow timeouts; it is partitioned by result so operators
	// can isolate successful-poll latency from failures, whose latency is bimodal
	// (fast credential/authorization rejections vs. slow timeouts). Only result is
	// added, not reason: reason belongs on the counter, and native histograms
	// allocate per label-set, so each extra label value multiplies retained
	// histograms per Backend. Buckets span from a fast in-region listing up to
	// ec2RefreshTimeout (30s).
	ec2DiscoveryPollDuration = metrics.NewHistogram(
		metrics.HistogramOpts{
			Subsystem: ec2DiscoverySubsystem,
			Name:      "poll_duration_seconds",
			Help:      "Duration of EC2 endpoint discovery polls per Backend",
			// Classic buckets are kept as a fallback for scrapers that do not
			// support native histograms; they span a fast in-region listing up to
			// the 30s refresh timeout.
			Buckets:                         []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 30},
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		},
		[]string{ec2MetricNamespaceLabel, ec2MetricNameLabel, ec2MetricResultLabel},
	)
)

func ec2MetricIdentity(namespace, name string) []metrics.Label {
	return []metrics.Label{
		{Name: ec2MetricNamespaceLabel, Value: namespace},
		{Name: ec2MetricNameLabel, Value: name},
	}
}

// recordEc2PollSuccess records the outcome of a successful discovery poll for a
// Backend: it increments the poll counter, updates the active endpoint gauge to the
// freshly resolved count, and clears the error-state gauge. The reason label mirrors
// the Backend's EndpointsDiscovered status condition: reason=Discovered when endpoints
// resolved, and reason=NoMatchingInstances when the poll succeeded but matched none.
func recordEc2PollSuccess(namespace, name string, endpointCount int) {
	if !metrics.Active() {
		return
	}
	reason := string(kgateway.BackendReasonDiscovered)
	if endpointCount == 0 {
		reason = string(kgateway.BackendReasonNoMatchingInstances)
	}
	identity := ec2MetricIdentity(namespace, name)
	ec2DiscoveryPollTotal.Inc(append(identity,
		metrics.Label{Name: ec2MetricResultLabel, Value: ec2PollResultSuccess},
		metrics.Label{Name: ec2MetricReasonLabel, Value: reason},
	)...)
	ec2DiscoveryEndpointsActive.Set(float64(endpointCount), identity...)
	ec2DiscoveryErrorState.Set(0, identity...)
}

// recordEc2PollError records the outcome of a failed discovery poll for a Backend:
// it increments the poll counter with the classified failure reason and sets the
// error-state gauge to 1. The active endpoint gauge is intentionally left unchanged
// so it retains the last successful value (graceful degradation). The reason
// is the underlying classification (CredentialError/AuthorizationError/DiscoveryError),
// not the Degraded status reason, so the counter always attributes a concrete cause.
func recordEc2PollError(namespace, name, reason string) {
	if !metrics.Active() {
		return
	}
	identity := ec2MetricIdentity(namespace, name)
	ec2DiscoveryPollTotal.Inc(append(identity,
		metrics.Label{Name: ec2MetricResultLabel, Value: ec2PollResultError},
		metrics.Label{Name: ec2MetricReasonLabel, Value: reason},
	)...)
	ec2DiscoveryErrorState.Set(1, identity...)
}

// recordEc2PollDuration observes how long a discovery poll took for a Backend,
// partitioned by result (success/error). The same duration is recorded for every
// Backend sharing a credential scope, since they are resolved from a single AWS
// DescribeInstances call.
func recordEc2PollDuration(namespace, name, result string, seconds float64) {
	if !metrics.Active() {
		return
	}
	ec2DiscoveryPollDuration.Observe(seconds, append(ec2MetricIdentity(namespace, name),
		metrics.Label{Name: ec2MetricResultLabel, Value: result},
	)...)
}

// recordEc2CredentialErrorState marks a Backend as being in an error state because its
// secret-auth credential could not be resolved. Such a Backend never enters the poll
// loop, so this is the only place its error_state is set; endpoints_active is set to 0
// because no endpoints are served while credentials are unresolved (and so a Backend
// that was previously healthy doesn't leave a stale non-zero gauge after its secret is
// removed). poll_total and poll_duration_seconds are intentionally left untouched: no
// poll occurs, and incrementing a counter from a KRT recompute would double-count.
func recordEc2CredentialErrorState(namespace, name string) {
	if !metrics.Active() {
		return
	}
	identity := ec2MetricIdentity(namespace, name)
	ec2DiscoveryErrorState.Set(1, identity...)
	ec2DiscoveryEndpointsActive.Set(0, identity...)
}

// deleteEc2DiscoveryMetrics removes every metric series for a Backend, called when the
// Backend is deleted so stale per-Backend gauges do not remain visible indefinitely.
func deleteEc2DiscoveryMetrics(namespace, name string) {
	identity := ec2MetricIdentity(namespace, name)
	ec2DiscoveryPollTotal.DeletePartialMatch(identity...)
	ec2DiscoveryEndpointsActive.DeletePartialMatch(identity...)
	ec2DiscoveryErrorState.DeletePartialMatch(identity...)
	ec2DiscoveryPollDuration.DeletePartialMatch(identity...)
}

// ResetEc2DiscoveryMetrics resets the EC2 discovery metrics.
// This is provided for testing purposes only.
func ResetEc2DiscoveryMetrics() {
	ec2DiscoveryPollTotal.Reset()
	ec2DiscoveryEndpointsActive.Reset()
	ec2DiscoveryErrorState.Reset()
	ec2DiscoveryPollDuration.Reset()
}
