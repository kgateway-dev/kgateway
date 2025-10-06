package krtcollections

import (
	"fmt"
	"maps"
	"strings"
	"testing"
	"time"

	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	"istio.io/istio/pkg/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"

	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

var (
	svcGk = schema.GroupKind{
		Group: corev1.GroupName,
		Kind:  "Service",
	}
	infPoolGk = schema.GroupKind{
		Group: inf.GroupVersion.Group,
		Kind:  wellknown.InferencePoolKind,
	}
	backendGk = schema.GroupKind{
		Group: wellknown.BackendGVK.Group,
		Kind:  wellknown.BackendGVK.Kind,
	}
)

func backends(refN, refNs string) []any {
	return []any{
		httpRouteWithSvcBackendRef(refN, refNs),
		tcpRouteWithBackendRef(refN, refNs),
		httpRouteWithInfPoolBackendRefWithPort(refN, refNs, gwv1.PortNumber(8080)), // matching port
		httpRouteWithInfPoolBackendRefWithPort(refN, refNs, gwv1.PortNumber(1234)), // overridden
	}
}

func TestGetBackendSameNamespace(t *testing.T) {
	for _, backend := range backends("foo", "") {
		t.Run(fmt.Sprintf("backend %T", backend), func(t *testing.T) {
			inputs := []any{svc("")}

			// If HTTPRoute points at an InferencePool, inject its CR first
			if rt, ok := backend.(*gwv1.HTTPRoute); ok {
				ref := rt.Spec.Rules[0].BackendRefs[0].BackendRef.BackendObjectReference
				if ref.Kind != nil && *ref.Kind == wellknown.InferencePoolKind {
					inputs = append(inputs, infPool(""))
				}
			}

			// Finally add the route itself and translate it
			inputs = append(inputs, backend)
			ir := translateRoute(t, inputs)
			if ir == nil {
				t.Fatalf("expected ir")
			}
			backends := getBackends(ir)
			if backends == nil {
				t.Fatalf("expected backends")
			}
			if backends[0].Err != nil {
				t.Fatalf("backend has error %v", backends[0].Err)
			}
			if backends[0].BackendObject.Name != "foo" {
				t.Fatalf("backend incorrect name")
			}
			if backends[0].BackendObject.Namespace != "default" {
				t.Fatalf("backend incorrect ns")
			}
		})
	}
}

func TestGetBackendDifNsWithRefGrant(t *testing.T) {
	for _, backend := range backends("foo", "default2") {
		t.Run(fmt.Sprintf("backend %T", backend), func(t *testing.T) {
			inputs := []any{svc("default2"), refGrant()}

			// If HTTPRoute points at an InferencePool, inject that pool CR
			if rt, ok := backend.(*gwv1.HTTPRoute); ok {
				ref := rt.Spec.Rules[0].BackendRefs[0].BackendRef.BackendObjectReference
				if ref.Kind != nil && *ref.Kind == wellknown.InferencePoolKind {
					inputs = append(inputs, infPool("default2"))
				}
			}
			// Add the route under test and translate it
			inputs = append(inputs, backend)
			ir := translateRoute(t, inputs)
			if ir == nil {
				t.Fatalf("expected ir")
			}
			backends := getBackends(ir)
			if backends == nil {
				t.Fatalf("expected backends")
			}
			if backends[0].Err != nil {
				t.Fatalf("backend has error %v", backends[0].Err)
			}
			if backends[0].BackendObject.Name != "foo" {
				t.Fatalf("backend incorrect name")
			}
			if backends[0].BackendObject.Namespace != "default2" {
				t.Fatalf("backend incorrect ns")
			}
		})
	}
}

func TestFailWithNotFoundIfWeHaveRefGrant(t *testing.T) {
	inputs := []any{
		refGrant(),
	}

	for _, backend := range backends("foo", "default2") {
		t.Run(fmt.Sprintf("backend %T", backend), func(t *testing.T) {
			inputs := append(inputs, backend)
			ir := translateRoute(t, inputs)
			if ir == nil {
				t.Fatalf("expected ir")
			}
			backends := getBackends(ir)

			if backends == nil {
				t.Fatalf("expected backends")
			}
			if backends[0].Err == nil {
				t.Fatalf("expected backend error")
			}
			if !strings.Contains(backends[0].Err.Error(), "not found") {
				t.Fatalf("expected not found error. found: %v", backends[0].Err)
			}
		})
	}
}

func TestFailWitWithRefGrantAndWrongFrom(t *testing.T) {
	rg := refGrant()
	rg.Spec.From[0].Kind = gwv1.Kind("NotARoute")
	rg.Spec.From[1].Kind = gwv1.Kind("NotARoute")

	inputs := []any{
		rg,
	}
	for _, backend := range backends("foo", "default2") {
		t.Run(fmt.Sprintf("backend %T", backend), func(t *testing.T) {
			inputs := append(inputs, backend)
			ir := translateRoute(t, inputs)
			if ir == nil {
				t.Fatalf("expected ir")
			}
			backends := getBackends(ir)

			if backends == nil {
				t.Fatalf("expected backends")
			}
			if backends[0].Err == nil {
				t.Fatalf("expected backend error")
			}
			if !strings.Contains(backends[0].Err.Error(), "missing reference grant") {
				t.Fatalf("expected not found error %v", backends[0].Err)
			}
		})
	}
}

func TestFailWithNoRefGrant(t *testing.T) {
	inputs := []any{
		svc("default2"),
	}

	for _, backend := range backends("foo", "default2") {
		t.Run(fmt.Sprintf("backend %T", backend), func(t *testing.T) {
			inputs := append(inputs, backend)
			ir := translateRoute(t, inputs)
			if ir == nil {
				t.Fatalf("expected ir")
			}
			backends := getBackends(ir)
			if backends == nil {
				t.Fatalf("expected backends")
			}
			if backends[0].Err == nil {
				t.Fatalf("expected backend error")
			}
			if !strings.Contains(backends[0].Err.Error(), "missing reference grant") {
				t.Fatalf("expected not found error %v", backends[0].Err)
			}
		})
	}
}

func TestFailWithWrongNs(t *testing.T) {
	inputs := []any{
		svc("default3"),
		refGrant(),
	}
	for _, backend := range backends("foo", "default3") {
		t.Run(fmt.Sprintf("backend %T", backend), func(t *testing.T) {
			inputs := append(inputs, backend)
			ir := translateRoute(t, inputs)
			if ir == nil {
				t.Fatalf("expected ir")
			}
			backends := getBackends(ir)
			if backends == nil {
				t.Fatalf("expected backends")
			}
			if backends[0].Err == nil {
				t.Fatalf("expected backend error %v", backends[0])
			}
			if !strings.Contains(backends[0].Err.Error(), "missing reference grant") {
				t.Fatalf("expected not found error %v", backends[0].Err)
			}
		})
	}
}

func TestInferencePoolBackendSameNamespace(t *testing.T) {
	inputs := []any{
		infPool(""),
	}

	backend := httpRouteWithInfPoolBackendRef("foo", "")
	inputs = append(inputs, backend)

	ir := translateRoute(t, inputs)
	if ir == nil {
		t.Fatalf("expected ir")
	}
	backends := getBackends(ir)
	if backends == nil {
		t.Fatalf("expected backends")
	}
	if backends[0].Err != nil {
		t.Fatalf("backend has error %v", backends[0].Err)
	}
	if backends[0].BackendObject.Name != "foo" {
		t.Fatalf("backend incorrect name")
	}
	if backends[0].BackendObject.Namespace != "default" {
		t.Fatalf("backend incorrect ns")
	}
	if backends[0].BackendObject.Group != inf.GroupVersion.Group {
		t.Fatalf("backend incorrect group")
	}
	if backends[0].BackendObject.Kind != wellknown.InferencePoolKind {
		t.Fatalf("backend incorrect kind")
	}
}

func TestInferencePoolDiffNsBackend(t *testing.T) {
	inputs := []any{
		infPool("default2"),
		refGrant(),
	}

	backend := httpRouteWithInfPoolBackendRef("foo", "default2")
	inputs = append(inputs, backend)
	ir := translateRoute(t, inputs)
	if ir == nil {
		t.Fatalf("expected ir")
	}
	backends := getBackends(ir)
	if backends == nil {
		t.Fatalf("expected backends")
	}
	if backends[0].Err != nil {
		t.Fatalf("backend has error %v", backends[0].Err)
	}
	if backends[0].BackendObject.Name != "foo" {
		t.Fatalf("backend incorrect name")
	}
	if backends[0].BackendObject.Namespace != "default2" {
		t.Fatalf("backend incorrect ns")
	}
	if backends[0].BackendObject.Group != inf.GroupVersion.Group {
		t.Fatalf("backend incorrect group")
	}
	if backends[0].BackendObject.Kind != wellknown.InferencePoolKind {
		t.Fatalf("backend incorrect kind")
	}
}

func TestFailInferencePoolBackendWithoutRefGrant(t *testing.T) {
	inputs := []any{
		infPool("default2"),
		// intentionally missing refGrant
	}

	backend := httpRouteWithInfPoolBackendRef("foo", "default2")
	inputs = append(inputs, backend)

	ir := translateRoute(t, inputs)
	if ir == nil {
		t.Fatalf("expected ir")
	}
	backends := getBackends(ir)
	if backends == nil {
		t.Fatalf("expected backends")
	}
	if backends[0].Err == nil {
		t.Fatalf("expected backend error")
	}
	if !strings.Contains(backends[0].Err.Error(), "missing reference grant") {
		t.Fatalf("expected missing reference grant error, found: %v", backends[0].Err)
	}
}

func TestFailInferencePoolWithRefGrantWrongKind(t *testing.T) {
	rg := refGrant()
	rg.Spec.To[1].Kind = gwv1.Kind("WrongKind")

	inputs := []any{
		infPool("default2"),
		rg,
		httpRouteWithInfPoolBackendRef("foo", "default2"),
	}

	ir := translateRoute(t, inputs)
	if ir == nil {
		t.Fatalf("expected ir")
	}
	backends := getBackends(ir)
	if backends == nil {
		t.Fatalf("expected backends")
	}
	if backends[0].Err == nil {
		t.Fatalf("expected backend error")
	}
	if !strings.Contains(backends[0].Err.Error(), "missing reference grant") {
		t.Fatalf("expected missing reference grant error, found: %v", backends[0].Err)
	}
}

func TestInferencePoolPortOverride(t *testing.T) {
	cases := []struct {
		name         string
		poolNs       string
		refGrant     *gwv1beta1.ReferenceGrant
		providedPort gwv1.PortNumber
		wantPort     int32
		expectError  bool
	}{
		{
			name:         "same‐ns override",
			poolNs:       "",
			refGrant:     nil,
			providedPort: 9001,
			wantPort:     8080,
		},
		{
			name:         "cross‐ns override with RefGrant",
			poolNs:       "foo-ns",
			refGrant:     refGrantWithNs("foo-ns"),
			providedPort: 9999,
			wantPort:     8080,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inputs := []any{
				infPool(tc.poolNs),
			}
			if tc.refGrant != nil {
				inputs = append(inputs, tc.refGrant)
			}
			// Build an HTTPRoute whose HTTPBackendRef.Port is “wrong”
			ns := ""
			if tc.poolNs != "" {
				ns = tc.poolNs
			}
			route := &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "httproute",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					Rules: []gwv1.HTTPRouteRule{{
						BackendRefs: []gwv1.HTTPBackendRef{{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Group:     ptr.To(gwv1.Group(inf.GroupVersion.Group)),
									Kind:      ptr.To(gwv1.Kind(wellknown.InferencePoolKind)),
									Name:      gwv1.ObjectName("foo"),
									Namespace: ptrToNamespace(ns),
									Port:      &tc.providedPort,
								},
							},
						}},
					}},
				},
			}
			inputs = append(inputs, route)

			ir := translateRoute(t, inputs)
			require.NotNil(t, ir)
			b := getBackends(ir)[0]
			if tc.expectError {
				require.Error(t, b.Err)
				return
			}
			require.NoError(t, b.Err)

			// Assert the BackendObject IR port number
			assert.Equal(t, tc.wantPort, b.BackendObject.Port,
				"expected the pool's TargetPortNumber to override the provided port")
		})
	}
}

