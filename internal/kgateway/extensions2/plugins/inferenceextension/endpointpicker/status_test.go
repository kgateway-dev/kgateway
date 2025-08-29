package endpointpicker

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
)

func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	// Create a new scheme and register the necessary types
	sch := schemes.DefaultScheme()
	require.NoError(t, corev1.AddToScheme(sch))
	require.NoError(t, inf.AddToScheme(sch))
	require.NoError(t, gwv1.Install(sch))

	// Create a fake client with the provided objects
	b := fakeclient.NewClientBuilder().WithScheme(sch)
	b = b.WithObjects(objs...)

	// Register status subresource for the InferencePool type
	b = b.WithStatusSubresource(&inf.InferencePool{})

	return b.Build()
}

func fakeRoutesIndex(col krt.Collection[ir.HttpRouteIR]) *krtcollections.RoutesIndex {
	ri := &krtcollections.RoutesIndex{}

	// Locate the unexported field.
	v := reflect.ValueOf(ri).Elem().FieldByName("httpRoutes")

	// Turn it into an addressable value and replace the contents.
	ptr := unsafe.Pointer(v.UnsafeAddr()) // #nosec G103 – test-only
	reflect.NewAt(v.Type(), ptr).Elem().Set(reflect.ValueOf(col))

	return ri
}

func TestUpdatePoolStatus_NoReferences_NoErrors(t *testing.T) {
	// Set up the context, controller name, namespace, and pool name
	ctx := context.Background()
	controllerName := "test-controller"
	ns := "default"
	poolName := "my-pool"
	poolNN := types.NamespacedName{Namespace: ns, Name: poolName}
	pool := &inf.InferencePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:       poolName,
			Namespace:  ns,
			Generation: 1,
		},
	}

	// Create a fake client with the InferencePool object
	fakeClient := newFakeClient(t, pool)
	mock := krttest.NewMock(t, []any{})
	col := krttest.GetMockCollection[ir.HttpRouteIR](mock)
	commonCol := &common.CommonCollections{
		CrudClient:     fakeClient,
		ControllerName: controllerName,
		Routes:         fakeRoutesIndex(col),
	}
	beIR := ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     inf.GroupVersion.Group,
			Kind:      wellknown.InferencePoolKind,
			Namespace: poolNN.Namespace,
			Name:      poolNN.Name,
		},
		ObjIr: &inferencePool{errors: nil},
	}

	// Call the function to update the pool status
	updatePoolStatus(ctx, commonCol, beIR, "", nil)
	var updated inf.InferencePool
	err := fakeClient.Get(ctx, poolNN, &updated)

	// Assert that there are no errors and the status is updated correctly
	require.NoError(t, err)
	assert.Empty(t, updated.Status.Parents)
}

