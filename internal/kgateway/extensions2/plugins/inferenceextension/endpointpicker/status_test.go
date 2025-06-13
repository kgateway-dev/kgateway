package endpointpicker

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	infextv1a2 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
)

func TestAddAndFindGatewayParentRef(t *testing.T) {
	status := &infextv1a2.InferencePoolStatus{}
	// initially empty
	assert.Equal(t, -1, findGatewayParentRef(status, "gw1"))

	addGatewayParentRef(status, "gw1")
	// now present at index 0
	assert.Equal(t, 0, findGatewayParentRef(status, "gw1"))
	assert.Len(t, status.Parents, 1)

	// adding again should not duplicate
	addGatewayParentRef(status, "gw1")
	assert.Len(t, status.Parents, 1)
}

func TestBuildAcceptedCondition(t *testing.T) {
	cond := buildAcceptedCondition(42, "ctrl")
	assert.Equal(t, string(infextv1a2.InferencePoolConditionAccepted), cond.Type)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, string(infextv1a2.InferencePoolReasonAccepted), cond.Reason)
	assert.Contains(t, cond.Message, "controller ctrl")
	assert.Equal(t, int64(42), cond.ObservedGeneration)
}

func TestBuildResolvedRefsCondition_NoErrors(t *testing.T) {
	cond := buildResolvedRefsCondition(7, nil)
	assert.Equal(t, string(infextv1a2.InferencePoolConditionResolvedRefs), cond.Type)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, string(infextv1a2.InferencePoolReasonResolvedRefs), cond.Reason)
	assert.Equal(t, "All InferencePool references have been resolved", cond.Message)
	assert.Equal(t, int64(7), cond.ObservedGeneration)
}

func TestBuildResolvedRefsCondition_WithErrors(t *testing.T) {
	err1 := errors.New("foo failed")
	err2 := errors.New("bar crash")

	cond1 := buildResolvedRefsCondition(1, []error{err1})
	assert.Equal(t, metav1.ConditionFalse, cond1.Status)
	assert.Equal(t, string(infextv1a2.InferencePoolReasonInvalidExtensionRef), cond1.Reason)
	// single-error prefix is now just "error:"
	assert.Contains(t, cond1.Message, "error:")
	// no embedded quotes
	assert.Contains(t, cond1.Message, "foo failed")

	cond2 := buildResolvedRefsCondition(2, []error{err1, err2})
	assert.Equal(t, metav1.ConditionFalse, cond2.Status)
	assert.Equal(t, string(infextv1a2.InferencePoolReasonInvalidExtensionRef), cond2.Reason)
	assert.Contains(t, cond2.Message, "InferencePool has 2 errors:")
	// joined with "; "
	assert.Contains(t, cond2.Message, "foo failed; bar crash")
}

func TestReferencesInferencePool(t *testing.T) {
	p := &inferencePool{}
	ctx := context.Background()
	commonCol := &common.CommonCollections{ControllerName: "ctrl"}
	poolNN := types.NamespacedName{Namespace: "ns", Name: "pool1"}

	// No routes -> return empty
	p.errors = nil
	rs := p.referencedGateway(ctx, commonCol, nil, poolNN)
	assert.Equal(t, "", rs)
	assert.Len(t, p.errors, 0)

	// Route with wrong SourceObject type
	p.errors = nil
	wrong := &corev1.Service{}
	routes := []ir.HttpRouteIR{{SourceObject: wrong}}
	rs = p.referencedGateway(ctx, commonCol, routes, poolNN)
	assert.Equal(t, "", rs)
	assert.Len(t, p.errors, 1)

	// Route with no matching backend
	p.errors = nil
	r := &gwv1.HTTPRoute{}
	r.Spec.Rules = []gwv1.HTTPRouteRule{{}}
	routes = []ir.HttpRouteIR{{SourceObject: r}}
	rs = p.referencedGateway(ctx, commonCol, routes, poolNN)
	assert.Equal(t, "", rs)

	// Route with matching backend but no parent with controllerName
	p.errors = nil
	r = &gwv1.HTTPRoute{}
	r.Spec.Rules = []gwv1.HTTPRouteRule{{
		BackendRefs: []gwv1.HTTPBackendRef{{
			BackendRef: gwv1.BackendRef{BackendObjectReference: gwv1.BackendObjectReference{
				Group: ptr.To(gwv1.Group(infextv1a2.GroupVersion.Group)),
				Kind:  ptr.To(gwv1.Kind(wellknown.InferencePoolKind)),
				Name:  gwv1.ObjectName(poolNN.Name),
			}},
		}},
	}}
	r.Status.Parents = []gwv1.RouteParentStatus{{
		ParentRef:      gwv1.ParentReference{Name: "gw1"},
		ControllerName: gwv1.GatewayController("other"),
	}}
	routes = []ir.HttpRouteIR{{SourceObject: r}}
	rs = p.referencedGateway(ctx, commonCol, routes, poolNN)
	assert.Equal(t, "", rs)

	// Route with matching backend and matching parent
	p.errors = nil
	r.Status.Parents = []gwv1.RouteParentStatus{{
		ParentRef:      gwv1.ParentReference{Name: "gw-ok"},
		ControllerName: gwv1.GatewayController("ctrl"),
	}}
	routes = []ir.HttpRouteIR{{SourceObject: r}}
	rs = p.referencedGateway(ctx, commonCol, routes, poolNN)
	assert.Equal(t, "gw-ok", rs)
}

