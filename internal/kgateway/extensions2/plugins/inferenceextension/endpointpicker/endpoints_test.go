package endpointpicker

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	infv1a2 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
)

func TestResolvePoolEndpoints(t *testing.T) {
	pool := &infv1a2.InferencePool{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "p"},
		Spec: infv1a2.InferencePoolSpec{
			Selector:         map[infv1a2.LabelKey]infv1a2.LabelValue{"app": "test"},
			TargetPortNumber: 8080,
		},
	}

	// Build two pods: one matching and one not
	p1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns", Name: "pod1",
			Labels: map[string]string{"app": "test"},
		},
		Status: corev1.PodStatus{PodIP: "1.2.3.4"},
	}
	p2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns", Name: "pod2",
			Labels: map[string]string{"app": "other"},
		},
		Status: corev1.PodStatus{PodIP: "5.6.7.8"},
	}

	// Wrap as LocalityPod
	lp1 := krtcollections.LocalityPod{
		Named:           krt.NewNamed(p1),
		AugmentedLabels: p1.Labels,
		Addresses:       []string{p1.Status.PodIP},
	}
	lp2 := krtcollections.LocalityPod{
		Named:           krt.NewNamed(p2),
		AugmentedLabels: p2.Labels,
		Addresses:       []string{p2.Status.PodIP},
	}

	// Create the Mock and collection
	mock := krttest.NewMock(t, []any{lp1, lp2})
	podCol := krttest.GetMockCollection[krtcollections.LocalityPod](mock)

	// Index pods (only our matching pod)
	key := fmt.Sprintf("%s/%s", pool.Namespace, pool.Name)
	podIdx := krtutil.UnnamedIndex(podCol, func(p krtcollections.LocalityPod) []string {
		if p.Address() == "1.2.3.4" {
			return []string{key}
		}
		return nil
	})

	// Call resolvePoolEndpoints and assert the endpoints
	eps := resolvePoolEndpoints(pool, podIdx)
	assert.Len(t, eps, 1, "only the matching pod should appear")
	assert.Equal(t, "1.2.3.4", eps[0].address)
	assert.Equal(t, int32(8080), eps[0].port)

	// If the index has no pods for that key, the result should be empty
	emptyIdx := krtutil.UnnamedIndex(podCol, func(_ krtcollections.LocalityPod) []string { return nil })
	assert.Empty(t, resolvePoolEndpoints(pool, emptyIdx))
}