func TestBackendPortNotAllowed(t *testing.T) {
	cases := []struct {
		name        string
		backendNs   string
		refGrant    *gwv1beta1.ReferenceGrant
		expectError bool
	}{
		{
			name:        "same namespace with port",
			backendNs:   "",
			refGrant:    nil,
			expectError: true,
		},
		{
			name:        "cross namespace with port and ref grant",
			backendNs:   "default2",
			refGrant:    refGrantWithBackend(),
			expectError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Determine the actual namespace for the backend
			backendNs := tc.backendNs
			if backendNs == "" {
				backendNs = "default"
			}

			inputs := []any{
				backend(backendNs),
			}
			if tc.refGrant != nil {
				inputs = append(inputs, tc.refGrant)
			}

			// Create HTTPRoute with Backend reference with port (this should always fail)
			portVal := gwv1.PortNumber(8080)
			route := httpRouteWithBackendRef("test-backend", tc.backendNs, &portVal)
			inputs = append(inputs, route)

			ir := translateRoute(t, inputs)
			require.NotNil(t, ir)
			b := getBackends(ir)[0]

			require.Error(t, b.Err)
			assert.Contains(t, b.Err.Error(), (&BackendPortNotAllowedError{BackendName: "test-backend"}).Error())
		})
	}
}

