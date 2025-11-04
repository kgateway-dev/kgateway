package endpointpicker

import (
	"context"
	"fmt"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	structpb "google.golang.org/protobuf/types/known/structpb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
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
			TargetPorts: []inf.Port{{Number: 9000}},
			EndpointPickerRef: inf.EndpointPickerRef{
				Name: "svc",
				Port: &inf.Port{Number: inf.PortNumber(9002)},
			},
		},
	}

	// Build the Backend IR and seed endpoints
	beIR := makeBackendIR(pool)
	irp := beIR.ObjIr.(*inferencePool)
	irp.setEndpoints([]endpoint{{address: "10.0.0.1", port: 9000}})

	// Call the code under test
	cluster := &envoyclusterv3.Cluster{}
	ret := processPoolBackendObjIR(context.Background(), *beIR, cluster)
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
			TargetPorts: []inf.Port{{Number: 9000}},
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

	cluster := &envoyclusterv3.Cluster{}
	ret := processPoolBackendObjIR(context.Background(), *beIR, cluster)
	assert.Nil(t, ret)

	cla := cluster.LoadAssignment
	require.NotNil(t, cla, "LoadAssignment must still be set on error")
	// We get exactly one empty LocalityLbEndpoints on errors
	require.Len(t, cla.Endpoints, 1)
	assert.Empty(t, cla.Endpoints[0].LbEndpoints)
}
