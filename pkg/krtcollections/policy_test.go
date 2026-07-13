package krtcollections

import (
	"fmt"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	"istio.io/istio/pkg/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

var (
	svcGk = schema.GroupKind{
		Group: corev1.GroupName,
		Kind:  "Service",
	}
	backendGk = schema.GroupKind{
		Group: wellknown.BackendGVK.Group,
		Kind:  wellknown.BackendGVK.Kind,
	}
)

func backends(refNs string) []any {
	return []any{
		httpRouteWithSvcBackendRef(refNs),
		tcpRouteWithBackendRef(refNs),
	}
}

func TestGetBackendSameNamespace(t *testing.T) {
	for _, backend := range backends("") {
		t.Run(fmt.Sprintf("backend %T", backend), func(t *testing.T) {
			inputs := []any{svc("")}
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
			if backends[0].BackendObject.GetName() != "foo" {
				t.Fatalf("backend incorrect name")
			}
			if backends[0].BackendObject.GetNamespace() != "default" {
				t.Fatalf("backend incorrect ns")
			}
		})
	}
}

func TestProcessRouteStatusMarkers(t *testing.T) {
	controllerName := gwv1.GatewayController(wellknown.DefaultGatewayControllerName)
	parentStatus := []gwv1.RouteParentStatus{{
		ControllerName: controllerName,
		ParentRef: gwv1.ParentReference{
			Name: "missing-gateway",
		},
	}}

	routeKey := types.NamespacedName{Namespace: "default", Name: "orphaned-route"}

	t.Run("grpc route", func(t *testing.T) {
		routes := preRouteIndex(t, []any{
			&gwv1.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{Name: routeKey.Name, Namespace: routeKey.Namespace},
				Status: gwv1.GRPCRouteStatus{
					RouteStatus: gwv1.RouteStatus{Parents: parentStatus},
				},
			},
		})
		reportMap := reports.NewReportMap()
		routes.ProcessRouteStatusMarkers(krt.TestingDummyContext{}, reportMap)

		require.NotNil(t, reportMap.GRPCRoutes[routeKey])
		require.Empty(t, reportMap.GRPCRoutes[routeKey].Parents)
	})

	t.Run("tcp route", func(t *testing.T) {
		routes := preRouteIndex(t, []any{
			&gwv1a2.TCPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: routeKey.Name, Namespace: routeKey.Namespace},
				Status: gwv1a2.TCPRouteStatus{
					RouteStatus: gwv1.RouteStatus{Parents: parentStatus},
				},
			},
		})
		reportMap := reports.NewReportMap()
		routes.ProcessRouteStatusMarkers(krt.TestingDummyContext{}, reportMap)

		require.NotNil(t, reportMap.TCPRoutes[routeKey])
		require.Empty(t, reportMap.TCPRoutes[routeKey].Parents)
	})

	t.Run("tls route", func(t *testing.T) {
		routes := preRouteIndex(t, []any{
			&gwv1a2.TLSRoute{
				ObjectMeta: metav1.ObjectMeta{Name: routeKey.Name, Namespace: routeKey.Namespace},
				Status: gwv1a2.TLSRouteStatus{
					RouteStatus: gwv1.RouteStatus{Parents: parentStatus},
				},
			},
		})
		reportMap := reports.NewReportMap()
		routes.ProcessRouteStatusMarkers(krt.TestingDummyContext{}, reportMap)

		require.NotNil(t, reportMap.TLSRoutes[routeKey])
		require.Empty(t, reportMap.TLSRoutes[routeKey].Parents)
	})
}

func TestProcessRouteStatusMarkersPreservesExistingReports(t *testing.T) {
	controllerName := gwv1.GatewayController(wellknown.DefaultGatewayControllerName)
	parentStatus := []gwv1.RouteParentStatus{{
		ControllerName: controllerName,
		ParentRef: gwv1.ParentReference{
			Name: "gateway",
		},
	}}

	routeKey := types.NamespacedName{Namespace: "default", Name: "reported-route"}
	routes := preRouteIndex(t, []any{
		&gwv1.GRPCRoute{
			ObjectMeta: metav1.ObjectMeta{Name: routeKey.Name, Namespace: routeKey.Namespace},
			Status: gwv1.GRPCRouteStatus{
				RouteStatus: gwv1.RouteStatus{Parents: parentStatus},
			},
		},
	})

	reportMap := reports.NewReportMap()
	reporter := reports.NewReporter(&reportMap)
	reporter.Route(&gwv1.GRPCRoute{ObjectMeta: metav1.ObjectMeta{
		Namespace: routeKey.Namespace,
		Name:      routeKey.Name,
	}}).ParentRef(&gwv1.ParentReference{Name: "gateway"})
	before := reportMap.GRPCRoutes[routeKey]

	routes.ProcessRouteStatusMarkers(krt.TestingDummyContext{}, reportMap)

	require.Same(t, before, reportMap.GRPCRoutes[routeKey])
	require.NotEmpty(t, reportMap.GRPCRoutes[routeKey].Parents)
}

