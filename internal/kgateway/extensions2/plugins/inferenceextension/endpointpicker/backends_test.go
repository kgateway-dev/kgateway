package endpointpicker

import (
	"context"
	"fmt"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	structpb "google.golang.org/protobuf/types/known/structpb"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

func makeBackendIR(pool *inf.InferencePool) *ir.BackendObjectIR {
	src := ir.ObjectSource{
		Group:     inf.GroupVersion.Group,
		Kind:      wellknown.InferencePoolKind,
		Namespace: pool.Namespace,
		Name:      pool.Name,
	}
	be := ir.NewBackendObjectIR(src, int32(pool.Spec.TargetPorts[0].Number), "")
	be.Obj = pool

	// Wrap the same pool in our internal IR so we can inject errors
	irp := newInferencePool(pool)
	be.ObjIr = irp

	return &be
}

func TestProcessPoolBackendObjIR_BuildsLoadAssignment(t *testing.T) {
	pool := &inf.InferencePool{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Spec: inf.InferencePoolSpec{
			Selector: inf.LabelSelector{
				MatchLabels: map[inf.LabelKey]inf.LabelValue{"app": "test"},
			},
			TargetPorts: []inf.Port{inf.Port{Number: 9000}},
			EndpointPickerRef: inf.EndpointPickerRef{
				Name: "svc",
				Port: &inf.Port{Number: inf.PortNumber(9002)},
			},
		},
	}

	// Build a fake Pod and wrap it into a LocalityPod
	corePod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod1",
			Namespace: "ns",
			Labels:    map[string]string{"app": "test"},
		},
		Status: corev1.PodStatus{PodIP: "10.0.0.1"},
	}
	fakeLP := krtcollections.LocalityPod{
		Named:           krt.NewNamed(corePod),
		AugmentedLabels: corePod.Labels,
		Addresses:       []string{corePod.Status.PodIP},
	}

	// Create a mock and with the LocalityPod collection
	mock := krttest.NewMock(t, []any{fakeLP})
	podCol := krttest.GetMockCollection[krtcollections.LocalityPod](mock)

	// Index the pods
	poolKey := fmt.Sprintf("%s/%s", pool.Namespace, pool.Name)
	podIdx := krtpkg.UnnamedIndex(podCol, func(p krtcollections.LocalityPod) []string {
		return []string{poolKey}
	})

	// Call the code under test
	cluster := &envoyclusterv3.Cluster{}
	ret := processPoolBackendObjIR(context.Background(), *makeBackendIR(pool), cluster, podIdx)
	assert.Nil(t, ret, "Should return nil for a static cluster")

	// Validate the generated LoadAssignment
	la := cluster.LoadAssignment
	require.NotNil(t, la, "LoadAssignment must be set")
	assert.Equal(t, cluster.Name, la.ClusterName)
	require.Len(t, la.Endpoints, 1, "Should have exactly one LocalityLbEndpoints")
	lbs := la.Endpoints[0].LbEndpoints
	require.Len(t, lbs, 1, "Should have exactly one LbEndpoint")

	// Check socket address
	sa := lbs[0].GetEndpoint().Address.GetSocketAddress()
	assert.Equal(t, "10.0.0.1", sa.Address)
	assert.Equal(t, uint32(9000), sa.GetPortValue())

	// Check the subset metadata key
	md := lbs[0].Metadata.FilterMetadata[envoyLbNamespace]
	val := md.Fields[dstEndpointKey]
	expected := structpb.NewStringValue("10.0.0.1:9000")
	assert.Equal(t, expected.GetStringValue(), val.GetStringValue())
}

func TestProcessPoolBackendObjIR_SkipsOnErrors(t *testing.T) {
	pool := &inf.InferencePool{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Spec: inf.InferencePoolSpec{
			TargetPorts: []inf.Port{inf.Port{Number: 9000}},
			EndpointPickerRef: inf.EndpointPickerRef{
				Name: "svc",
				Port: &inf.Port{Number: inf.PortNumber(9002)},
			},
		},
	}
	beIR := makeBackendIR(pool)
	// Inject an error
	irp := beIR.ObjIr.(*inferencePool)
	irp.setErrors([]error{fmt.Errorf("failure injected")})

	// Empty pod index
	mock := krttest.NewMock(t, []any{})
	podCol := krttest.GetMockCollection[krtcollections.LocalityPod](mock)
	podIdx := krtpkg.UnnamedIndex(podCol, func(krtcollections.LocalityPod) []string { return nil })

	cluster := &envoyclusterv3.Cluster{}
	ret := processPoolBackendObjIR(context.Background(), *beIR, cluster, podIdx)
	assert.Nil(t, ret)

	cla := cluster.LoadAssignment
	require.NotNil(t, cla, "LoadAssignment must still be set on error")
	// We get exactly one empty LocalityLbEndpoints on errors
	require.Len(t, cla.Endpoints, 1)
	assert.Empty(t, cla.Endpoints[0].LbEndpoints)
}