func TestUpdatePoolStatus_WithReference_NoErrors(t *testing.T) {
	// Set up the context, controller name, namespace, pool name, and gateway name
	ctx := context.Background()
	controllerName := "test-controller"
	ns := "default"
	poolName := "my-pool"
	poolNN := types.NamespacedName{Namespace: ns, Name: poolName}
	gwName := "my-gateway"

	// Create a sample HTTPRoute with a reference to the InferencePool
	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "my-route",
			UID:       "uid1",
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Group: ptr.To(gwv1.Group(gwv1.GroupName)),
						Kind:  ptr.To(gwv1.Kind(wellknown.GatewayKind)),
						Name:  gwv1.ObjectName(gwName),
					},
				},
			},
			Rules: []gwv1.HTTPRouteRule{
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Group: ptr.To(gwv1.Group(inf.GroupVersion.Group)),
									Kind:  ptr.To(gwv1.Kind(wellknown.InferencePoolKind)),
									Name:  gwv1.ObjectName(poolName),
								},
							},
						},
					},
				},
			},
		},
	}
	pool := &inf.InferencePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:       poolName,
			Namespace:  ns,
			Generation: 1,
		},
	}

	// Create a fake client with the InferencePool object
	fakeClient := newFakeClient(t, pool)
	mock := krttest.NewMock(t, []any{
		ir.HttpRouteIR{
			ObjectSource: ir.ObjectSource{
				Group:     gwv1.SchemeGroupVersion.Group,
				Kind:      "HTTPRoute",
				Namespace: ns,
				Name:      "my-route",
			},
			SourceObject: route,
		},
	})

	// Get the mock collection for HTTPRouteIR
	col := krttest.GetMockCollection[ir.HttpRouteIR](mock)
	commonCol := &common.CommonCollections{
		CrudClient:     fakeClient,
		ControllerName: controllerName,
		Routes:         fakeRoutesIndex(col),
	}
	beIR := ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     inf.GroupVersion.Group,
			Kind:      wellknown.InferencePoolKind,
			Namespace: poolNN.Namespace,
			Name:      poolNN.Name,
		},
		ObjIr: &inferencePool{errors: nil},
	}

	// Call the function to update the pool status
	updatePoolStatus(ctx, commonCol, beIR, "", nil)
	var updated inf.InferencePool
	err := fakeClient.Get(ctx, poolNN, &updated)

	// Assert that there are no errors and the status is updated correctly
	require.NoError(t, err)
	require.Len(t, updated.Status.Parents, 1)
	p := updated.Status.Parents[0]
	assert.Equal(t, inf.ParentReference{
		Kind:      inf.Kind(wellknown.GatewayKind),
		Namespace: inf.Namespace(ns),
		Name:      inf.ObjectName(gwName),
	}, p.ParentRef)

	// Check the accepted condition
	accepted := meta.FindStatusCondition(p.Conditions, string(inf.InferencePoolConditionAccepted))
	require.NotNil(t, accepted)
	assert.Equal(t, metav1.ConditionTrue, accepted.Status)
	assert.Equal(t, string(inf.InferencePoolReasonAccepted), accepted.Reason)
	assert.Contains(t, accepted.Message, controllerName)
	assert.Equal(t, int64(1), accepted.ObservedGeneration)
	assert.NotZero(t, accepted.LastTransitionTime)

	// Check the resolved references condition
	resolved := meta.FindStatusCondition(p.Conditions, string(inf.InferencePoolConditionResolvedRefs))
	require.NotNil(t, resolved)
	assert.Equal(t, metav1.ConditionTrue, resolved.Status)
	assert.Equal(t, string(inf.InferencePoolReasonResolvedRefs), resolved.Reason)
	assert.Equal(t, "All InferencePool references have been resolved", resolved.Message)
	assert.Equal(t, int64(1), resolved.ObservedGeneration)
	assert.NotZero(t, resolved.LastTransitionTime)
}