func TestRoutesFor(t *testing.T) {
	parentStatus := []gwv1.RouteParentStatus{{
		ControllerName: gwv1.GatewayController(wellknown.DefaultGatewayControllerName),
		ParentRef: gwv1.ParentReference{
			Name: "example-gateway",
		},
	}}

	routes := preRouteIndex(t, []any{
		&gwv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "http-route", Namespace: "default"},
			Spec: gwv1.HTTPRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{{
						Name: "example-gateway",
					}},
				},
			},
			Status: gwv1.HTTPRouteStatus{
				RouteStatus: gwv1.RouteStatus{Parents: parentStatus},
			},
		},
		&gwv1.GRPCRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "grpc-route", Namespace: "default"},
			Spec: gwv1.GRPCRouteSpec{
				CommonRouteSpec: gwv1.CommonRouteSpec{
					ParentRefs: []gwv1.ParentReference{{
						Name: "example-gateway",
					}},
				},
			},
			Status: gwv1.GRPCRouteStatus{
				RouteStatus: gwv1.RouteStatus{Parents: parentStatus},
			},
		},
	})

	nns := types.NamespacedName{Namespace: "default", Name: "example-gateway"}
	group := wellknown.GatewayGVK.Group
	kind := wellknown.GatewayGVK.Kind

	rts := routes.RoutesFor(krt.TestingDummyContext{}, nns, group, kind)
	require.Len(t, rts, 2)
}

func TestGetBackendDifNsWithRefGrant(t *testing.T) {
	for _, backend := range backends("default2") {
		t.Run(fmt.Sprintf("backend %T", backend), func(t *testing.T) {
			inputs := []any{svc("default2"), refGrant()}
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
			if backends[0].BackendObject.GetName() != "foo" {
				t.Fatalf("backend incorrect name")
			}
			if backends[0].BackendObject.GetNamespace() != "default2" {
				t.Fatalf("backend incorrect ns")
			}
		})
	}
}

func TestFailWithNotFoundIfWeHaveRefGrant(t *testing.T) {
	inputs := []any{
		refGrant(),
	}

	for _, backend := range backends("default2") {
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
	for _, backend := range backends("default2") {
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

	for _, backend := range backends("default2") {
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
	for _, backend := range backends("default3") {
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

func TestBackendPortNotAllowed(t *testing.T) {
	cases := []struct {
		name        string
		backendNs   string
		refGrant    *gwv1b1.ReferenceGrant
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

func refGrant() *gwv1b1.ReferenceGrant {
	return &gwv1b1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default2",
			Name:      "foo",
		},
		Spec: gwv1b1.ReferenceGrantSpec{
			From: []gwv1b1.ReferenceGrantFrom{
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
			To: []gwv1b1.ReferenceGrantTo{
				{
					Group: gwv1.Group("core"),
					Kind:  gwv1.Kind("Service"),
				},
			},
		},
	}
}

// Helper that creates a ReferenceGrant for Backend resources
func refGrantWithBackend() *gwv1b1.ReferenceGrant {
	return &gwv1b1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default2",
			Name:      "backend-ref-grant",
		},
		Spec: gwv1b1.ReferenceGrantSpec{
			From: []gwv1b1.ReferenceGrantFrom{
				{
					Group:     gwv1.Group("gateway.networking.k8s.io"),
					Kind:      gwv1.Kind("HTTPRoute"),
					Namespace: gwv1.Namespace("default"),
				},
			},
			To: []gwv1b1.ReferenceGrantTo{
				{
					Group: gwv1.Group(wellknown.BackendGVK.Group),
					Kind:  gwv1.Kind(wellknown.BackendGVK.Kind),
				},
			},
		},
	}
}

// Helper that creates a Backend resource
func backend(ns string) *kgateway.Backend {
	if ns == "" {
		ns = "default"
	}
	return &kgateway.Backend{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-backend",
			Namespace: ns,
		},
		Spec: kgateway.BackendSpec{
			Type: new(kgateway.BackendTypeStatic),
			Static: &kgateway.StaticBackend{
				Hosts: []kgateway.Host{
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
									Group:     new(gwv1.Group(wellknown.BackendGVK.Group)),
									Kind:      new(gwv1.Kind(wellknown.BackendGVK.Kind)),
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
			}, port.Port, "", "")
			backend.Obj = svc
			uss = append(uss, backend)
		}
		return uss
	})
}

func backendUpstreams(backendCol krt.Collection[*kgateway.Backend]) krt.Collection[ir.BackendObjectIR] {
	return krt.NewCollection(backendCol, func(kctx krt.HandlerContext, backend *kgateway.Backend) *ir.BackendObjectIR {
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
		}, port, "", "")
		backendIR.Obj = backend
		return &backendIR
	})
}

func httpRouteWithSvcBackendRef(refNs string) *gwv1.HTTPRoute {
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
									Name:      gwv1.ObjectName("foo"),
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

func tcpRouteWithBackendRef(refNs string) *gwv1a2.TCPRoute {
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
								Name:      gwv1.ObjectName("foo"),
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
	refgrants := NewRefGrantIndex(krttest.GetMockCollection[*gwv1b1.ReferenceGrant](mock), apisettings.ReferenceGrantPermissive)
	upstreams := NewBackendIndex(krtutil.KrtOptions{}, policies, refgrants)
	upstreams.AddBackends(svcGk, k8sSvcUpstreams(services))
	backends := krttest.GetMockCollection[*kgateway.Backend](mock)
	upstreams.AddBackends(backendGk, backendUpstreams(backends))

	httproutes := krttest.GetMockCollection[*gwv1.HTTPRoute](mock)
	tcpproutes := krttest.GetMockCollection[*gwv1a2.TCPRoute](mock)
	tlsroutes := krttest.GetMockCollection[*gwv1a2.TLSRoute](mock)
	grpcroutes := krttest.GetMockCollection[*gwv1.GRPCRoute](mock)
	rtidx := NewRoutesIndex(krtutil.KrtOptions{}, wellknown.DefaultGatewayControllerName, httproutes, grpcroutes, tcpproutes, tlsroutes, policies, upstreams, refgrants, apisettings.Settings{})
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
												Port: new(gwv1.PortNumber(8080)),
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
					Policy:   &kgateway.TrafficPolicy{},
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