// Helper to build a Namespace pointer, or nil if empty
func ptrToNamespace(ns string) *gwv1.Namespace {
	if ns == "" {
		return nil
	}
	n := gwv1.Namespace(ns)
	return &n
}

func svc(ns string) *corev1.Service {
	if ns == "" {
		ns = "default"
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: ns,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port: 8080,
				},
			},
		},
	}
}

func infPool(ns string) *inf.InferencePool {
	if ns == "" {
		ns = "default"
	}
	return &inf.InferencePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: ns,
		},
		Spec: inf.InferencePoolSpec{
			Selector: inf.LabelSelector{
				MatchLabels: map[inf.LabelKey]inf.LabelValue{},
			},
			TargetPorts: []inf.Port{{Number: 8080}},
			EndpointPickerRef: inf.EndpointPickerRef{
				Group: ptr.To(inf.Group("")),
				Kind:  inf.Kind(wellknown.ServiceKind),
				Name:  "fake",
				// Port defaults to 9002 unless overridden
			},
		},
	}
}

func refGrant() *gwv1beta1.ReferenceGrant {
	return &gwv1beta1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default2",
			Name:      "foo",
		},
		Spec: gwv1beta1.ReferenceGrantSpec{
			From: []gwv1beta1.ReferenceGrantFrom{
				{
					Group:     gwv1.Group("gateway.networking.k8s.io"),
					Kind:      gwv1.Kind("HTTPRoute"),
					Namespace: gwv1.Namespace("default"),
				},
				{
					Group:     gwv1.Group("gateway.networking.k8s.io"),
					Kind:      gwv1.Kind("TCPRoute"),
					Namespace: gwv1.Namespace("default"),
				},
			},
			To: []gwv1beta1.ReferenceGrantTo{
				{
					Group: gwv1.Group("core"),
					Kind:  gwv1.Kind("Service"),
				},
				{
					Group: gwv1.Group(inf.GroupVersion.Group),
					Kind:  gwv1.Kind(wellknown.InferencePoolKind),
				},
			},
		},
	}
}

