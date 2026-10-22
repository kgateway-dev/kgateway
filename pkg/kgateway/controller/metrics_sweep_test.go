package controller

import (
	"context"
	"testing"

	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

func TestMetricsSweeperUsesInformerCaches(t *testing.T) {
	ResetMetrics()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	gateways := krt.NewStaticCollection[*gwv1.Gateway](nil,
		[]*gwv1.Gateway{{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "live"}}}, krtopts.ToOptions("Gateways")...)
	sweeper := newMetricsSweeper(gateways)

	for _, name := range []string{"live", "deleted"} {
		collectReconciliationMetrics("gateway", types.NamespacedName{Namespace: "ns", Name: name})(nil)
	}
	collectReconciliationMetrics("gatewayclass", types.NamespacedName{Name: "class"})(nil)
	sweeper.Sweep()

	// The sweep keeps the series of a Gateway in the cache and of the cluster-scoped
	// GatewayClass, and removes the others. ResetMetrics leaves a series with empty labels.
	metricstest.MustGatherMetrics(t).AssertMetricsLabels("kgateway_controller_reconciliations_total", [][]metrics.Label{
		{{Name: controllerNameLabel, Value: ""}, {Name: nameLabel, Value: ""}, {Name: namespaceLabel, Value: ""}, {Name: resultLabel, Value: ""}},
		{{Name: controllerNameLabel, Value: "gateway"}, {Name: nameLabel, Value: "live"}, {Name: namespaceLabel, Value: "ns"}, {Name: resultLabel, Value: "success"}},
		{{Name: controllerNameLabel, Value: "gatewayclass"}, {Name: nameLabel, Value: "class"}, {Name: namespaceLabel, Value: ""}, {Name: resultLabel, Value: "success"}},
	})
}
