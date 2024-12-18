package krtcollections

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	extensionsplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"
	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/utils/krtutil"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

var (
	SvcGk = schema.GroupKind{
		Group: corev1.GroupName,
		Kind:  "Service",
	}
)

func backends(refN, refNs string) []any {
	return []any{httpRouteWithBackendRef(refN, refNs),
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
			if backends[0].Upstream.Name != "foo" {
				t.Fatalf("backend incorrect name")
			}
			if backends[0].Upstream.Namespace != "default" {
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
			if backends[0].Upstream.Name != "foo" {
				t.Fatalf("backend incorrect name")
			}
			if backends[0].Upstream.Namespace != "default2" {
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
			},
		},
	}
}

func k8sUpstreams(services krt.Collection[*corev1.Service]) krt.Collection[ir.Upstream] {
	return krt.NewManyCollection(services, func(kctx krt.HandlerContext, svc *corev1.Service) []ir.Upstream {
		uss := []ir.Upstream{}

		for _, port := range svc.Spec.Ports {
			uss = append(uss, ir.Upstream{
				ObjectSource: ir.ObjectSource{
					Kind:      SvcGk.Kind,
					Group:     SvcGk.Group,
					Namespace: svc.Namespace,
					Name:      svc.Name,
				},
				Obj:  svc,
				Port: port.Port,
			})
		}
		return uss
	})
}

func httpRouteWithBackendRef(refN, refNs string) *gwv1.HTTPRoute {
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

func preRouteIndex(t *testing.T, inputs []any) *RoutesIndex {
	mock := krttest.NewMock(t, inputs)
	services := krttest.GetMockCollection[*corev1.Service](mock)

	policies := NewPolicyIndex(krtutil.KrtOptions{}, extensionsplug.ContributesPolicies{})
	upstreams := NewUpstreamIndex(krtutil.KrtOptions{}, nil, policies)
	upstreams.AddUpstreams(SvcGk, k8sUpstreams(services))
	refgrants := NewRefGrantIndex(krttest.GetMockCollection[*gwv1beta1.ReferenceGrant](mock))

	httproutes := krttest.GetMockCollection[*gwv1.HTTPRoute](mock)
	tcpproutes := krttest.GetMockCollection[*gwv1a2.TCPRoute](mock)
	rtidx := NewRoutesIndex(krtutil.KrtOptions{}, httproutes, tcpproutes, policies, upstreams, refgrants)
	services.Synced().WaitUntilSynced(nil)
	for !rtidx.HasSynced() || !refgrants.HasSynced() {
		time.Sleep(time.Second / 10)
	}
	return rtidx
}

func getBackends(r ir.Route) []ir.Backend {
	if r == nil {
		return nil
	}
	switch r := r.(type) {
	case *ir.HttpRouteIR:
		var ret []ir.Backend
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

func translate(t *testing.T, inputs []any) *ir.HttpRouteIR {
	rtidx := preRouteIndex(t, inputs)
	return rtidx.FetchHttp(krt.TestingDummyContext{}, "default", "httproute")
}
