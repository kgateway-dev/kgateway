package krtcollections

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestGatewaySpecEquals(t *testing.T) {
	base := &gwv1.Gateway{ObjectMeta: metav1.ObjectMeta{
		Name:            "gw",
		Namespace:       "default",
		UID:             "uid",
		Generation:      1,
		ResourceVersion: "1",
		Labels:          map[string]string{"a": "b"},
		Annotations:     map[string]string{"c": "d"},
	}}
	tests := []struct {
		name   string
		mutate func(*gwv1.Gateway)
		want   bool
	}{
		{
			name: "status-only write",
			mutate: func(gw *gwv1.Gateway) {
				gw.ResourceVersion = "2"
				gw.Status.Conditions = []metav1.Condition{{Type: "Programmed", Status: metav1.ConditionTrue}}
			},
			want: true,
		},
		{name: "spec change", mutate: func(gw *gwv1.Gateway) { gw.Generation = 2 }, want: false},
		{name: "label change", mutate: func(gw *gwv1.Gateway) { gw.Labels = map[string]string{"a": "z"} }, want: false},
		{name: "annotation change", mutate: func(gw *gwv1.Gateway) { gw.Annotations = nil }, want: false},
		{name: "recreated", mutate: func(gw *gwv1.Gateway) { gw.UID = "other" }, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			changed := base.DeepCopy()
			tt.mutate(changed)
			a, b := gatewaySpec{gw: base}, gatewaySpec{gw: changed}
			g.Expect(a.Equals(b)).To(Equal(tt.want))
			g.Expect(b.Equals(a)).To(Equal(tt.want))
		})
	}
}

// TestGatewayIRIgnoresStatusOnlyWrites counts runs of the Envoy Gateway transformation:
// status-only writes to the Gateway or its ListenerSets must not re-run it, while spec,
// label and annotation changes must.
func TestGatewayIRIgnoresStatusOnlyWrites(t *testing.T) {
	g := NewWithT(t)
	gw := testGateway("gw")
	ls := testListenerSet("ls", "gw")
	h := newGatewayIndexHarness(t, gw, []*gwv1.ListenerSet{ls})

	steps := []struct {
		name      string
		write     func()
		wantRerun bool
	}{
		{
			name: "Gateway status",
			write: func() {
				gw = updated(gw, func(o *gwv1.Gateway) {
					o.Status.Conditions = []metav1.Condition{{Type: "Programmed", Status: metav1.ConditionTrue}}
				})
				h.gateways.UpdateObject(gw)
			},
		},
		{
			name: "ListenerSet status",
			write: func() {
				ls = updated(ls, func(o *gwv1.ListenerSet) {
					o.Status.Conditions = []metav1.Condition{{Type: "Accepted", Status: metav1.ConditionTrue}}
				})
				h.listenerSets.UpdateObject(ls)
			},
		},
		{
			name: "Gateway spec",
			write: func() {
				gw = bumpGeneration(gw)
				gw.Spec.Listeners[0].Name = "renamed"
				h.gateways.UpdateObject(gw)
			},
			wantRerun: true,
		},
		{
			name: "Gateway labels",
			write: func() {
				gw = updated(gw, func(o *gwv1.Gateway) { o.Labels = map[string]string{"team": "a"} })
				h.gateways.UpdateObject(gw)
			},
			wantRerun: true,
		},
		{
			name: "Gateway annotations",
			write: func() {
				gw = updated(gw, func(o *gwv1.Gateway) { o.Annotations = map[string]string{"note": "a"} })
				h.gateways.UpdateObject(gw)
			},
			wantRerun: true,
		},
		{
			name: "ListenerSet spec",
			write: func() {
				ls = bumpGeneration(ls)
				ls.Spec.Listeners[0].Port = 9090
				h.listenerSets.UpdateObject(ls)
			},
			wantRerun: true,
		},
		{
			name: "ListenerSet labels",
			write: func() {
				ls = updated(ls, func(o *gwv1.ListenerSet) { o.Labels = map[string]string{"team": "a"} })
				h.listenerSets.UpdateObject(ls)
			},
			wantRerun: true,
		},
		{
			name: "ListenerSet annotations",
			write: func() {
				ls = updated(ls, func(o *gwv1.ListenerSet) { o.Annotations = map[string]string{"note": "a"} })
				h.listenerSets.UpdateObject(ls)
			},
			wantRerun: true,
		},
	}
	for _, step := range steps {
		before := h.runs.Load()
		step.write()
		h.flushGateways()
		h.flushListenerSets()
		want := before
		if step.wantRerun {
			want++
		}
		g.Expect(h.runs.Load()).To(Equal(want), "%s write: wantRerun=%v", step.name, step.wantRerun)
	}

	// The IR reflects the latest spec of both objects.
	out := h.index.Gateways.GetKey(ir.ObjectSource{Group: gwv1.GroupName, Kind: wellknown.GatewayKind, Namespace: "default", Name: "gw"}.ResourceName())
	g.Expect(out).NotTo(BeNil())
	g.Expect(out.Obj.Generation).To(Equal(gw.Generation))
	g.Expect(out.Listeners).To(HaveLen(2))
	g.Expect(out.Listeners[0].Name).To(Equal(gwv1.SectionName("renamed")))
	g.Expect(out.Listeners[1].Port).To(Equal(gwv1.PortNumber(9090)))
}

