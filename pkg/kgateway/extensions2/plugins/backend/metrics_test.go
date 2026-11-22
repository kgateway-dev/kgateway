package backend

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/smithy-go"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	pollTotalMetric       = "kgateway_ec2_discovery_poll_total"
	endpointsActiveMetric = "kgateway_ec2_discovery_endpoints_active"
	errorStateMetric      = "kgateway_ec2_discovery_error_state"
	pollDurationMetric    = "kgateway_ec2_discovery_poll_duration_seconds"
)

func backendIdentityLabels(namespace, name string) []metrics.Label {
	return []metrics.Label{
		{Name: "name", Value: name},
		{Name: "namespace", Value: namespace},
	}
}

// pollDurationLabels returns the label set for a poll_duration_seconds series,
// which is partitioned by result in addition to the Backend identity. Labels are
// kept in sorted order (name < namespace < result) to match the gathered output.
func pollDurationLabels(namespace, name, result string) []metrics.Label {
	return append(backendIdentityLabels(namespace, name),
		metrics.Label{Name: "result", Value: result},
	)
}

func TestComputeStateRecordsSuccessfulPollMetrics(t *testing.T) {
	ResetEc2DiscoveryMetrics()

	backend := newEc2Backend("backend-a", "arn:aws:iam::123456789012:role/shared", nil)
	c := &ec2EndpointsCollection{
		backends: krt.NewStaticCollection(nil, []ir.BackendObjectIR{
			backendObjectIR(backend, newTestAWSSecret("aws-creds", "default", "1")),
		}),
		lister: &fakeEc2InstanceLister{
			instances: []ec2DiscoveredInstance{
				{instanceID: "i-1", privateIP: "10.0.0.10"},
				{instanceID: "i-2", privateIP: "10.0.0.11"},
			},
		},
	}

	if _, err := c.computeState(context.Background()); err != nil {
		t.Fatalf("computeState() error = %v", err)
	}

	gathered := metricstest.MustGatherMetrics(t)
	gathered.AssertMetricsInclude(pollTotalMetric, []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "name", Value: "backend-a"},
				{Name: "namespace", Value: "default"},
				{Name: "reason", Value: "Discovered"},
				{Name: "result", Value: "success"},
			},
			Value: 1,
		},
	})
	gathered.AssertMetric(endpointsActiveMetric, &metricstest.ExpectedMetric{
		Labels: backendIdentityLabels("default", "backend-a"),
		Value:  2,
	})
	gathered.AssertMetric(errorStateMetric, &metricstest.ExpectedMetric{
		Labels: backendIdentityLabels("default", "backend-a"),
		Value:  0,
	})
	gathered.AssertMetricLabels(pollDurationMetric, pollDurationLabels("default", "backend-a", "success"))
	gathered.AssertHistogramPopulated(pollDurationMetric)
}

func TestComputeStateRecordsNoMatchingInstancesMetrics(t *testing.T) {
	ResetEc2DiscoveryMetrics()

	backend := newEc2Backend("backend-a", "arn:aws:iam::123456789012:role/shared", []kgateway.AwsTagFilter{tagKeyValue("app", "nope")})
	c := &ec2EndpointsCollection{
		backends: krt.NewStaticCollection(nil, []ir.BackendObjectIR{
			backendObjectIR(backend, newTestAWSSecret("aws-creds", "default", "1")),
		}),
		lister: &fakeEc2InstanceLister{
			instances: []ec2DiscoveredInstance{
				{instanceID: "i-1", privateIP: "10.0.0.10", tags: map[string]string{"app": "payments"}},
			},
		},
	}

	if _, err := c.computeState(context.Background()); err != nil {
		t.Fatalf("computeState() error = %v", err)
	}

	gathered := metricstest.MustGatherMetrics(t)
	// A successful poll that matched nothing is still result=success, with
	// reason=NoMatchingInstances rather than an error.
	gathered.AssertMetricsInclude(pollTotalMetric, []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "name", Value: "backend-a"},
				{Name: "namespace", Value: "default"},
				{Name: "reason", Value: "NoMatchingInstances"},
				{Name: "result", Value: "success"},
			},
			Value: 1,
		},
	})
	gathered.AssertMetric(endpointsActiveMetric, &metricstest.ExpectedMetric{
		Labels: backendIdentityLabels("default", "backend-a"),
		Value:  0,
	})
	gathered.AssertMetric(errorStateMetric, &metricstest.ExpectedMetric{
		Labels: backendIdentityLabels("default", "backend-a"),
		Value:  0,
	})
}

