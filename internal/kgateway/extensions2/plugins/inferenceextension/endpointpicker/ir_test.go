package endpointpicker

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

func makePool(opts ...func(*inf.InferencePool)) *inferencePool {
	base := &inf.InferencePool{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "ns",
			Name:              "p",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
		},
		Spec: inf.InferencePoolSpec{
			Selector: inf.LabelSelector{
				MatchLabels: map[inf.LabelKey]inf.LabelValue{"app": "t"},
			},
			TargetPorts: []inf.Port{{Number: 8080}},
			EndpointPickerRef: inf.EndpointPickerRef{
				Name:       "svc",
				PortNumber: ptr.To(inf.PortNumber(1234)),
				FailureMode: func() inf.EndpointPickerFailureMode {
					m := inf.EndpointPickerFailClose
					return m
				}(),
			},
		},
	}

	for _, o := range opts {
		o(base)
	}
	return newInferencePool(base)
}

func TestNewInferencePool_DefaultAndOverridePort(t *testing.T) {
	// Set the default grpcPort
	p := makePool(func(pool *inf.InferencePool) {
		pool.Spec.EndpointPickerRef.PortNumber = ptr.To(inf.PortNumber(grpcPort))
	})

	// We should have exactly one port (grpcPort 9002)
	assert.Len(t, p.configRef.ports, 1)
	assert.Equal(t, int32(grpcPort), p.configRef.ports[0].portNum)

	// Now override the port number
	q := makePool()
	assert.Len(t, q.configRef.ports, 1)
	assert.Equal(t, int32(1234), q.configRef.ports[0].portNum)
}

func TestIsFailOpen(t *testing.T) {
	// The default FailureMode (closed)
	p := makePool()
	assert.False(t, p.failOpen)

	// A nil pool must be false
	assert.False(t, isFailOpen(nil))

	// Set FailureMode to FailOpen
	r := makePool(func(pool *inf.InferencePool) {
		pool.Spec.EndpointPickerRef.FailureMode = inf.EndpointPickerFailOpen
	})
	assert.True(t, r.failOpen)
}

func TestResolvePoolEndpoints_Indexing(t *testing.T) {
	// Create the pool IR
	poolIR := makePool()

	// Create the LocalityPod collection
	p1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "pod1", Labels: map[string]string{"app": "t"}},
		Status:     corev1.PodStatus{PodIP: "10.0.0.1"},
	}
	lp := krtcollections.LocalityPod{Named: krt.NewNamed(p1), AugmentedLabels: p1.Labels, Addresses: []string{p1.Status.PodIP}}
	mock := krttest.NewMock(t, []any{lp})
	col := krttest.GetMockCollection[krtcollections.LocalityPod](mock)

	// Create the LocalityPod index
	key := fmt.Sprintf("ns/p")
	idx := krtpkg.UnnamedIndex(col, func(p krtcollections.LocalityPod) []string {
		return []string{key}
	})

	// Call the code under test and assert the results
	eps := poolIR.resolvePoolEndpoints(idx)
	assert.Len(t, eps, 1)
	assert.Equal(t, "10.0.0.1", eps[0].address)
	assert.Equal(t, int32(8080), eps[0].port)
}

func TestEqualsAndEndpoints(t *testing.T) {
	// Create the pool IRs
	a := makePool()
	b := makePool()

	// Initially equal
	assert.True(t, a.Equals(b))

	// Different selector
	c := makePool(func(pool *inf.InferencePool) {
		pool.Spec.Selector.MatchLabels = map[inf.LabelKey]inf.LabelValue{"app": "x"}
	})
	assert.False(t, a.Equals(c))
	assert.False(t, c.Equals(a))

	// Different targetPort
	d := makePool(func(pool *inf.InferencePool) {
		pool.Spec.TargetPorts[0].Number = 9999
	})
	assert.False(t, a.Equals(d))

	// Equal and different endpoints
	a.setEndpoints([]endpoint{{address: "1.1.1.1", port: 80}})
	b.setEndpoints([]endpoint{{address: "1.1.1.1", port: 80}})
	assert.True(t, a.Equals(b))
	b.setEndpoints([]endpoint{{address: "2.2.2.2", port: 80}})
	assert.False(t, a.Equals(b))

	// Failure mode difference
	e := makePool(func(pool *inf.InferencePool) {
		pool.Spec.EndpointPickerRef.FailureMode = inf.EndpointPickerFailOpen
	})
	assert.False(t, a.Equals(e))
}

func TestErrorHandling(t *testing.T) {
	// Create the pool IR
	p := makePool()
	assert.False(t, p.hasErrors())

	// Set and snapshot
	errList := []error{fmt.Errorf("e1"), fmt.Errorf("e2")}
	p.setErrors(errList)
	snap := p.snapshotErrors()
	assert.Len(t, snap, 2)
	assert.True(t, p.hasErrors())

	// Modifying the snapshot does not affect internal
	snap[0] = fmt.Errorf("changed")
	snap2 := p.snapshotErrors()
	assert.Equal(t, "e1", snap2[0].Error())
}
