package serviceentry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	networking "istio.io/api/networking/v1alpha3"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
)

// TestEndpointsFromWorkloads_SkipsNotReadyPods verifies that endpointsFromWorkloads
// excludes pod-backed workloads that are NotReady (mirroring the EndpointSlice/Service
// path) while keeping Ready pods and WorkloadEntry/inline endpoints (which are always
// treated as Ready). This is the behavior that lets traffic to a ServiceEntry cluster
// with workloadSelector-backed pod endpoints fail over to a peer cluster when all
// locally selected pods go NotReady.
func TestEndpointsFromWorkloads_SkipsNotReadyPods(t *testing.T) {
	se := &networkingclient.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{Name: "autogen.server.server", Namespace: "istio-system"},
		Spec: networking.ServiceEntry{
			Hosts:      []string{"server.server.mesh.internal"},
			Location:   networking.ServiceEntry_MESH_INTERNAL,
			Resolution: networking.ServiceEntry_STATIC,
			Ports: []*networking.ServicePort{
				{Name: "http", Number: 80, Protocol: "HTTP", TargetPort: 9898},
			},
		},
	}
	be := BuildServiceEntryBackendObjectIR(se, "server.server.mesh.internal", 80, "HTTP", nil)

	podWorkload := func(name, ip string, ready bool) selectedWorkload {
		return selectedWorkload{
			LocalityPod: krtcollections.LocalityPod{
				Named:           krt.Named{Name: name, Namespace: "server"},
				Addresses:       []string{ip},
				AugmentedLabels: map[string]string{"app": "server"},
				Ready:           ready,
			},
		}
	}

	// A WorkloadEntry-backed workload (e.g. a cross-cluster endpoint). These have no
	// pod readiness concept and selectedWorkloadFromEntry marks them Ready=true.
	weWorkload := selectedWorkloadFromEntry(
		"cross-cluster", "server",
		nil,
		&networking.WorkloadEntry{Address: "10.0.0.3"},
		nil,
	)
	assert.True(t, weWorkload.Ready, "WorkloadEntry-backed workloads must be treated as Ready")

	workloads := []selectedWorkload{
		podWorkload("ready-pod", "10.0.0.1", true),
		podWorkload("notready-pod", "10.0.0.2", false),
		weWorkload,
	}

	eps := endpointsFromWorkloads(se, be, workloads)
	assert.NotNil(t, eps)

	got := map[string]bool{}
	for _, lbeps := range eps.LbEps {
		for _, emd := range lbeps {
			addr := emd.GetEndpoint().GetAddress().GetSocketAddress().GetAddress()
			got[addr] = true
		}
	}

	assert.True(t, got["10.0.0.1"], "Ready pod endpoint should be included")
	assert.True(t, got["10.0.0.3"], "WorkloadEntry endpoint should be included")
	assert.False(t, got["10.0.0.2"], "NotReady pod endpoint should be excluded")
	assert.Len(t, got, 2, "exactly the Ready pod and the WorkloadEntry endpoint should remain")
}

// TestEndpointsFromWorkloads_SkipsTerminatingPods verifies that endpointsFromWorkloads
// excludes pod-backed workloads that are Terminating even though they are still Ready
// (a terminating pod keeps its Ready condition True through its grace period), while
// keeping Ready non-terminating pods and WorkloadEntry/inline endpoints (which are
// always treated as Ready and never terminating). This is the behavior that lets
// ingress drain a terminating local pod and fail over to a peer cluster, matching the
// EndpointSlice/Service path and ztunnel.
func TestEndpointsFromWorkloads_SkipsTerminatingPods(t *testing.T) {
	se := &networkingclient.ServiceEntry{
		ObjectMeta: metav1.ObjectMeta{Name: "autogen.server.server", Namespace: "istio-system"},
		Spec: networking.ServiceEntry{
			Hosts:      []string{"server.server.mesh.internal"},
			Location:   networking.ServiceEntry_MESH_INTERNAL,
			Resolution: networking.ServiceEntry_STATIC,
			Ports: []*networking.ServicePort{
				{Name: "http", Number: 80, Protocol: "HTTP", TargetPort: 9898},
			},
		},
	}
	be := BuildServiceEntryBackendObjectIR(se, "server.server.mesh.internal", 80, "HTTP", nil)

	podWorkload := func(name, ip string, ready, terminating bool) selectedWorkload {
		return selectedWorkload{
			LocalityPod: krtcollections.LocalityPod{
				Named:           krt.Named{Name: name, Namespace: "server"},
				Addresses:       []string{ip},
				AugmentedLabels: map[string]string{"app": "server"},
				Ready:           ready,
				Terminating:     terminating,
			},
		}
	}

	// A WorkloadEntry-backed workload (e.g. a cross-cluster endpoint). These have no
	// pod readiness/termination concept; selectedWorkloadFromEntry marks them Ready=true
	// and Terminating=false, so they must never be dropped by local pod state.
	weWorkload := selectedWorkloadFromEntry(
		"cross-cluster", "server",
		nil,
		&networking.WorkloadEntry{Address: "10.0.0.3"},
		nil,
	)
	assert.True(t, weWorkload.Ready, "WorkloadEntry-backed workloads must be treated as Ready")
	assert.False(t, weWorkload.Terminating, "WorkloadEntry-backed workloads must never be Terminating")

	workloads := []selectedWorkload{
		podWorkload("ready-pod", "10.0.0.1", true, false),
		// Ready=true but Terminating=true: the terminating-pod case.
		podWorkload("terminating-pod", "10.0.0.2", true, true),
		weWorkload,
	}

	eps := endpointsFromWorkloads(se, be, workloads)
	assert.NotNil(t, eps)

	got := map[string]bool{}
	for _, lbeps := range eps.LbEps {
		for _, emd := range lbeps {
			addr := emd.GetEndpoint().GetAddress().GetSocketAddress().GetAddress()
			got[addr] = true
		}
	}

	assert.True(t, got["10.0.0.1"], "Ready non-terminating pod endpoint should be included")
	assert.True(t, got["10.0.0.3"], "WorkloadEntry endpoint should be included")
	assert.False(t, got["10.0.0.2"], "Terminating pod endpoint should be excluded even though Ready=true")
	assert.Len(t, got, 2, "exactly the Ready pod and the WorkloadEntry endpoint should remain")
}