func TestComputeStateRecordsErrorPollMetricsAndRetainsEndpointGauge(t *testing.T) {
	ResetEc2DiscoveryMetrics()

	backend := newEc2Backend("backend-a", "arn:aws:iam::123456789012:role/shared", nil)
	backendIR := backendObjectIR(backend, newTestAWSSecret("aws-creds", "default", "1"))
	c := &ec2EndpointsCollection{
		backends: krt.NewStaticCollection(nil, []ir.BackendObjectIR{backendIR}),
		lister: &fakeEc2InstanceLister{
			instances: []ec2DiscoveredInstance{{instanceID: "i-1", privateIP: "10.0.0.10"}},
		},
	}

	// First poll succeeds and resolves one endpoint.
	state, err := c.computeState(context.Background())
	if err != nil {
		t.Fatalf("computeState() error = %v", err)
	}
	c.state = state

	// Second poll fails: endpoints carry forward (degraded), but the counter must
	// record the underlying classification reason, not the Degraded status reason.
	c.lister = &fakeEc2InstanceLister{
		err: fmt.Errorf("describe instances: %w", &smithy.GenericAPIError{Code: "AuthFailure", Message: "auth failed"}),
	}
	if _, err := c.computeState(context.Background()); err == nil {
		t.Fatal("computeState() error = nil, want error")
	}

	gathered := metricstest.MustGatherMetrics(t)
	gathered.AssertMetricsInclude(pollTotalMetric, []metricstest.ExpectMetric{
		&metricstest.ExpectedMetric{
			Labels: []metrics.Label{
				{Name: "name", Value: "backend-a"},
				{Name: "namespace", Value: "default"},
				{Name: "reason", Value: "AuthorizationError"},
				{Name: "result", Value: "error"},
			},
			Value: 1,
		},
	})
	gathered.AssertMetric(errorStateMetric, &metricstest.ExpectedMetric{
		Labels: backendIdentityLabels("default", "backend-a"),
		Value:  1,
	})
	// The active-endpoint gauge is not updated on a failed poll, so it retains the
	// value from the last successful poll (NFR-3 graceful degradation).
	gathered.AssertMetric(endpointsActiveMetric, &metricstest.ExpectedMetric{
		Labels: backendIdentityLabels("default", "backend-a"),
		Value:  1,
	})
	// A failed poll still has a duration (it can be a slow timeout), so it is observed
	// under result=error. The first (successful) poll is recorded separately under
	// result=success, demonstrating the result partition.
	gathered.AssertMetricsLabels(pollDurationMetric, [][]metrics.Label{
		pollDurationLabels("default", "backend-a", "error"),
		pollDurationLabels("default", "backend-a", "success"),
	})
	gathered.AssertHistogramPopulated(pollDurationMetric)
}

func TestDiscoveryStatusForBackendRecordsErrorStateForUnresolvedSecret(t *testing.T) {
	ResetEc2DiscoveryMetrics()

	// A secret-auth backend whose secret cannot be resolved never enters the poll
	// loop, but must still flip error_state so missing-secret misconfigurations are
	// visible to alerting.
	be := newEc2Backend("backend-a", "", nil)
	be.Spec.Aws.Auth = &kgateway.AwsAuth{
		Type:      kgateway.AwsAuthTypeSecret,
		SecretRef: &corev1.LocalObjectReference{Name: "missing-secret"},
	}
	backend := backendObjectIR(be, nil)

	c := &ec2EndpointsCollection{
		enabled:  true,
		backends: krt.NewStaticCollection(nil, []ir.BackendObjectIR{backend}),
	}
	if got := c.discoveryStatusForBackend(krt.TestingDummyContext{}, backend); got == nil {
		t.Fatal("discoveryStatusForBackend() = nil, want a CredentialError status")
	}

	gathered := metricstest.MustGatherMetrics(t)
	gathered.AssertMetric(errorStateMetric, &metricstest.ExpectedMetric{
		Labels: backendIdentityLabels("default", "backend-a"),
		Value:  1,
	})
	// No endpoints are served while credentials are unresolved.
	gathered.AssertMetric(endpointsActiveMetric, &metricstest.ExpectedMetric{
		Labels: backendIdentityLabels("default", "backend-a"),
		Value:  0,
	})
	// poll_total / poll_duration_seconds stay poll-scoped: no poll occurred, so no
	// per-backend series is recorded for this backend.
	gathered.AssertMetricNotExists(pollDurationMetric)
}