// gatewayIndexHarness runs the Envoy Gateway transformation over static collections holding
// one Gateway under test, its ListenerSets, and a barrier Gateway with one ListenerSet of its
// own. Writes to the barrier objects re-run only the barrier Gateway. Collections deliver
// events in order, so once that run is seen, every earlier write to the same input was handled.
type gatewayIndexHarness struct {
	t            *testing.T
	index        *GatewayIndex
	gateways     krt.StaticCollection[*gwv1.Gateway]
	listenerSets krt.StaticCollection[*gwv1.ListenerSet]
	// runs counts transformation runs for the Gateway under test.
	runs        atomic.Int64
	barrierGW   *gwv1.Gateway
	barrierLS   *gwv1.ListenerSet
	barrierRuns chan struct{}
}

func newGatewayIndexHarness(t *testing.T, gw *gwv1.Gateway, listenerSets []*gwv1.ListenerSet) *gatewayIndexHarness {
	const barrier = "barrier"
	h := &gatewayIndexHarness{
		t:           t,
		barrierGW:   testGateway(barrier),
		barrierLS:   testListenerSet("barrier-ls", barrier),
		barrierRuns: make(chan struct{}, 16),
	}

	config := getConfig(t)
	h.gateways = krt.NewStaticCollection(nil, []*gwv1.Gateway{gw, h.barrierGW}, config.KrtOpts.ToOptions("Gateways")...)
	h.listenerSets = krt.NewStaticCollection(nil, append([]*gwv1.ListenerSet{h.barrierLS}, listenerSets...), config.KrtOpts.ToOptions("ListenerSets")...)
	config.Gateways = h.gateways
	config.ListenerSets = h.listenerSets
	config.GatewayClasses = krt.NewStaticCollection(nil, []*gwv1.GatewayClass{{
		ObjectMeta: metav1.ObjectMeta{Name: "envoy"},
		Spec:       gwv1.GatewayClassSpec{ControllerName: gwv1.GatewayController(config.EnvoyControllerName)},
	}}, config.KrtOpts.ToOptions("GatewayClasses")...)

	countRuns := func(config *GatewayIndexConfig) func(krt.HandlerContext, *gwv1.Gateway) *ir.Gateway {
		transform := GatewaysForEnvoyTransformationFunc(config)
		return func(kctx krt.HandlerContext, in *gwv1.Gateway) *ir.Gateway {
			out := transform(kctx, in)
			if in.Name == barrier {
				h.barrierRuns <- struct{}{}
			} else {
				h.runs.Add(1)
			}
			return out
		}
	}
	h.index = NewGatewayIndex(config, WithGatewayForEnvoyTransformationFunc(countRuns))
	if !h.index.Gateways.WaitUntilSynced(t.Context().Done()) {
		t.Fatal("Gateway IR collection did not sync")
	}
	h.awaitBarrier()
	return h
}

func (h *gatewayIndexHarness) flushGateways() {
	h.barrierGW = bumpGeneration(h.barrierGW)
	h.gateways.UpdateObject(h.barrierGW)
	h.awaitBarrier()
}

func (h *gatewayIndexHarness) flushListenerSets() {
	h.barrierLS = bumpGeneration(h.barrierLS)
	h.listenerSets.UpdateObject(h.barrierLS)
	h.awaitBarrier()
}

func (h *gatewayIndexHarness) awaitBarrier() {
	select {
	case <-h.barrierRuns:
	case <-time.After(time.Minute):
		h.t.Fatal("barrier Gateway was not re-run")
	}
}

func testGateway(name string) *gwv1.Gateway {
	return &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: types.UID(name), Generation: 1, ResourceVersion: "1"},
		Spec: gwv1.GatewaySpec{
			GatewayClassName: "envoy",
			Listeners:        []gwv1.Listener{{Name: "http", Port: 80, Protocol: gwv1.HTTPProtocolType}},
			AllowedListeners: &gwv1.AllowedListeners{Namespaces: &gwv1.ListenerNamespaces{From: new(gwv1.NamespacesFromSame)}},
		},
	}
}

func testListenerSet(name, parent string) *gwv1.ListenerSet {
	return &gwv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: types.UID(name), Generation: 1, ResourceVersion: "1"},
		Spec: gwv1.ListenerSetSpec{
			ParentRef: gwv1.ParentGatewayReference{Name: gwv1.ObjectName(parent)},
			Listeners: []gwv1.ListenerEntry{{Name: "http", Port: 8080, Protocol: gwv1.HTTPProtocolType}},
		},
	}
}

// updated returns a copy of obj as the API server would store it after a write that does not
// touch the spec: a new resourceVersion and the same generation.
func updated[T controllers.Object](obj T, mutate func(T)) T {
	out := obj.DeepCopyObject().(T)
	rv, _ := strconv.Atoi(out.GetResourceVersion())
	out.SetResourceVersion(strconv.Itoa(rv + 1))
	mutate(out)
	return out
}

// bumpGeneration returns a copy of obj as the API server would store it after a spec write.
func bumpGeneration[T controllers.Object](obj T) T {
	return updated(obj, func(o T) { o.SetGeneration(o.GetGeneration() + 1) })
}