// Helper that calls refGrant() but with its Namespace set to the given namespace
func refGrantWithNs(ns string) *gwv1beta1.ReferenceGrant {
	rg := refGrant()
	rg.Namespace = ns
	for i := range rg.Spec.From {
		rg.Spec.From[i].Namespace = gwv1.Namespace("default")
	}
	return rg
}

// Helper that creates a ReferenceGrant for Backend resources
func refGrantWithBackend() *gwv1beta1.ReferenceGrant {
	return &gwv1beta1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default2",
			Name:      "backend-ref-grant",
		},
		Spec: gwv1beta1.ReferenceGrantSpec{
			From: []gwv1beta1.ReferenceGrantFrom{
				{
					Group:     gwv1.Group("gateway.networking.k8s.io"),
					Kind:      gwv1.Kind("HTTPRoute"),
					Namespace: gwv1.Namespace("default"),
				},
			},
			To: []gwv1beta1.ReferenceGrantTo{
				{
					Group: gwv1.Group(wellknown.BackendGVK.Group),
					Kind:  gwv1.Kind(wellknown.BackendGVK.Kind),
				},
			},
		},
	}
}

// Helper that creates a Backend resource
func backend(ns string) *v1alpha1.Backend {
	if ns == "" {
		ns = "default"
	}
	return &v1alpha1.Backend{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-backend",
			Namespace: ns,
		},
		Spec: v1alpha1.BackendSpec{
			Type: v1alpha1.BackendTypeStatic,
			Static: &v1alpha1.StaticBackend{
				Hosts: []v1alpha1.Host{
					{
						Host: "1.2.3.4",
						Port: gwv1.PortNumber(8080),
					},
				},
			},
		},
	}
}

