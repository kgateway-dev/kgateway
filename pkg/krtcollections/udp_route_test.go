package krtcollections

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// A backend with inline endpoints in a weighted UDPRoute rule is rejected and
// keeps its weight, which the rule's aggregate cluster drops to its blackhole.
func TestRejectUnweightableBackends(t *testing.T) {
	service := ir.NewBackendObjectIR(ir.ObjectSource{Kind: "Service", Namespace: "ns", Name: "svc"}, 8080, "", "kube")
	static := ir.NewBackendObjectIR(ir.ObjectSource{Group: wellknown.BackendGVK.Group, Kind: "Backend", Namespace: "ns", Name: "static"}, 0, "", "backend")
	static.InlineEndpoints = true
	backends := []ir.BackendRefIR{
		{BackendObject: &service, ClusterName: service.ClusterName(), Weight: 80},
		{BackendObject: &static, ClusterName: static.ClusterName(), Weight: 20},
	}

	rejectUnweightableBackends(ir.ObjectSource{Group: gwv1.GroupVersion.Group, Kind: "UDPRoute", Namespace: "ns", Name: "route"}, backends)

	assert.Same(t, &service, backends[0].BackendObject, "a backend with endpoint rows is kept")
	assert.NoError(t, backends[0].Err)
	assert.Nil(t, backends[1].BackendObject)
	assert.Equal(t, wellknown.BlackholeClusterName, backends[1].ClusterName)
	assert.Equal(t, uint32(20), backends[1].Weight, "the weight stays so the aggregate drops it")
	var unweightable *UnweightableBackendError
	require.ErrorAs(t, backends[1].Err, &unweightable)
	assert.Equal(t, "Backend ns/static carries its endpoints in its own cluster, so it can only be the sole backendRef of a UDPRoute rule",
		unweightable.Error())
}
