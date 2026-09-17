package ir

import (
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type revisionTestPolicy struct{}

func (revisionTestPolicy) CreationTime() time.Time { return time.Time{} }
func (revisionTestPolicy) Equals(any) bool         { return true }

func TestEqualsIgnoresResourceVersion(t *testing.T) {
	for _, gen := range []int64{0, 1} {
		a := metav1.ObjectMeta{Name: "x", Namespace: "ns", UID: "uid", Generation: gen, ResourceVersion: "1"}
		b := *a.DeepCopy()
		b.ResourceVersion = "2"
		b.ManagedFields = []metav1.ManagedFieldsEntry{{Manager: "status-writer"}}
		ga := &gwv1.Gateway{ObjectMeta: a}
		gb := &gwv1.Gateway{ObjectMeta: b}
		ba := NewBackendObjectIR(ObjectSource{Name: "x", Kind: "Service"}, 80, "", "svc")
		ba.Obj = &corev1.Service{ObjectMeta: a}
		bb := ba
		bb.Obj = &corev1.Service{ObjectMeta: b}
		cases := map[string]bool{
			"versionEquals":   versionEquals(&a, &b),
			"Secret":          (Secret{Obj: &corev1.Secret{ObjectMeta: a}}).Equals(Secret{Obj: &corev1.Secret{ObjectMeta: b}}),
			"BackendObjectIR": ba.Equals(bb),
			"Gateway":         (Gateway{Obj: ga}).Equals(Gateway{Obj: gb}),
			"Listener":        (Listener{Parent: ga}).Equals(Listener{Parent: gb}),
			"ListenerSet":     (ListenerSet{Obj: &gwv1.ListenerSet{ObjectMeta: a}}).Equals(ListenerSet{Obj: &gwv1.ListenerSet{ObjectMeta: b}}),
			"HttpRouteIR":     (HttpRouteIR{SourceObject: &gwv1.HTTPRoute{ObjectMeta: a}}).Equals(HttpRouteIR{SourceObject: &gwv1.HTTPRoute{ObjectMeta: b}}),
			"TcpRouteIR":      (TcpRouteIR{SourceObject: &gwv1a2.TCPRoute{ObjectMeta: a}}).Equals(TcpRouteIR{SourceObject: &gwv1a2.TCPRoute{ObjectMeta: b}}),
			"TlsRouteIR":      (TlsRouteIR{SourceObject: &gwv1a2.TLSRoute{ObjectMeta: a}}).Equals(TlsRouteIR{SourceObject: &gwv1a2.TLSRoute{ObjectMeta: b}}),
			"PolicyWrapper":   (PolicyWrapper{Policy: &a, PolicyIR: revisionTestPolicy{}}).Equals(PolicyWrapper{Policy: &b, PolicyIR: revisionTestPolicy{}}),
		}
		for name, got := range cases {
			t.Run(fmt.Sprintf("%s/gen%d", name, gen), func(t *testing.T) {
				if !got {
					t.Fatalf("Equals=%v", got)
				}
			})
		}
	}
	sa := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc", ResourceVersion: "1"}}
	sb := sa.DeepCopy()
	sb.ResourceVersion = "2"
	sb.ManagedFields = []metav1.ManagedFieldsEntry{{Manager: "status-writer"}}
	ba := NewBackendObjectIR(ObjectSource{Name: "svc", Kind: "Service"}, 80, "", "svc")
	ba.Obj = sa
	bb := ba
	bb.Obj = sb
	route := &gwv1a2.TCPRoute{ObjectMeta: metav1.ObjectMeta{Generation: 1}}
	ra := TcpRouteIR{SourceObject: route, Backends: []BackendRefIR{{BackendObject: &ba}}}
	rb := TcpRouteIR{SourceObject: route, Backends: []BackendRefIR{{BackendObject: &bb}}}
	if !ra.Equals(rb) {
		t.Fatal("Service RV-only change propagated into an unchanged route")
	}
	dataA := Secret{Obj: &metav1.ObjectMeta{}, Data: map[string][]byte{"key": []byte("a")}}
	dataB := Secret{Obj: &metav1.ObjectMeta{}, Data: map[string][]byte{"key": []byte("b")}}
	if dataA.Equals(dataB) {
		t.Fatal("Secret equality missed data-only change at same RV")
	}
}

func TestVersionEqualsGenerationZeroContent(t *testing.T) {
	base := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc", UID: "uid"}, Spec: corev1.ServiceSpec{ClusterIP: "10.0.0.1"}}
	for _, tc := range []struct {
		name   string
		mutate func(*corev1.Service)
		want   bool
	}{
		{"revision", func(s *corev1.Service) { s.ResourceVersion = "2" }, true},
		{"status", func(s *corev1.Service) { s.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "1.2.3.4"}} }, true},
		{"spec", func(s *corev1.Service) { s.Spec.ClusterIP = "10.0.0.2" }, false},
		{"labels", func(s *corev1.Service) { s.Labels = map[string]string{"selector": "new"} }, false},
		{"annotations", func(s *corev1.Service) { s.Annotations = map[string]string{"config": "new"} }, false},
		{"replacement", func(s *corev1.Service) { s.UID = "new" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := base.DeepCopy()
			tc.mutate(changed)
			if versionEquals(base, changed) != tc.want || versionEquals(changed, base) != tc.want {
				t.Fatalf("source equality should be %v in both directions", tc.want)
			}
		})
	}
	// CRD fixtures with no generation still need to observe real spec changes.
	route := &gwv1a2.TCPRoute{}
	changedRoute := route.DeepCopy()
	changedRoute.Spec.ParentRefs = []gwv1.ParentReference{{Name: "other-gateway"}}
	if versionEquals(route, changedRoute) {
		t.Fatal("missed generation-zero route spec change")
	}
	config := &corev1.ConfigMap{Data: map[string]string{"key": "old"}}
	changedConfig := config.DeepCopy()
	changedConfig.Data["key"] = "new"
	if versionEquals(config, changedConfig) {
		t.Fatal("missed ConfigMap data change")
	}
}

func TestVersionEqualsDoesNotMutateUnstructuredSources(t *testing.T) {
	a := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.io/v1", "kind": "Policy",
		"metadata": map[string]any{"name": "policy", "resourceVersion": "1"},
		"spec":     map[string]any{"key": "value"},
		"status":   map[string]any{"ready": false},
	}}
	b := a.DeepCopy()
	b.SetResourceVersion("2")
	b.Object["status"] = map[string]any{"ready": true}
	if !versionEquals(a, b) {
		t.Fatal("RV/status-only update should be equal")
	}
	if a.GetResourceVersion() != "1" || b.GetResourceVersion() != "2" || a.Object["status"] == nil || b.Object["status"] == nil {
		t.Fatal("comparison mutated source objects")
	}
	b.Object["spec"] = map[string]any{"key": "new"}
	if versionEquals(a, b) {
		t.Fatal("missed unstructured spec change")
	}
}

func TestSecretEqualsComparesDataWithNonzeroGeneration(t *testing.T) {
	a := Secret{Obj: &metav1.ObjectMeta{Generation: 1}, Data: map[string][]byte{"key": []byte("old")}}
	b := Secret{Obj: &metav1.ObjectMeta{Generation: 1}, Data: map[string][]byte{"key": []byte("new")}}
	if a.Equals(b) || b.Equals(a) {
		t.Fatal("missed Secret data change with unchanged metadata")
	}
}