// Helper that creates an HTTPRoute with a Backend reference
func httpRouteWithBackendRef(refN, refNs string, port *gwv1.PortNumber) *gwv1.HTTPRoute {
	var ns *gwv1.Namespace
	if refNs != "" {
		n := gwv1.Namespace(refNs)
		ns = &n
	}
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httproute",
			Namespace: "default",
		},
		Spec: gwv1.HTTPRouteSpec{
			Rules: []gwv1.HTTPRouteRule{
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Group:     ptr.To(gwv1.Group(wellknown.BackendGVK.Group)),
									Kind:      ptr.To(gwv1.Kind(wellknown.BackendGVK.Kind)),
									Name:      gwv1.ObjectName(refN),
									Namespace: ns,
									Port:      port,
								},
							},
						},
					},
				},
			},
		},
	}
}

func k8sSvcUpstreams(services krt.Collection[*corev1.Service]) krt.Collection[ir.BackendObjectIR] {
	return krt.NewManyCollection(services, func(kctx krt.HandlerContext, svc *corev1.Service) []ir.BackendObjectIR {
		uss := []ir.BackendObjectIR{}

		for _, port := range svc.Spec.Ports {
			backend := ir.NewBackendObjectIR(ir.ObjectSource{
				Kind:      svcGk.Kind,
				Group:     svcGk.Group,
				Namespace: svc.Namespace,
				Name:      svc.Name,
			}, port.Port, "")
			backend.Obj = svc
			uss = append(uss, backend)
		}
		return uss
	})
}

func infPoolUpstreams(poolCol krt.Collection[*inf.InferencePool]) krt.Collection[ir.BackendObjectIR] {
	return krt.NewCollection(poolCol, func(kctx krt.HandlerContext, pool *inf.InferencePool) *ir.BackendObjectIR {
		// Create a BackendObjectIR IR representation from the given InferencePool.
		backend := ir.NewBackendObjectIR(ir.ObjectSource{
			Kind:      infPoolGk.Kind,
			Group:     infPoolGk.Group,
			Namespace: pool.Namespace,
			Name:      pool.Name,
		}, int32(pool.Spec.TargetPorts[0].Number), "")
		backend.Obj = pool
		backend.GvPrefix = "endpoint-picker"
		backend.CanonicalHostname = ""
		return &backend
	})
}

func backendUpstreams(backendCol krt.Collection[*v1alpha1.Backend]) krt.Collection[ir.BackendObjectIR] {
	return krt.NewCollection(backendCol, func(kctx krt.HandlerContext, backend *v1alpha1.Backend) *ir.BackendObjectIR {
		// Create a BackendObjectIR IR representation from the given Backend.
		// For static backends, use the port from the first host
		var port int32 = 8080 // default port
		if backend.Spec.Static != nil && len(backend.Spec.Static.Hosts) > 0 {
			port = int32(backend.Spec.Static.Hosts[0].Port)
		}

		backendIR := ir.NewBackendObjectIR(ir.ObjectSource{
			Kind:      backendGk.Kind,
			Group:     backendGk.Group,
			Namespace: backend.Namespace,
			Name:      backend.Name,
		}, port, "")
		backendIR.Obj = backend
		return &backendIR
	})
}

func httpRouteWithSvcBackendRef(refN, refNs string) *gwv1.HTTPRoute {
	var ns *gwv1.Namespace
	if refNs != "" {
		n := gwv1.Namespace(refNs)
		ns = &n
	}
	var port gwv1.PortNumber = 8080
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httproute",
			Namespace: "default",
		},
		Spec: gwv1.HTTPRouteSpec{
			Rules: []gwv1.HTTPRouteRule{
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name:      gwv1.ObjectName(refN),
									Namespace: ns,
									Port:      &port,
								},
							},
						},
					},
				},
			},
		},
	}
}