func TestUpdatePoolStatus_WithReference_WithErrors(t *testing.T) {
	// Set up the context, controller name, namespace, pool name, and gateway name
	ctx := context.Background()
	controllerName := "test-controller"
	ns := "default"
	poolName := "my-pool"
	poolNN := types.NamespacedName{Namespace: ns, Name: poolName}
	gwName := "my-gateway"

	// Create a sample HTTPRoute with a reference to the InferencePool
	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "my-route",
			UID:       "uid1",
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Group: ptr.To(gwv1.Group(gwv1.GroupName)),
						Kind:  ptr.To(gwv1.Kind(wellknown.GatewayKind)),
						Name:  gwv1.ObjectName(gwName),
					},
				},
			},
			Rules: []gwv1.HTTPRouteRule{
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Group: ptr.To(gwv1.Group(inf.GroupVersion.Group)),
									Kind:  ptr.To(gwv1.Kind(wellknown.InferencePoolKind)),
									Name:  gwv1.ObjectName(poolName),
								},
							},
						},
					},
				},
			},
		},
	}
	pool := &inf.InferencePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:       poolName,
			Namespace:  ns,
			Generation: 1,
		},
	}

	fakeClient := newFakeClient(t, pool)
	mock := krttest.NewMock(t, []any{
		ir.HttpRouteIR{
			ObjectSource: ir.ObjectSource{
				Group:     gwv1.SchemeGroupVersion.Group,
				Kind:      "HTTPRoute",
				Namespace: ns,
				Name:      "my-route",
			},
			SourceObject: route,
		},
	})

	// Get the mock collection for HTTPRouteIR
	col := krttest.GetMockCollection[ir.HttpRouteIR](mock)
	commonCol := &common.CommonCollections{
		CrudClient:     fakeClient,
		ControllerName: controllerName,
		Routes:         fakeRoutesIndex(col),
	}
	beIR := ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     inf.GroupVersion.Group,
			Kind:      wellknown.InferencePoolKind,
			Namespace: poolNN.Namespace,
			Name:      poolNN.Name,
		},
		ObjIr: &inferencePool{errors: []error{fmt.Errorf("test error")}},
	}

	// Call the function to update the pool status with errors
	updatePoolStatus(ctx, commonCol, beIR, "", nil)
	var updated inf.InferencePool
	err := fakeClient.Get(ctx, poolNN, &updated)

	// Assert that there are no errors and the status is updated correctly
	require.NoError(t, err)
	require.Len(t, updated.Status.Parents, 2)

	// Check the gateway parent status
	var gwParent, defaultParent inf.ParentStatus
	for _, p := range updated.Status.Parents {
		if p.ParentRef.Kind == inf.Kind(wellknown.GatewayKind) {
			gwParent = p
		} else if p.ParentRef.Kind == inf.Kind(defaultInfPoolStatusKind) {
			defaultParent = p
		}
	}
	require.NotZero(t, gwParent)
	assert.Equal(t, inf.ParentReference{
		Kind:      inf.Kind(wellknown.GatewayKind),
		Namespace: inf.Namespace(ns),
		Name:      inf.ObjectName(gwName),
	}, gwParent.ParentRef)
	accepted := meta.FindStatusCondition(gwParent.Conditions, string(inf.InferencePoolConditionAccepted))
	require.NotNil(t, accepted)
	assert.Equal(t, metav1.ConditionTrue, accepted.Status)
	resolved := meta.FindStatusCondition(gwParent.Conditions, string(inf.InferencePoolConditionResolvedRefs))
	require.NotNil(t, resolved)
	assert.Equal(t, metav1.ConditionFalse, resolved.Status)
	assert.Equal(t, string(inf.InferencePoolReasonInvalidExtensionRef), resolved.Reason)
	assert.Equal(t, "error: test error", resolved.Message)

	// Default parent
	require.NotZero(t, defaultParent)
	assert.Equal(t, inf.ParentReference{
		Kind: inf.Kind(defaultInfPoolStatusKind),
		Name: inf.ObjectName(defaultInfPoolStatusName),
	}, defaultParent.ParentRef)
	require.Len(t, defaultParent.Conditions, 1)
	// Check the conditions for the default parent
	resolved = meta.FindStatusCondition(defaultParent.Conditions, string(inf.InferencePoolConditionResolvedRefs))
	require.NotNil(t, resolved)
	assert.Equal(t, metav1.ConditionFalse, resolved.Status)
	assert.Equal(t, string(inf.InferencePoolReasonInvalidExtensionRef), resolved.Reason)
	assert.Equal(t, "error: test error", resolved.Message)
	assert.Nil(t, meta.FindStatusCondition(defaultParent.Conditions, string(inf.InferencePoolConditionAccepted)))
}

