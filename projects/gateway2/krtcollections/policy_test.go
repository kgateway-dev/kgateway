package krtcollections

import (
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

func TestGetBackendSameNamespace(t *testing.T) {
	inputs := []any{
		svc(""),
		httpRouteWithBackendRef("foo", ""),
	}

	ir := translate(t, inputs)
	if ir == nil {
		t.Fatalf("expected ir")
	}
	if ir.Rules[0].Backends == nil {
		t.Fatalf("expected backends")
	}
	if ir.Rules[0].Backends[0].Backend.Err != nil {
		t.Fatalf("backend has error %v", ir.Rules[0].Backends[0].Backend.Err)
	}
	if ir.Rules[0].Backends[0].Backend.Upstream.Name != "foo" {
		t.Fatalf("backend incorrect name")
	}
	if ir.Rules[0].Backends[0].Backend.Upstream.Namespace != "default" {
		t.Fatalf("backend incorrect ns")
	}
}

func TestGetBackendDifNsWithRefGrant(t *testing.T) {
	inputs := []any{
		svc("default2"),
		refGrant(),
		httpRouteWithBackendRef("foo", "default2"),
	}

	ir := translate(t, inputs)
	if ir == nil {
		t.Fatalf("expected ir")
	}
	if ir.Rules[0].Backends == nil {
		t.Fatalf("expected backends")
	}
	if ir.Rules[0].Backends[0].Backend.Err != nil {
		t.Fatalf("backend has error %v", ir.Rules[0].Backends[0].Backend.Err)
	}
	if ir.Rules[0].Backends[0].Backend.Upstream.Name != "foo" {
		t.Fatalf("backend incorrect name")
	}
	if ir.Rules[0].Backends[0].Backend.Upstream.Namespace != "default2" {
		t.Fatalf("backend incorrect ns")
	}
}

func TestFailWithNotFoundIfWeHaveRefGrant(t *testing.T) {
	inputs := []any{
		refGrant(),
		httpRouteWithBackendRef("foo", "default2"),
	}

	ir := translate(t, inputs)
	if ir == nil {
		t.Fatalf("expected ir")
	}
	if ir.Rules[0].Backends == nil {
		t.Fatalf("expected backends")
	}
	if ir.Rules[0].Backends[0].Backend.Err == nil {
		t.Fatalf("expected backend error")
	}
	if !strings.Contains(ir.Rules[0].Backends[0].Backend.Err.Error(), "not found") {
		t.Fatalf("expected not found error")
	}
}

func TestFailWitWithRefGrantAndWrongFrom(t *testing.T) {
	rg := refGrant()
	rg.Spec.From[0].Kind = gwv1.Kind("NotHTTPRoute")

	inputs := []any{
		rg,
		httpRouteWithBackendRef("foo", "default2"),
	}

	ir := translate(t, inputs)
	if ir == nil {
		t.Fatalf("expected ir")
	}
	if ir.Rules[0].Backends == nil {
		t.Fatalf("expected backends")
	}
	if ir.Rules[0].Backends[0].Backend.Err == nil {
		t.Fatalf("expected backend error")
	}
	if !strings.Contains(ir.Rules[0].Backends[0].Backend.Err.Error(), "missing reference grant") {
		t.Fatalf("expected not found error %v", ir.Rules[0].Backends[0].Backend.Err)
	}
}

func TestFailWithNoRefGrant(t *testing.T) {
	inputs := []any{
		svc("default2"),
		httpRouteWithBackendRef("foo", "default2"),
	}

	ir := translate(t, inputs)
	if ir == nil {
		t.Fatalf("expected ir")
	}
	if ir.Rules[0].Backends == nil {
		t.Fatalf("expected backends")
	}
	if ir.Rules[0].Backends[0].Backend.Err == nil {
		t.Fatalf("expected backend error")
	}
	if !strings.Contains(ir.Rules[0].Backends[0].Backend.Err.Error(), "missing reference grant") {
		t.Fatalf("expected not found error %v", ir.Rules[0].Backends[0].Backend.Err)
	}
}
func TestFailWithWrongNs(t *testing.T) {
	inputs := []any{
		svc("default3"),
		refGrant(),
		httpRouteWithBackendRef("foo", "default3"),
	}

	ir := translate(t, inputs)
	if ir == nil {
		t.Fatalf("expected ir")
	}
	if ir.Rules[0].Backends == nil {
		t.Fatalf("expected backends")
	}
	if ir.Rules[0].Backends[0].Backend.Err == nil {
		t.Fatalf("expected backend error %v", ir.Rules[0].Backends[0].Backend)
	}
	if !strings.Contains(ir.Rules[0].Backends[0].Backend.Err.Error(), "missing reference grant") {
		t.Fatalf("expected not found error %v", ir.Rules[0].Backends[0].Backend.Err)
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

func translate(t *testing.T, inputs []any) *ir.HttpRouteIR {
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

	return rtidx.FetchHttp(krt.TestingDummyContext{}, "default", "httproute")
}