func httpRouteWithInfPoolBackendRef(refN, refNs string) *gwv1.HTTPRoute {
	var ns *gwv1.Namespace
	if refNs != "" {
		n := gwv1.Namespace(refNs)
		ns = &n
	}
	var port gwv1.PortNumber = 8080
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httproute",
			Namespace: "default",
		},
		Spec: gwv1.HTTPRouteSpec{
			Rules: []gwv1.HTTPRouteRule{
				{
					BackendRefs: []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Group:     ptr.To(gwv1.Group(inf.GroupVersion.Group)),
									Kind:      ptr.To(gwv1.Kind(wellknown.InferencePoolKind)),
									Name:      gwv1.ObjectName(refN),
									Namespace: ns,
									Port:      &port,
								},
							},
						},
					},
				},
			},
		},
	}
}

func tcpRouteWithBackendRef(refN, refNs string) *gwv1a2.TCPRoute {
	var ns *gwv1.Namespace
	if refNs != "" {
		n := gwv1.Namespace(refNs)
		ns = &n
	}
	var port gwv1.PortNumber = 8080
	return &gwv1a2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tcproute",
			Namespace: "default",
		},
		Spec: gwv1a2.TCPRouteSpec{
			Rules: []gwv1a2.TCPRouteRule{
				{
					BackendRefs: []gwv1.BackendRef{
						{
							BackendObjectReference: gwv1.BackendObjectReference{
								Name:      gwv1.ObjectName(refN),
								Namespace: ns,
								Port:      &port,
							},
						},
					},
				},
			},
		},
	}
}

// helper to generate an HTTPRoute that references an InferencePool with a custom port
func httpRouteWithInfPoolBackendRefWithPort(refN, refNs string, port gwv1.PortNumber) *gwv1.HTTPRoute {
	var ns *gwv1.Namespace
	if refNs != "" {
		n := gwv1.Namespace(refNs)
		ns = &n
	}
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "httproute",
			Namespace: "default",
		},
		Spec: gwv1.HTTPRouteSpec{
			Rules: []gwv1.HTTPRouteRule{{
				BackendRefs: []gwv1.HTTPBackendRef{{
					BackendRef: gwv1.BackendRef{
						BackendObjectReference: gwv1.BackendObjectReference{
							Group:     ptr.To(gwv1.Group(inf.GroupVersion.Group)),
							Kind:      ptr.To(gwv1.Kind(wellknown.InferencePoolKind)),
							Name:      gwv1.ObjectName(refN),
							Namespace: ns,
							Port:      &port,
						},
					},
				}},
			}},
		},
	}
}

func preRouteIndex(t test.Failer, inputs []any) *RoutesIndex {
	mock := krttest.NewMock(t, inputs)
	services := krttest.GetMockCollection[*corev1.Service](mock)
	policyCol := krttest.GetMockCollection[ir.PolicyWrapper](mock)

	policies := NewPolicyIndex(
		krtutil.KrtOptions{},
		sdk.ContributesPolicies{
			wellknown.TrafficPolicyGVK.GroupKind(): {
				Policies: policyCol,
			},
		},
		apisettings.Settings{},
	)
	refgrants := NewRefGrantIndex(krttest.GetMockCollection[*gwv1beta1.ReferenceGrant](mock))
	upstreams := NewBackendIndex(krtutil.KrtOptions{}, policies, refgrants)
	upstreams.AddBackends(svcGk, k8sSvcUpstreams(services))
	pools := krttest.GetMockCollection[*inf.InferencePool](mock)
	upstreams.AddBackends(infPoolGk, infPoolUpstreams(pools))
	backends := krttest.GetMockCollection[*v1alpha1.Backend](mock)
	upstreams.AddBackends(backendGk, backendUpstreams(backends))

	httproutes := krttest.GetMockCollection[*gwv1.HTTPRoute](mock)
	tcpproutes := krttest.GetMockCollection[*gwv1a2.TCPRoute](mock)
	tlsroutes := krttest.GetMockCollection[*gwv1a2.TLSRoute](mock)
	grpcroutes := krttest.GetMockCollection[*gwv1.GRPCRoute](mock)
	rtidx := NewRoutesIndex(krtutil.KrtOptions{}, httproutes, grpcroutes, tcpproutes, tlsroutes, policies, upstreams, refgrants, apisettings.Settings{})
	services.WaitUntilSynced(nil)
	policyCol.WaitUntilSynced(nil)
	for !rtidx.HasSynced() || !refgrants.HasSynced() || !policyCol.HasSynced() {
		time.Sleep(time.Second / 10)
	}
	return rtidx
}

