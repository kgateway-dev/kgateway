package krtcollections

import (
	"strconv"
	"sync"
	"testing"

	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

func TestSpecVersionEquals(t *testing.T) {
	base := &gwv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{
		Name:            "ls",
		Namespace:       "default",
		UID:             "uid",
		Generation:      1,
		ResourceVersion: "1",
		Labels:          map[string]string{"a": "b"},
		Annotations:     map[string]string{"c": "d"},
	}}
	tests := []struct {
		name   string
		mutate func(*gwv1.ListenerSet)
		want   bool
	}{
		{
			name: "status-only write",
			mutate: func(ls *gwv1.ListenerSet) {
				ls.ResourceVersion = "2"
				ls.Status.Conditions = []metav1.Condition{{Type: "Accepted", Status: metav1.ConditionTrue}}
			},
			want: true,
		},
		{name: "spec change", mutate: func(ls *gwv1.ListenerSet) { ls.Generation = 2 }, want: false},
		{name: "label change", mutate: func(ls *gwv1.ListenerSet) { ls.Labels = map[string]string{"a": "z"} }, want: false},
		{name: "annotation change", mutate: func(ls *gwv1.ListenerSet) { ls.Annotations = nil }, want: false},
		{name: "recreated", mutate: func(ls *gwv1.ListenerSet) { ls.UID = "other" }, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			changed := base.DeepCopy()
			tt.mutate(changed)
			a, b := specVersion[*gwv1.ListenerSet]{obj: base}, specVersion[*gwv1.ListenerSet]{obj: changed}
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
	krtOpts := krtutil.NewKrtOptions(t.Context().Done(), nil)

	const controller = "envoy-controller"
	gw := testGateway("gw")
	ls := testListenerSet("ls", "gw")
	// Writes to the barrier objects re-run only the barrier Gateway. Collections deliver events
	// in order, so once that run is seen, every earlier write to the same input was handled.
	barrierGW := testGateway("barrier")
	barrierLS := testListenerSet("barrier-ls", "barrier")

	gateways := krt.NewStaticCollection(nil, []*gwv1.Gateway{gw, barrierGW}, krtOpts.ToOptions("Gateways")...)
	listenerSets := krt.NewStaticCollection(nil, []*gwv1.ListenerSet{ls, barrierLS}, krtOpts.ToOptions("ListenerSets")...)
	gatewayClasses := krt.NewStaticCollection(nil, []*gwv1.GatewayClass{{
		ObjectMeta: metav1.ObjectMeta{Name: "envoy"},
		Spec:       gwv1.GatewayClassSpec{ControllerName: controller},
	}}, krtOpts.ToOptions("GatewayClasses")...)

	var mu sync.Mutex
	runs := map[string]int{}
	countRuns := func(config *GatewayIndexConfig) func(krt.HandlerContext, *gwv1.Gateway) *ir.Gateway {
		transform := GatewaysForEnvoyTransformationFunc(config)
		return func(kctx krt.HandlerContext, gw *gwv1.Gateway) *ir.Gateway {
			mu.Lock()
			runs[gw.Name]++
			mu.Unlock()
			return transform(kctx, gw)
		}
	}
	runsOf := func(name string) int {
		mu.Lock()
		defer mu.Unlock()
		return runs[name]
	}

	index := NewGatewayIndex(GatewayIndexConfig{
		KrtOpts:             krtOpts,
		EnvoyControllerName: controller,
		PolicyIndex:         NewPolicyIndex(krtOpts, nil, settings.Settings{}),
		Gateways:            gateways,
		ListenerSets:        listenerSets,
		GatewayClasses:      gatewayClasses,
		Namespaces:          krt.NewStaticCollection[NamespaceMetadata](nil, nil, krtOpts.ToOptions("Namespaces")...),
	}, WithGatewayForEnvoyTransformationFunc(countRuns))
	g.Expect(index.Gateways.WaitUntilSynced(t.Context().Done())).To(BeTrue())

	flush := func() {
		before := runsOf("barrier")
		barrierGW = bumpGeneration(barrierGW)
		gateways.UpdateObject(barrierGW)
		g.Eventually(func() int { return runsOf("barrier") }).Should(Equal(before+1), "barrier Gateway write should re-run the barrier Gateway")
		barrierLS = bumpGeneration(barrierLS)
		listenerSets.UpdateObject(barrierLS)
		g.Eventually(func() int { return runsOf("barrier") }).Should(Equal(before+2), "barrier ListenerSet write should re-run the barrier Gateway")
	}
	gatewayIR := func() *ir.Gateway {
		return index.Gateways.GetKey(ir.ObjectSource{Group: gwv1.GroupName, Kind: wellknown.GatewayKind, Namespace: "default", Name: "gw"}.ResourceName())
	}
	// A late ListenerSet event may still re-run the Gateway once after the initial sync.
	flush()

	steps := []struct {
		name      string
		write     func()
		wantRerun bool
	}{
		{
			name: "Gateway status",
			write: func() {
				gw = writeStatus(gw, func(o *gwv1.Gateway) {
					o.Status.Conditions = []metav1.Condition{{Type: "Programmed", Status: metav1.ConditionTrue}}
				})
				gateways.UpdateObject(gw)
			},
		},
		{
			name: "ListenerSet status",
			write: func() {
				ls = writeStatus(ls, func(o *gwv1.ListenerSet) {
					o.Status.Conditions = []metav1.Condition{{Type: "Accepted", Status: metav1.ConditionTrue}}
				})
				listenerSets.UpdateObject(ls)
			},
		},
		{
			name: "Gateway spec",
			write: func() {
				gw = bumpGeneration(gw)
				gw.Spec.Listeners[0].Name = "renamed"
				gateways.UpdateObject(gw)
			},
			wantRerun: true,
		},
		{
			name: "Gateway labels",
			write: func() {
				gw = writeStatus(gw, func(o *gwv1.Gateway) { o.Labels = map[string]string{"team": "a"} })
				gateways.UpdateObject(gw)
			},
			wantRerun: true,
		},
		{
			name: "Gateway annotations",
			write: func() {
				gw = writeStatus(gw, func(o *gwv1.Gateway) { o.Annotations = map[string]string{"note": "a"} })
				gateways.UpdateObject(gw)
			},
			wantRerun: true,
		},
		{
			name: "ListenerSet spec",
			write: func() {
				ls = bumpGeneration(ls)
				ls.Spec.Listeners[0].Port = 9090
				listenerSets.UpdateObject(ls)
			},
			wantRerun: true,
		},
		{
			name: "ListenerSet labels",
			write: func() {
				ls = writeStatus(ls, func(o *gwv1.ListenerSet) { o.Labels = map[string]string{"team": "a"} })
				listenerSets.UpdateObject(ls)
			},
			wantRerun: true,
		},
		{
			name: "ListenerSet annotations",
			write: func() {
				ls = writeStatus(ls, func(o *gwv1.ListenerSet) { o.Annotations = map[string]string{"note": "a"} })
				listenerSets.UpdateObject(ls)
			},
			wantRerun: true,
		},
	}
	for _, step := range steps {
		before := runsOf("gw")
		step.write()
		flush()
		want := before
		if step.wantRerun {
			want++
		}
		g.Expect(runsOf("gw")).To(Equal(want), "%s write: wantRerun=%v", step.name, step.wantRerun)
	}

	// The IR still reflects the latest spec of both objects.
	out := gatewayIR()
	g.Expect(out).NotTo(BeNil())
	g.Expect(out.Obj.Generation).To(Equal(gw.Generation))
	g.Expect(out.Listeners).To(HaveLen(2))
	g.Expect(out.Listeners[0].Name).To(Equal(gwv1.SectionName("renamed")))
	g.Expect(out.Listeners[1].Port).To(Equal(gwv1.PortNumber(9090)))
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

type versionedObject[T any] interface {
	*T
	metav1.Object
	DeepCopy() *T
}

// writeStatus returns a copy of obj as the API server would store it after a write that
// does not touch the spec: a new resourceVersion and the same generation.
func writeStatus[T any, PT versionedObject[T]](obj PT, mutate func(PT)) PT {
	out := PT(obj.DeepCopy())
	rv, _ := strconv.Atoi(out.GetResourceVersion())
	out.SetResourceVersion(strconv.Itoa(rv + 1))
	mutate(out)
	return out
}

// bumpGeneration returns a copy of obj as the API server would store it after a spec write.
func bumpGeneration[T any, PT versionedObject[T]](obj PT) PT {
	return writeStatus(obj, func(o PT) { o.SetGeneration(o.GetGeneration() + 1) })
}