func TestUpdatePoolStatus_DeleteRoute(t *testing.T) {
	// Set up the context, controller name, namespace, pool name, and route UID
	ctx := context.Background()
	controllerName := "test-controller"
	ns := "default"
	poolName := "my-pool"
	poolNN := types.NamespacedName{Namespace: ns, Name: poolName}
	gwName := "my-gateway"
	routeUID := types.UID("uid1")

	// Create a sample HTTPRoute with a reference to the InferencePool
	route := &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      "my-route",
			UID:       routeUID,
		},
		Spec: gwv1.HTTPRouteSpec{
			CommonRouteSpec: gwv1.CommonRouteSpec{
				ParentRefs: []gwv1.ParentReference{
					{
						Group: ptr.To(gwv1.Group(gwv1.GroupName)),
						Kind:  ptr.To(gwv1.Kind(wellknown.GatewayKind)),
						Name:  gwv1.ObjectName(gwName),
					},
				},
			},
			Rules: []gwv1.HTTPRouteRule{
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Group: ptr.To(gwv1.Group(inf.GroupVersion.Group)),
									Kind:  ptr.To(gwv1.Kind(wellknown.InferencePoolKind)),
									Name:  gwv1.ObjectName(poolName),
								},
							},
						},
					},
				},
			},
		},
	}
	pool := &inf.InferencePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:       poolName,
			Namespace:  ns,
			Generation: 1,
		},
	}

	// Create a fake client with the InferencePool object
	fakeClient := newFakeClient(t, pool)
	mock := krttest.NewMock(t, []any{
		ir.HttpRouteIR{
			ObjectSource: ir.ObjectSource{
				Group:     gwv1.SchemeGroupVersion.Group,
				Kind:      "HTTPRoute",
				Namespace: ns,
				Name:      "my-route",
			},
			SourceObject: route,
		},
	})

	// Get the mock collection for HTTPRouteIR
	col := krttest.GetMockCollection[ir.HttpRouteIR](mock)
	commonCol := &common.CommonCollections{
		CrudClient:     fakeClient,
		ControllerName: controllerName,
		Routes:         fakeRoutesIndex(col),
	}
	beIR := ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     inf.GroupVersion.Group,
			Kind:      wellknown.InferencePoolKind,
			Namespace: poolNN.Namespace,
			Name:      poolNN.Name,
		},
		ObjIr: &inferencePool{errors: nil},
	}

	// Call the function to update the pool status with the route
	updatePoolStatus(ctx, commonCol, beIR, routeUID, nil)
	var updated inf.InferencePool
	err := fakeClient.Get(ctx, poolNN, &updated)

	// Assert that there are no errors and the status is updated correctly
	require.NoError(t, err)
	assert.Empty(t, updated.Status.Parents)
}

func TestUpdatePoolStatus_WithExtraGws(t *testing.T) {
	// Set up the context, namespace, pool name, and extra gateway name
	ctx := context.Background()
	ns := "default"
	poolName := "my-pool"
	poolNN := types.NamespacedName{Namespace: ns, Name: poolName}
	gwName := "extra-gw"

	// Create a sample InferencePool object
	pool := &inf.InferencePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:       poolName,
			Namespace:  ns,
			Generation: 1,
		},
	}

	// Create a fake client with the InferencePool object
	fakeClient := newFakeClient(t, pool)
	mock := krttest.NewMock(t, []any{}) // no HTTPRouteIRs
	col := krttest.GetMockCollection[ir.HttpRouteIR](mock)

	// Create a CommonCollections instance with the fake client and routes index
	commonCol := &common.CommonCollections{
		CrudClient:     fakeClient,
		ControllerName: "test-controller",
		Routes:         fakeRoutesIndex(col),
	}
	beIR := ir.BackendObjectIR{
		ObjectSource: ir.ObjectSource{
			Group:     inf.GroupVersion.Group,
			Kind:      wellknown.InferencePoolKind,
			Namespace: ns,
			Name:      poolName,
		},
		ObjIr: &inferencePool{errors: nil},
	}

	// Simulate controller knowing about a parent Gateway even if no HTTPRoute is present
	extraGws := map[types.NamespacedName]struct{}{
		{Namespace: ns, Name: gwName}: {},
	}

	// Call the function to update the pool status with the extra gateways
	updatePoolStatus(ctx, commonCol, beIR, "", extraGws)

	// Assert that the InferencePool status is updated correctly
	var updated inf.InferencePool
	err := fakeClient.Get(ctx, poolNN, &updated)
	require.NoError(t, err)
	require.Len(t, updated.Status.Parents, 1)

	assert.Equal(t, inf.ParentReference{
		Kind:      inf.Kind(wellknown.GatewayKind),
		Namespace: inf.Namespace(ns),
		Name:      inf.ObjectName(gwName),
	}, updated.Status.Parents[0].ParentRef)
}