func getBackends(r ir.Route) []ir.BackendRefIR {
	if r == nil {
		return nil
	}
	switch r := r.(type) {
	case *ir.HttpRouteIR:
		var ret []ir.BackendRefIR
		for _, r := range r.Rules[0].Backends {
			ret = append(ret, *r.Backend)
		}
		return ret
	case *ir.TcpRouteIR:
		return r.Backends
	}
	panic("should not get here")
}

func translateRoute(t *testing.T, inputs []any) ir.Route {
	rtidx := preRouteIndex(t, inputs)
	tcpGk := schema.GroupKind{
		Group: gwv1a2.GroupName,
		Kind:  "TCPRoute",
	}
	if t := rtidx.Fetch(krt.TestingDummyContext{}, tcpGk, "default", "tcproute"); t != nil {
		return t.Route
	}

	/*poolGk := schema.GroupKind{
		Group: inf.GroupVersion.Group,
		Kind:  wellknown.InferencePoolKind,
	}
	if t := rtidx.Fetch(krt.TestingDummyContext{}, poolGk, "default", "inferencepool"); t != nil {
		return t.Route
	}*/

	h := rtidx.FetchHttp(krt.TestingDummyContext{}, "default", "httproute")
	if h == nil {
		// do this nil check so we don't return a typed nil
		return nil
	}
	return h
}

type fakePolicyIR struct{}

func (f fakePolicyIR) CreationTime() time.Time {
	return metav1.Now().Time
}

func (f fakePolicyIR) Equals(_ any) bool {
	return false
}

type routeSelection string

const (
	onePolicyPerRoute routeSelection = "onePolicyPerRoute"

	allPoliciesPerRoute routeSelection = "allPoliciesPerRoute"
)

