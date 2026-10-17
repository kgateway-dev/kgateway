package krtcollections

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/slices"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
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
	out := h.gatewayIR("gw")
	g.Expect(out).NotTo(BeNil())
	g.Expect(out.Obj.Generation).To(Equal(gw.Generation))
	g.Expect(out.Listeners).To(HaveLen(2))
	g.Expect(out.Listeners[0].Name).To(Equal(gwv1.SectionName("renamed")))
	g.Expect(out.Listeners[1].Port).To(Equal(gwv1.PortNumber(9090)))
}

// TestGatewayIRRebuildsOnlyChangedListenerSets counts policy lookups: creating, changing,
// deleting or attaching a policy to one ListenerSet looks up policies for that ListenerSet only,
// while the Gateway IR still carries every ListenerSet's listeners in precedence order.
func TestGatewayIRRebuildsOnlyChangedListenerSets(t *testing.T) {
	g := NewWithT(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	listenerSetAt := func(name string, age time.Duration) *gwv1.ListenerSet {
		ls := testListenerSet(name, "gw")
		ls.CreationTimestamp = metav1.NewTime(created.Add(age))
		return ls
	}
	// Precedence is creation time, then namespace/name, so the order is c, a, b.
	lsA, lsB, lsC := listenerSetAt("ls-a", time.Second), listenerSetAt("ls-b", time.Second), listenerSetAt("ls-c", 0)
	lsGone := listenerSetAt("ls-gone", 0)
	denied := listenerSetAt("denied", 0)
	denied.Namespace = "other"
	denied.Spec.ParentRef.Namespace = new(gwv1.Namespace("default"))
	h := newGatewayIndexHarness(t, testGateway("gw"), []*gwv1.ListenerSet{lsA, lsB, lsC, lsGone, denied}, testPolicy("policy-a", "ls-a"))

	steps := []struct {
		name  string
		write func()
		// rebuilt is how many ListenerSet IRs the write should rebuild.
		rebuilt int64
	}{
		{
			name:    "create a ListenerSet",
			write:   func() { h.listenerSets.UpdateObject(listenerSetAt("ls-d", 2*time.Second)) },
			rebuilt: 1,
		},
		{
			name: "change a ListenerSet's spec",
			write: func() {
				lsB = bumpGeneration(lsB)
				lsB.Spec.Listeners[0].Port = 9090
				h.listenerSets.UpdateObject(lsB)
			},
			rebuilt: 1,
		},
		{
			name:    "attach a policy to a ListenerSet",
			write:   func() { h.policies.UpdateObject(testPolicy("policy-c", "ls-c")) },
			rebuilt: 1,
		},
		{
			name:  "delete a ListenerSet",
			write: func() { h.listenerSets.DeleteObject(krt.GetKey(lsGone)) },
		},
	}
	for _, step := range steps {
		runs, lookups := h.runs.Load(), h.lookups.Load()
		step.write()
		h.flushListenerSets()
		h.flushPolicies()
		g.Expect(h.runs.Load()).To(Equal(runs+1), "%s: Gateway should re-run once", step.name)
		// Every Gateway and ListenerSet here has one listener, so building either looks up
		// policies twice. Besides the rebuilt ListenerSets, a step runs the Gateway once and the
		// barrier Gateway twice, and both flushes rebuild the barrier ListenerSet: 10 lookups.
		g.Expect(h.lookups.Load()-lookups).To(Equal(2*step.rebuilt+10), "%s: only the changed ListenerSet should look up policies", step.name)
	}

	out := h.gatewayIR("gw")
	g.Expect(out).NotTo(BeNil())
	type listenerView struct {
		Parent   string
		Port     gwv1.PortNumber
		Policies []string
	}
	viewListeners := func(listeners []ir.Listener) []listenerView {
		return slices.Map(listeners, func(l ir.Listener) listenerView {
			var names []string
			for _, atts := range l.AttachedPolicies.Policies {
				for _, att := range atts {
					names = append(names, att.PolicyRef.Name)
				}
			}
			return listenerView{Parent: l.Parent.GetName(), Port: l.Port, Policies: names}
		})
	}
	g.Expect(viewListeners(out.Listeners)).To(Equal([]listenerView{
		{Parent: "gw", Port: 80},
		{Parent: "ls-c", Port: 8080, Policies: []string{"policy-c"}},
		{Parent: "ls-a", Port: 8080, Policies: []string{"policy-a"}},
		{Parent: "ls-b", Port: 9090},
		{Parent: "ls-d", Port: 8080},
	}), "Gateway listeners first, then admitted ListenerSets' listeners in precedence order")

	listenerSetNames := func(sets ir.ListenerSets) []string {
		return slices.Map(sets, func(ls ir.ListenerSet) string { return ls.Name })
	}
	g.Expect(listenerSetNames(out.AllowedListenerSets[wellknown.ListenerSetGVK])).To(Equal([]string{"ls-c", "ls-a", "ls-b", "ls-d"}))
	deniedSets := out.DeniedListenerSets[wellknown.ListenerSetGVK]
	g.Expect(listenerSetNames(deniedSets)).To(Equal([]string{"denied"}), "a ListenerSet from another namespace is not allowed by From: Same")
	g.Expect(deniedSets[0].Err).To(MatchError("Attachment not allowed"))
	g.Expect(viewListeners(deniedSets[0].Listeners)).To(Equal([]listenerView{{Parent: "denied", Port: 8080}}), "a denied ListenerSet still carries its listeners for status")
}

// gatewayIndexHarness runs the Envoy Gateway transformation over static collections holding
// one Gateway under test, its ListenerSets and policies, and a barrier Gateway with one
// ListenerSet and one policy of its own. Writes to the barrier objects re-run only the barrier
// Gateway. Collections deliver events in order, so once that run is seen, every earlier write
// to the same input was handled.
type gatewayIndexHarness struct {
	t            *testing.T
	index        *GatewayIndex
	gateways     krt.StaticCollection[*gwv1.Gateway]
	listenerSets krt.StaticCollection[*gwv1.ListenerSet]
	policies     krt.StaticCollection[ir.PolicyWrapper]
	// runs counts transformation runs for the Gateway under test.
	runs atomic.Int64
	// lookups counts policy lookups, by the transformation and the ListenerSet IRs alike.
	lookups       atomic.Int64
	barrierGW     *gwv1.Gateway
	barrierLS     *gwv1.ListenerSet
	barrierPolicy ir.PolicyWrapper
	barrierRuns   chan struct{}
}

func newGatewayIndexHarness(t *testing.T, gw *gwv1.Gateway, listenerSets []*gwv1.ListenerSet, policies ...ir.PolicyWrapper) *gatewayIndexHarness {
	const barrier = "barrier"
	h := &gatewayIndexHarness{
		t:             t,
		barrierGW:     testGateway(barrier),
		barrierLS:     testListenerSet("barrier-ls", barrier),
		barrierPolicy: testPolicy("barrier-policy", "barrier-ls"),
		barrierRuns:   make(chan struct{}, 16),
	}

	config := getConfig(t)
	h.gateways = krt.NewStaticCollection(nil, []*gwv1.Gateway{gw, h.barrierGW}, config.KrtOpts.ToOptions("Gateways")...)
	h.listenerSets = krt.NewStaticCollection(nil, append([]*gwv1.ListenerSet{h.barrierLS}, listenerSets...), config.KrtOpts.ToOptions("ListenerSets")...)
	h.policies = krt.NewStaticCollection(nil, append([]ir.PolicyWrapper{h.barrierPolicy}, policies...), config.KrtOpts.ToOptions("Policies")...)
	config.Gateways = h.gateways
	config.ListenerSets = h.listenerSets
	config.PolicyIndex = NewPolicyIndex(config.KrtOpts, sdk.ContributesPolicies{
		wellknown.TrafficPolicyGVK.GroupKind(): {
			Policies: h.policies,
			// Runs once per policy lookup, so it counts them.
			GlobalPolicies: func(krt.HandlerContext) ir.PolicyIR {
				h.lookups.Add(1)
				return nil
			},
		},
	}, settings.Settings{})
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

func (h *gatewayIndexHarness) gatewayIR(name string) *ir.Gateway {
	return h.index.Gateways.GetKey(ir.ObjectSource{Group: gwv1.GroupName, Kind: wellknown.GatewayKind, Namespace: "default", Name: name}.ResourceName())
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

func (h *gatewayIndexHarness) flushPolicies() {
	h.barrierPolicy.Policy = bumpGeneration(h.barrierPolicy.Policy.(*kgateway.TrafficPolicy))
	h.policies.UpdateObject(h.barrierPolicy)
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

// testPolicy returns a TrafficPolicy that targets the named ListenerSet.
func testPolicy(name, listenerSet string) ir.PolicyWrapper {
	return ir.PolicyWrapper{
		ObjectSource: ir.ObjectSource{Group: wellknown.TrafficPolicyGVK.Group, Kind: wellknown.TrafficPolicyGVK.Kind, Namespace: "default", Name: name},
		Policy:       &kgateway.TrafficPolicy{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", UID: types.UID(name), Generation: 1, ResourceVersion: "1"}},
		PolicyIR:     testPolicyIR{},
		TargetRefs:   []ir.PolicyRef{{Group: wellknown.GatewayGroup, Kind: wellknown.ListenerSetKind, Name: listenerSet}},
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
