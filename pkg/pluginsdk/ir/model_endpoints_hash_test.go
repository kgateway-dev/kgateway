package ir

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

func backendWithLabelsForHashTest(labels map[string]string) BackendObjectIR {
	backend := NewBackendObjectIR(ObjectSource{
		Kind:      "Service",
		Namespace: "default",
		Name:      "my-service",
	}, 8080, "", "")
	backend.Obj = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "my-service",
			Namespace:       "default",
			UID:             "svc-uid-1",
			ResourceVersion: "1",
			Labels:          labels,
		},
	}
	backend.CanonicalHostname = "my-service.default.svc.cluster.local"
	backend.TrafficDistribution = wellknown.TrafficDistributionAny
	return backend
}

// TestEndpointsForBackendEqualityHashIsDeterministic is a regression test: the
// backend labels used to be written into a single running FNV hasher in Go map
// iteration order, so two identical backends hashed differently. Because that
// hash flows into LbEpsEqualityHash (and from there into the per-client EDS
// hash), every recomputation of the endpoints collection looked like a change to
// KRT and pushed EDS again.
func TestEndpointsForBackendEqualityHashIsDeterministic(t *testing.T) {
	labels := map[string]string{
		"a": "1", "b": "2", "c": "3", "d": "4", "e": "5",
		"f": "6", "g": "7", "h": "8", "i": "9", "j": "10",
	}

	first := NewEndpointsForBackend(backendWithLabelsForHashTest(labels))
	for range 100 {
		next := NewEndpointsForBackend(backendWithLabelsForHashTest(labels))
		if !first.Equals(*next) {
			t.Fatalf("two identical EndpointsForBackend compared unequal: hash %d vs %d",
				first.LbEpsEqualityHash, next.LbEpsEqualityHash)
		}
	}
}

// TestEndpointsForBackendEqualityHashCoversLabels pins the other half of the
// contract: BackendLabels stays out of Equals only because it is folded into the
// hash, so a label change must still be observed.
func TestEndpointsForBackendEqualityHashCoversLabels(t *testing.T) {
	base := NewEndpointsForBackend(backendWithLabelsForHashTest(map[string]string{
		"app": "my-app", "version": "v1",
	}))
	relabelled := NewEndpointsForBackend(backendWithLabelsForHashTest(map[string]string{
		"app": "my-app", "version": "v2",
	}))

	if base.Equals(*relabelled) {
		t.Error("Equals returned true after a backend label changed; the label hash must move")
	}
}