func TestRemoveGatewayParentRef(t *testing.T) {
	ctx := context.Background()
	nn := types.NamespacedName{Name: "pool1", Namespace: "ns1"}

	// Setup a pool with two parents: one matching gateway, one not
	pool := &infextv1a2.InferencePool{
		ObjectMeta: metav1.ObjectMeta{Name: nn.Name, Namespace: nn.Namespace},
		Status: infextv1a2.InferencePoolStatus{Parents: []infextv1a2.PoolStatus{
			{GatewayRef: corev1.ObjectReference{Name: "kgtw"}},
			{GatewayRef: corev1.ObjectReference{Name: "gw-other"}},
		}},
	}

	// Setup the scheme and client with initial inferencepool
	scheme := schemes.DefaultScheme()
	err := infextv1a2.AddToScheme(scheme)
	assert.NoError(t, err, "Failed to add InferencePool scheme")
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(pool).WithRuntimeObjects(pool).Build()

	// Use a fake krt.Collection[ir.Gateway]
	ig := ir.Gateway{ObjectSource: ir.ObjectSource{Name: "kgtw", Namespace: "ns1"}}
	fakeGwCol := fakeGatewayCollection{items: []ir.Gateway{ig}}
	idx := &krtcollections.GatewayIndex{Gateways: fakeGwCol}

	// Remove the matching gateway parent reference
	err = removeGatewayParentRef(ctx, fakeClient, pool, idx)
	assert.NoError(t, err)

	// Read back the pool to ensure status update
	updated := &infextv1a2.InferencePool{}
	err = fakeClient.Get(ctx, nn, updated)
	assert.NoError(t, err)

	// Expect one parent removed
	expected := []infextv1a2.PoolStatus{{GatewayRef: corev1.ObjectReference{Name: "gw-other"}}}
	assert.Equal(t, expected, updated.Status.Parents)

	// Case: no matching gateway -> no status change
	pool2 := &infextv1a2.InferencePool{
		ObjectMeta: metav1.ObjectMeta{Name: nn.Name, Namespace: nn.Namespace},
		Status:     infextv1a2.InferencePoolStatus{Parents: []infextv1a2.PoolStatus{{GatewayRef: corev1.ObjectReference{Name: "gw-none"}}}},
	}
	fakeClient2 := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(pool2).WithRuntimeObjects(pool2).Build()
	err = removeGatewayParentRef(ctx, fakeClient2, pool2, idx)
	assert.NoError(t, err)

	unchanged := &infextv1a2.InferencePool{}
	err = fakeClient2.Get(ctx, nn, unchanged)
	assert.NoError(t, err)
	assert.Equal(t, pool2.Status.Parents, unchanged.Status.Parents)

	// Case: nil pool should return error
	err = removeGatewayParentRef(ctx, fakeClient2, nil, idx)
	assert.Error(t, err)

	// Case: empty parents
	pool3 := &infextv1a2.InferencePool{ObjectMeta: metav1.ObjectMeta{Name: nn.Name, Namespace: nn.Namespace}}
	fakeClient3 := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(pool3).WithRuntimeObjects(pool3).Build()
	err = removeGatewayParentRef(ctx, fakeClient3, pool3, idx)
	assert.NoError(t, err)

	// Case: no gateways in index
	idxEmpty := &krtcollections.GatewayIndex{Gateways: fakeGatewayCollection{items: nil}}
	err = removeGatewayParentRef(ctx, fakeClient, pool, idxEmpty)
	assert.Error(t, err)
}

// fakeGatewayCollection remains for gateway index listing
type fakeGatewayCollection struct {
	items []ir.Gateway
}

func (f fakeGatewayCollection) List() []ir.Gateway                                      { return f.items }
func (f fakeGatewayCollection) GetKey(_ string) *ir.Gateway                             { return nil }
func (f fakeGatewayCollection) HasSynced() bool                                         { return true }
func (f fakeGatewayCollection) WaitUntilSynced(stop <-chan struct{}) bool               { return true }
func (f fakeGatewayCollection) Register(handler func(krt.Event[ir.Gateway])) krt.Syncer { return f }
func (f fakeGatewayCollection) RegisterBatch(handler func([]krt.Event[ir.Gateway], bool), _ bool) krt.Syncer {
	return f
}