func TestReferencedGateways(t *testing.T) {
	// Set up the test with a namespace, pool name, and two gateways in different namespaces
	ns := "default"
	poolNN := types.NamespacedName{Namespace: ns, Name: "my-pool"}
	gw1 := types.NamespacedName{Namespace: ns, Name: "gw1"}
	gw2 := types.NamespacedName{Namespace: "other", Name: "gw2"}

	// Create two gateways with different namespaces
	route1 := ir.HttpRouteIR{
		SourceObject: &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
			},
			Spec: gwv1.HTTPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						{
							Group: ptr.To(gwv1.Group(gwv1.GroupName)),
							Kind:  ptr.To(gwv1.Kind(wellknown.GatewayKind)),
							Name:  gwv1.ObjectName(gw1.Name),
						},
						{
							Group:     ptr.To(gwv1.Group(gwv1.GroupName)),
							Kind:      ptr.To(gwv1.Kind(wellknown.GatewayKind)),
							Namespace: ptr.To(gwv1.Namespace(gw2.Namespace)),
							Name:      gwv1.ObjectName(gw2.Name),
						},
					},
				},
				Rules: []gwv1.HTTPRouteRule{
					{
						BackendRefs: []gwv1.HTTPBackendRef{
							{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Group: ptr.To(gwv1.Group(inf.GroupVersion.Group)),
										Kind:  ptr.To(gwv1.Kind(wellknown.InferencePoolKind)),
										Name:  gwv1.ObjectName(poolNN.Name),
									},
								},
							},
						},
					},
				},
			},
		},
	}
	route2 := ir.HttpRouteIR{
		SourceObject: &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:         ns,
				DeletionTimestamp: ptr.To(metav1.Now()),
			},
			Spec: gwv1.HTTPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						{
							Group: ptr.To(gwv1.Group(gwv1.GroupName)),
							Kind:  ptr.To(gwv1.Kind(wellknown.GatewayKind)),
							Name:  gwv1.ObjectName("deleted-gw"),
						},
					},
				},
				Rules: []gwv1.HTTPRouteRule{
					{
						BackendRefs: []gwv1.HTTPBackendRef{
							{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Group: ptr.To(gwv1.Group(inf.GroupVersion.Group)),
										Kind:  ptr.To(gwv1.Kind(wellknown.InferencePoolKind)),
										Name:  gwv1.ObjectName(poolNN.Name),
									},
								},
							},
						},
					},
				},
			},
		},
	}
	route3 := ir.HttpRouteIR{
		SourceObject: &gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
			},
			Spec: gwv1.HTTPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{
						{
							Group: ptr.To(gwv1.Group(gwv1.GroupName)),
							Kind:  ptr.To(gwv1.Kind(wellknown.GatewayKind)),
							Name:  gwv1.ObjectName("unrelated-gw"),
						},
					},
				},
				Rules: []gwv1.HTTPRouteRule{
					{
						BackendRefs: []gwv1.HTTPBackendRef{
							{
								BackendRef: gwv1.BackendRef{
									BackendObjectReference: gwv1.BackendObjectReference{
										Group: ptr.To(gwv1.Group(gwv1.GroupName)),
										Kind:  ptr.To(gwv1.Kind(wellknown.ServiceKind)),
										Name:  gwv1.ObjectName("unrelated"),
									},
								},
							},
						},
					},
				},
			},
		},
	}
	gws := referencedGateways([]ir.HttpRouteIR{route1, route2, route3}, poolNN)
	assert.Equal(t, map[types.NamespacedName]struct{}{
		gw1: {},
		gw2: {},
	}, gws)
}

