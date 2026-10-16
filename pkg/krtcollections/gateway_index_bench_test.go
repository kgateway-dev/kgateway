package krtcollections

import (
	"strconv"
	"sync/atomic"
	"testing"

	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// BenchmarkGatewayIRStatusWrite measures what one status-only write to a Gateway, or to one of
// its ListenerSets, costs the Envoy Gateway transformation when 2,500 ListenerSets are attached.
func BenchmarkGatewayIRStatusWrite(b *testing.B) {
	for _, target := range []string{"Gateway", "ListenerSet"} {
		b.Run(target, func(b *testing.B) {
			const numListenerSets = 2500
			krtOpts := krtutil.NewKrtOptions(b.Context().Done(), nil)

			gw := testGateway("gw")
			// Writes to the barrier objects re-run only the barrier Gateway; once that run is
			// seen, every earlier write to the same input has been handled.
			barrierGW := testGateway("barrier")
			barrierLS := testListenerSet("barrier-ls", "barrier")
			listenerSets := []*gwv1.ListenerSet{barrierLS}
			for i := range numListenerSets {
				listenerSets = append(listenerSets, testListenerSet("ls-"+strconv.Itoa(i), "gw"))
			}
			gatewayCol := krt.NewStaticCollection(nil, []*gwv1.Gateway{gw, barrierGW}, krtOpts.ToOptions("Gateways")...)
			listenerSetCol := krt.NewStaticCollection(nil, listenerSets, krtOpts.ToOptions("ListenerSets")...)

			var gwRuns atomic.Int64
			barrierRuns := make(chan struct{}, 16)
			countRuns := func(config *GatewayIndexConfig) func(krt.HandlerContext, *gwv1.Gateway) *ir.Gateway {
				transform := GatewaysForEnvoyTransformationFunc(config)
				return func(kctx krt.HandlerContext, gw *gwv1.Gateway) *ir.Gateway {
					out := transform(kctx, gw)
					if gw.Name == "barrier" {
						barrierRuns <- struct{}{}
					} else {
						gwRuns.Add(1)
					}
					return out
				}
			}
			index := NewGatewayIndex(GatewayIndexConfig{
				KrtOpts:             krtOpts,
				EnvoyControllerName: "envoy-controller",
				PolicyIndex:         NewPolicyIndex(krtOpts, nil, settings.Settings{}),
				Gateways:            gatewayCol,
				ListenerSets:        listenerSetCol,
				GatewayClasses: krt.NewStaticCollection(nil, []*gwv1.GatewayClass{{
					ObjectMeta: metav1.ObjectMeta{Name: "envoy"},
					Spec:       gwv1.GatewayClassSpec{ControllerName: "envoy-controller"},
				}}, krtOpts.ToOptions("GatewayClasses")...),
				Namespaces: krt.NewStaticCollection[NamespaceMetadata](nil, nil, krtOpts.ToOptions("Namespaces")...),
			}, WithGatewayForEnvoyTransformationFunc(countRuns))
			if !index.Gateways.WaitUntilSynced(b.Context().Done()) {
				b.Fatal("Gateway IR collection did not sync")
			}
			<-barrierRuns

			flushGateways := func() {
				barrierGW = bumpGeneration(barrierGW)
				gatewayCol.UpdateObject(barrierGW)
				<-barrierRuns
			}
			flushListenerSets := func() {
				barrierLS = bumpGeneration(barrierLS)
				listenerSetCol.UpdateObject(barrierLS)
				<-barrierRuns
			}
			flushListenerSets()
			flushGateways()

			ls := listenerSets[1]
			gwRuns.Store(0)
			for b.Loop() {
				switch target {
				case "Gateway":
					gw = writeStatus(gw, func(o *gwv1.Gateway) { o.Status.Conditions = nil })
					gatewayCol.UpdateObject(gw)
					flushGateways()
				case "ListenerSet":
					ls = writeStatus(ls, func(o *gwv1.ListenerSet) { o.Status.Conditions = nil })
					listenerSetCol.UpdateObject(ls)
					flushListenerSets()
				}
			}
			b.ReportMetric(float64(gwRuns.Load())/float64(b.N), "transform-runs/op")
		})
	}
}
