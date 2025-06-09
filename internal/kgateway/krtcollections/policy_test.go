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

	infextv1a2 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

var (
	svcGk = schema.GroupKind{
		Group: corev1.GroupName,
		Kind:  "Service",
	}
	infPoolGk = schema.GroupKind{
		Group: infextv1a2.GroupVersion.Group,
		Kind:  wellknown.InferencePoolKind,
	}
)

func backends(refN, refNs string) []any {
	return []any{
		httpRouteWithSvcBackendRef(refN, refNs),
		tcpRouteWithBackendRef(refN, refNs),
	}
}

func TestGetBackendSameNamespace(t *testing.T) {
	inputs := []any{
		svc(""),
	}

	for _, backend := range backends("foo", "") {
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
	inputs := []any{
		svc("default2"),
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
	if backends[0].BackendObject.Group != infextv1a2.GroupVersion.Group {
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
	if backends[0].BackendObject.Group != infextv1a2.GroupVersion.Group {
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

func infPool(ns string) *infextv1a2.InferencePool {
	if ns == "" {
		ns = "default"
	}
	return &infextv1a2.InferencePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: ns,
		},
		Spec: infextv1a2.InferencePoolSpec{
			Selector:         map[infextv1a2.LabelKey]infextv1a2.LabelValue{},
			TargetPortNumber: int32(8080),
			EndpointPickerConfig: infextv1a2.EndpointPickerConfig{
				ExtensionRef: &infextv1a2.Extension{
					ExtensionReference: infextv1a2.ExtensionReference{
						Group:      ptr.To(infextv1a2.Group("")),
						Kind:       ptr.To(infextv1a2.Kind(wellknown.ServiceKind)),
						Name:       "fake",
						PortNumber: ptr.To(infextv1a2.PortNumber(9002)),
					},
				},
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
					Group: gwv1.Group(infextv1a2.GroupVersion.Group),
					Kind:  gwv1.Kind(wellknown.InferencePoolKind),
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

func infPoolUpstreams(poolCol krt.Collection[*infextv1a2.InferencePool]) krt.Collection[ir.BackendObjectIR] {
	return krt.NewCollection(poolCol, func(kctx krt.HandlerContext, pool *infextv1a2.InferencePool) *ir.BackendObjectIR {
		// Create a BackendObjectIR IR representation from the given InferencePool.
		backend := ir.NewBackendObjectIR(ir.ObjectSource{
			Kind:      infPoolGk.Kind,
			Group:     infPoolGk.Group,
			Namespace: pool.Namespace,
			Name:      pool.Name,
		}, pool.Spec.TargetPortNumber, "")
		backend.Obj = pool
		backend.GvPrefix = "endpoint-picker"
		backend.CanonicalHostname = ""
		return &backend
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
									Group:     ptr.To(gwv1.Group(infextv1a2.GroupVersion.Group)),
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

func preRouteIndex(t test.Failer, inputs []any) *RoutesIndex {
	mock := krttest.NewMock(t, inputs)
	services := krttest.GetMockCollection[*corev1.Service](mock)
	policyCol := krttest.GetMockCollection[ir.PolicyWrapper](mock)

	policies := NewPolicyIndex(krtutil.KrtOptions{}, extensionsplug.ContributesPolicies{
		wellknown.TrafficPolicyGVK.GroupKind(): {
			Policies: policyCol,
		},
	})
	refgrants := NewRefGrantIndex(krttest.GetMockCollection[*gwv1beta1.ReferenceGrant](mock))
	upstreams := NewBackendIndex(krtutil.KrtOptions{}, policies, refgrants)
	upstreams.AddBackends(svcGk, k8sSvcUpstreams(services))
	pools := krttest.GetMockCollection[*infextv1a2.InferencePool](mock)
	upstreams.AddBackends(infPoolGk, infPoolUpstreams(pools))

	httproutes := krttest.GetMockCollection[*gwv1.HTTPRoute](mock)
	tcpproutes := krttest.GetMockCollection[*gwv1a2.TCPRoute](mock)
	tlsroutes := krttest.GetMockCollection[*gwv1a2.TLSRoute](mock)
	grpcroutes := krttest.GetMockCollection[*gwv1.GRPCRoute](mock)
	rtidx := NewRoutesIndex(krtutil.KrtOptions{}, httproutes, grpcroutes, tcpproutes, tlsroutes, policies, upstreams, refgrants)
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
		Group: infextv1a1.GroupVersion.Group,
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
