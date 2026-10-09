package krtcollections

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/api/label"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

func TestFetchSystemNamespaceNetwork(t *testing.T) {
	assert.Empty(t, FetchSystemNamespaceNetwork(krt.TestingDummyContext{}, nil), "nil singleton")
	assert.Empty(t, FetchSystemNamespaceNetwork(krt.TestingDummyContext{}, krt.NewStatic[string](nil, true)), "unset network")
	assert.Equal(t, "n1", FetchSystemNamespaceNetwork(krt.TestingDummyContext{}, krt.NewStatic(new("n1"), true)))
}

// Like Istio, an EndpointSlice endpoint without a Pod falls back to the system
// namespace network, while a Pod-backed endpoint keeps its pod's labels.
func TestK8sEndpointsWithoutPodGetSystemNamespaceNetwork(t *testing.T) {
	mock := krttest.NewMock(t, []any{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remote",
				Namespace: "ns",
				Labels:    map[string]string{label.TopologyNetwork.Name: "n2"},
			},
			Status: corev1.PodStatus{PodIP: "5.6.7.8"},
		},
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "svc-abcde",
				Namespace: "ns",
				Labels:    map[string]string{discoveryv1.LabelServiceName: "svc"},
			},
			AddressType: discoveryv1.AddressTypeIPv4,
			Endpoints: []discoveryv1.Endpoint{
				{
					// handmade slice: no Pod targetRef
					Addresses:  []string{"1.2.3.4"},
					Conditions: discoveryv1.EndpointConditions{Ready: new(true)},
				},
				{
					Addresses:  []string{"5.6.7.8"},
					Conditions: discoveryv1.EndpointConditions{Ready: new(true)},
					TargetRef:  &corev1.ObjectReference{Kind: "Pod", Name: "remote", Namespace: "ns"},
				},
			},
			Ports: []discoveryv1.EndpointPort{{
				Name:     new("http"),
				Port:     new(int32(8080)),
				Protocol: new(corev1.ProtocolTCP),
			}},
		},
	})
	nodes := NewNodeMetadataCollection(krttest.GetMockCollection[*corev1.Node](mock))
	pods := NewLocalityPodsCollection(nodes, nil, krttest.GetMockCollection[*corev1.Pod](mock), krtutil.KrtOptions{})
	pods.WaitUntilSynced(context.Background().Done())
	endpointSlices := krttest.GetMockCollection[*discoveryv1.EndpointSlice](mock)
	endpointSlicesByService := krtpkg.UnnamedIndex(endpointSlices, func(es *discoveryv1.EndpointSlice) []types.NamespacedName {
		return []types.NamespacedName{{Namespace: es.Namespace, Name: es.Labels[discoveryv1.LabelServiceName]}}
	})
	builder := transformK8sEndpoints(EndpointsInputs{
		EndpointSlices:          endpointSlices,
		EndpointSlicesByService: endpointSlicesByService,
		Pods:                    pods,
		SystemNamespaceNetwork:  krt.NewStatic(new("n1"), true),
	})

	backend := newBackendObjectIR(backendObjectIRInput{
		ObjectSource: ir.ObjectSource{Namespace: "ns", Name: "svc", Kind: "Service"},
		Port:         8080,
		Obj: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns"},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "http", Port: 8080}}},
		},
	})
	eps := builder(krt.TestingDummyContext{}, backend)
	require.NotNil(t, eps)

	networks := map[string]string{}
	for _, group := range eps.LbEps {
		for _, ep := range group {
			networks[ep.GetEndpoint().GetAddress().GetSocketAddress().GetAddress()] = ep.EndpointMd.Labels[label.TopologyNetwork.Name]
		}
	}
	assert.Equal(t, map[string]string{"1.2.3.4": "n1", "5.6.7.8": "n2"}, networks)
}