// ec2InstanceListerFunc adapts a function to ec2InstanceLister so a test can act
// while a poll is in flight.
type ec2InstanceListerFunc func(ctx context.Context, source ec2CredentialSource) ([]ec2DiscoveredInstance, error)

func (f ec2InstanceListerFunc) ListInstances(ctx context.Context, source ec2CredentialSource) ([]ec2DiscoveredInstance, error) {
	return f(ctx, source)
}

func TestComputeStateSkipsMetricsForBackendDeletedDuringPoll(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "successful poll"},
		{name: "failed poll", err: fmt.Errorf("describe instances: %w", &smithy.GenericAPIError{Code: "AuthFailure", Message: "auth failed"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ResetEc2DiscoveryMetrics()

			secret := newTestAWSSecret("aws-creds", "default", "1")
			deleted := backendObjectIR(newEc2Backend("backend-a", "arn:aws:iam::123456789012:role/shared", nil), secret)
			kept := backendObjectIR(newEc2Backend("backend-b", "arn:aws:iam::123456789012:role/shared", nil), secret)
			backends := krt.NewMutableCollection(nil, []ir.BackendObjectIR{deleted, kept})
			c := &ec2EndpointsCollection{
				backends: backends.AsCollection(),
				// The Backend was listed by this poll, and both its removal and its
				// one-shot metrics cleanup happen before AWS answers.
				lister: ec2InstanceListerFunc(func(context.Context, ec2CredentialSource) ([]ec2DiscoveredInstance, error) {
					backends.DeleteObject(deleted.ResourceName())
					deleteEc2DiscoveryMetrics("default", "backend-a")
					return []ec2DiscoveredInstance{{instanceID: "i-1", privateIP: "10.0.0.10"}}, tc.err
				}),
			}

			_, _ = c.computeState(context.Background())

			// Only the surviving Backend is recorded; nothing would ever remove
			// series written for the deleted one.
			gathered := metricstest.MustGatherMetrics(t)
			gathered.AssertMetricsLabels(errorStateMetric, [][]metrics.Label{
				backendIdentityLabels("default", "backend-b"),
			})
			result := ec2PollResultSuccess
			if tc.err != nil {
				result = ec2PollResultError
			}
			gathered.AssertMetricsLabels(pollDurationMetric, [][]metrics.Label{
				pollDurationLabels("default", "backend-b", result),
			})
		})
	}
}

func TestDiscoveryStatusForBackendSkipsMetricsForDeletedBackend(t *testing.T) {
	ResetEc2DiscoveryMetrics()

	be := newEc2Backend("backend-a", "", nil)
	be.Spec.Aws.Auth = &kgateway.AwsAuth{
		Type:      kgateway.AwsAuthTypeSecret,
		SecretRef: &corev1.LocalObjectReference{Name: "missing-secret"},
	}

	// The transform is still running for a Backend that has already left the collection.
	c := &ec2EndpointsCollection{
		enabled:  true,
		backends: krt.NewStaticCollection[ir.BackendObjectIR](nil, nil),
	}
	if got := c.discoveryStatusForBackend(krt.TestingDummyContext{}, backendObjectIR(be, nil)); got == nil {
		t.Fatal("discoveryStatusForBackend() = nil, want a CredentialError status")
	}

	gathered := metricstest.MustGatherMetrics(t)
	gathered.AssertMetricNotExists(errorStateMetric)
	gathered.AssertMetricNotExists(endpointsActiveMetric)
}

func TestDeleteEc2DiscoveryMetricsRemovesBackendSeries(t *testing.T) {
	ResetEc2DiscoveryMetrics()

	recordEc2PollSuccess("default", "backend-a", 3)
	recordEc2PollDuration("default", "backend-a", ec2PollResultSuccess, 0.2)
	recordEc2PollSuccess("default", "backend-b", 5)
	recordEc2PollDuration("default", "backend-b", ec2PollResultSuccess, 0.3)

	deleteEc2DiscoveryMetrics("default", "backend-a")

	gathered := metricstest.MustGatherMetrics(t)
	// Only the surviving Backend's series remain; the deleted Backend leaves no
	// stale per-Backend gauge or histogram behind.
	gathered.AssertMetricsLabels(endpointsActiveMetric, [][]metrics.Label{
		backendIdentityLabels("default", "backend-b"),
	})
	gathered.AssertMetricsLabels(errorStateMetric, [][]metrics.Label{
		backendIdentityLabels("default", "backend-b"),
	})
	gathered.AssertMetricsLabels(pollDurationMetric, [][]metrics.Label{
		pollDurationLabels("default", "backend-b", "success"),
	})
}