func TestIsPoolBackend(t *testing.T) {
	group := gwv1.Group(inf.GroupVersion.Group)
	kind := gwv1.Kind(wellknown.InferencePoolKind)
	be := gwv1.HTTPBackendRef{
		BackendRef: gwv1.BackendRef{
			BackendObjectReference: gwv1.BackendObjectReference{
				Group: &group,
				Kind:  &kind,
				Name:  "my-pool",
			},
		},
	}
	poolNN := types.NamespacedName{Name: "my-pool"}
	// Default namespace (nil) – should match.
	assert.True(t, isPoolBackend(be, poolNN))

	// Wrong name
	be.Name = "wrong"
	assert.False(t, isPoolBackend(be, poolNN))

	// Nil group/kind
	be.Group = nil
	assert.False(t, isPoolBackend(be, poolNN))
	be.Group = &group
	be.Kind = nil
	assert.False(t, isPoolBackend(be, poolNN))

	// Explicit different namespace – should NOT match
	otherNS := gwv1.Namespace("other")
	be.Namespace = &otherNS
	be.Group = &group
	be.Kind = &kind
	be.Name = "my-pool"
	assert.False(t, isPoolBackend(be, poolNN))

	// Explicit matching namespace – should match
	sameNS := gwv1.Namespace("")
	sameNS = gwv1.Namespace(poolNN.Namespace) // assign route namespace
	be.Namespace = &sameNS
	assert.True(t, isPoolBackend(be, poolNN))

	// Wrong group/kind
	wrongGroup := gwv1.Group("wrong")
	be.Group = &wrongGroup
	assert.False(t, isPoolBackend(be, poolNN))
}

func TestParentsEqual(t *testing.T) {
	a := []inf.ParentStatus{
		{
			ParentRef: inf.ParentReference{
				Kind:      inf.Kind(wellknown.GatewayKind),
				Namespace: inf.Namespace("ns"),
				Name:      "gw1",
			},
		},
		{
			ParentRef: inf.ParentReference{
				Group: ptr.To(inf.Group(inf.GroupVersion.Group)),
				Kind:  inf.Kind(defaultInfPoolStatusKind),
				Name:  defaultInfPoolStatusName,
			},
		},
	}
	b := []inf.ParentStatus{
		{
			ParentRef: inf.ParentReference{
				Kind: inf.Kind(defaultInfPoolStatusKind),
				Name: defaultInfPoolStatusName,
			},
		},
		{
			ParentRef: inf.ParentReference{
				Group:     ptr.To(inf.Group(inf.GroupVersion.Group)),
				Kind:      inf.Kind(wellknown.GatewayKind),
				Namespace: inf.Namespace("ns"),
				Name:      "gw1",
			},
		},
	}
	assert.True(t, parentsEqual(a, b))

	// Different
	b[0].ParentRef.Name = "wrong"
	assert.False(t, parentsEqual(a, b))

	// Different length
	b = append(b, a[0])
	assert.False(t, parentsEqual(a, b))
}

func TestBuildAcceptedCondition(t *testing.T) {
	gen := int64(1)
	controllerName := "test-controller"
	// Test the buildAcceptedCondition function
	c := buildAcceptedCondition(gen, controllerName)
	assert.Equal(t, string(inf.InferencePoolConditionAccepted), c.Type)
	assert.Equal(t, metav1.ConditionTrue, c.Status)
	assert.Equal(t, string(inf.InferencePoolReasonAccepted), c.Reason)
	assert.Equal(t, fmt.Sprintf("InferencePool has been accepted by controller %s", controllerName), c.Message)
	assert.Equal(t, gen, c.ObservedGeneration)
	assert.NotZero(t, c.LastTransitionTime)
}

func TestBuildResolvedRefsCondition(t *testing.T) {
	gen := int64(1)
	// Test the buildResolvedRefsCondition function
	c := buildResolvedRefsCondition(gen, nil)
	assert.Equal(t, string(inf.InferencePoolConditionResolvedRefs), c.Type)
	assert.Equal(t, metav1.ConditionTrue, c.Status)
	assert.Equal(t, string(inf.InferencePoolReasonResolvedRefs), c.Reason)
	assert.Equal(t, "All InferencePool references have been resolved", c.Message)
	assert.Equal(t, gen, c.ObservedGeneration)
	assert.NotZero(t, c.LastTransitionTime)

	// With one error
	errs := []error{fmt.Errorf("test error")}
	c = buildResolvedRefsCondition(gen, errs)
	assert.Equal(t, metav1.ConditionFalse, c.Status)
	assert.Equal(t, string(inf.InferencePoolReasonInvalidExtensionRef), c.Reason)
	assert.Equal(t, "error: test error", c.Message)

	// With multiple errors
	errs = append(errs, fmt.Errorf("another error"))
	c = buildResolvedRefsCondition(gen, errs)
	assert.Equal(t, "InferencePool has 2 errors: test error; another error", c.Message)
}
