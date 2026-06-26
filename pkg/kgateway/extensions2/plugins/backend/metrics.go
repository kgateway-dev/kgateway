package backend

import (
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
	// ec2PollReasonNone is the reason recorded on a successful poll that resolved
	// endpoints. It mirrors the convention of a non-empty reason on every series so
	// the label is never absent.
	ec2PollReasonNone = "none"
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
)

func ec2MetricIdentity(namespace, name string) []metrics.Label {
	return []metrics.Label{
		{Name: ec2MetricNamespaceLabel, Value: namespace},
		{Name: ec2MetricNameLabel, Value: name},
	}
}

// recordEc2PollSuccess records the outcome of a successful discovery poll for a
// Backend: it increments the poll counter, updates the active endpoint gauge to the
// freshly resolved count, and clears the error-state gauge. A successful poll that
// matched no instances is still a success, recorded with reason=NoMatchingInstances.
func recordEc2PollSuccess(namespace, name string, endpointCount int) {
	if !metrics.Active() {
		return
	}
	reason := ec2PollReasonNone
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

// deleteEc2DiscoveryMetrics removes every metric series for a Backend, called when the
// Backend is deleted so stale per-Backend gauges do not remain visible indefinitely.
func deleteEc2DiscoveryMetrics(namespace, name string) {
	identity := ec2MetricIdentity(namespace, name)
	ec2DiscoveryPollTotal.DeletePartialMatch(identity...)
	ec2DiscoveryEndpointsActive.DeletePartialMatch(identity...)
	ec2DiscoveryErrorState.DeletePartialMatch(identity...)
}

// ResetEc2DiscoveryMetrics resets the EC2 discovery metrics.
// This is provided for testing purposes only.
func ResetEc2DiscoveryMetrics() {
	ec2DiscoveryPollTotal.Reset()
	ec2DiscoveryEndpointsActive.Reset()
	ec2DiscoveryErrorState.Reset()
}