// BenchmarkPolicyAttachment is a benchmark to test the performance of policy attachment
// with TargetRef.Name and TargetRef.LabelSelector for different scenarios.
func BenchmarkPolicyAttachment(b *testing.B) {
	tests := []struct {
		routes                   int
		policies                 int
		byLabel                  bool
		selectionPolicy          routeSelection
		randN                    int
		expectedPoliciesPerRoute int
	}{
		{routes: 1000, policies: 1000, byLabel: false, selectionPolicy: onePolicyPerRoute, expectedPoliciesPerRoute: 1},
		{routes: 1000, policies: 1000, byLabel: true, selectionPolicy: onePolicyPerRoute, expectedPoliciesPerRoute: 1},
		{routes: 5000, policies: 5000, byLabel: false, selectionPolicy: onePolicyPerRoute, expectedPoliciesPerRoute: 1},
		{routes: 5000, policies: 5000, byLabel: true, selectionPolicy: onePolicyPerRoute, expectedPoliciesPerRoute: 1},
		{routes: 10000, policies: 10000, byLabel: false, selectionPolicy: onePolicyPerRoute, expectedPoliciesPerRoute: 1},
		{routes: 10000, policies: 10000, byLabel: true, selectionPolicy: onePolicyPerRoute, expectedPoliciesPerRoute: 1},
		{routes: 1, policies: 10000, byLabel: false, selectionPolicy: allPoliciesPerRoute, expectedPoliciesPerRoute: 10000},
		{routes: 1, policies: 10000, byLabel: true, selectionPolicy: allPoliciesPerRoute, expectedPoliciesPerRoute: 10000},
		{routes: 10, policies: 10000, byLabel: false, selectionPolicy: allPoliciesPerRoute, expectedPoliciesPerRoute: 10000},
		{routes: 10, policies: 10000, byLabel: true, selectionPolicy: allPoliciesPerRoute, expectedPoliciesPerRoute: 10000},
		{routes: 1000, policies: 10000, byLabel: false, selectionPolicy: allPoliciesPerRoute, expectedPoliciesPerRoute: 10000},
		{routes: 1000, policies: 10000, byLabel: true, selectionPolicy: allPoliciesPerRoute, expectedPoliciesPerRoute: 10000},
	}

	for _, tc := range tests {
		b.Run(fmt.Sprintf("routes=%d,policies=%d,byLabel=%t,selectionPolicy=%s,randN=%d", tc.routes, tc.policies, tc.byLabel, tc.selectionPolicy, tc.randN), func(b *testing.B) {
			r := require.New(b)
			if tc.selectionPolicy == onePolicyPerRoute {
				r.Equal(tc.routes, tc.policies)
			}

			total := tc.routes + tc.policies
			inputs := make([]any, 0, total)
			var routeLabels map[string]string
			if tc.byLabel {
				routeLabels = map[string]string{"k1": "v1", "k2": "v2", "k3": "v3", "k4": "v4", "k5": "v5"}
			}
			for i := range tc.routes {
				routeLabels := maps.Clone(routeLabels)
				if tc.byLabel {
					routeLabels[fmt.Sprint(i)] = "yes"
				}
				inputs = append(inputs, &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "httproute-" + fmt.Sprint(i),
						Namespace: "default",
						Labels:    routeLabels,
					},
					Spec: gwv1.HTTPRouteSpec{
						Rules: []gwv1.HTTPRouteRule{
							{
								BackendRefs: []gwv1.HTTPBackendRef{
									{
										BackendRef: gwv1.BackendRef{
											BackendObjectReference: gwv1.BackendObjectReference{
												Name: gwv1.ObjectName("foo"),
												Port: ptr.To(gwv1.PortNumber(8080)),
											},
										},
									},
								},
							},
						},
					},
				})
			}

			for i := range tc.policies {
				routeLabels := maps.Clone(routeLabels)
				p := ir.PolicyWrapper{
					ObjectSource: ir.ObjectSource{
						Group:     wellknown.TrafficPolicyGVK.Group,
						Kind:      wellknown.TrafficPolicyGVK.Kind,
						Namespace: "default",
						Name:      "policy-" + fmt.Sprint(i),
					},
					Policy:   &v1alpha1.TrafficPolicy{},
					PolicyIR: fakePolicyIR{},
				}
				if tc.byLabel {
					switch tc.selectionPolicy {
					case onePolicyPerRoute:
						routeLabels[fmt.Sprint(i)] = "yes"
					case allPoliciesPerRoute:
					}
					p.TargetRefs = []ir.PolicyRef{
						{
							Group:       "gateway.networking.k8s.io",
							Kind:        "HTTPRoute",
							MatchLabels: routeLabels,
						},
					}
				} else {
					switch tc.selectionPolicy {
					case onePolicyPerRoute:
						p.TargetRefs = []ir.PolicyRef{
							{
								Group: "gateway.networking.k8s.io",
								Kind:  "HTTPRoute",
								Name:  "httproute-" + fmt.Sprint(i),
							},
						}
					case allPoliciesPerRoute:
						p.TargetRefs = make([]ir.PolicyRef, 0, tc.routes)
						for r := range tc.routes {
							p.TargetRefs = append(p.TargetRefs, ir.PolicyRef{
								Group: "gateway.networking.k8s.io",
								Kind:  "HTTPRoute",
								Name:  "httproute-" + fmt.Sprint(r),
							})
						}
					}
				}
				inputs = append(inputs, p)
			}

			a := assert.New(b)
			for b.Loop() {
				rtidx := preRouteIndex(b, inputs)
				firstRoute := "httproute-0"
				lastRoute := "httproute-" + fmt.Sprint(tc.routes-1)

				for _, route := range []string{firstRoute, lastRoute} {
					h := rtidx.FetchHttp(krt.TestingDummyContext{}, "default", route)
					a.NotNil(h)
					a.Len(h.AttachedPolicies.Policies, 1)
					a.Len(h.AttachedPolicies.Policies[wellknown.TrafficPolicyGVK.GroupKind()], tc.expectedPoliciesPerRoute)
				}
			}
		})
	}
}
