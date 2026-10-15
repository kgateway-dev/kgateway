package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// reconciledGateways returns the namespace/name of every Gateway with a reconcile series.
func reconciledGateways(t *testing.T) []string {
	t.Helper()
	families, err := metrics.Registry().Gather()
	require.NoError(t, err)
	var out []string
	for _, mf := range families {
		if mf.GetName() != "kgateway_controller_reconciliations_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			labels := map[string]string{}
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			if labels[controllerNameLabel] == "gateway" {
				out = append(out, labels[namespaceLabel]+"/"+labels[nameLabel])
			}
		}
	}
	return out
}

func TestMetricsSweeperUsesInformerCaches(t *testing.T) {
	ResetMetrics()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	namespaces := krt.NewStaticCollection[krtcollections.NamespaceMetadata](nil,
		[]krtcollections.NamespaceMetadata{{Name: "ns"}}, krtopts.ToOptions("Namespaces")...)
	gateways := krt.NewStaticCollection[*gwv1.Gateway](nil,
		[]*gwv1.Gateway{{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "live"}}}, krtopts.ToOptions("Gateways")...)
	sweeper, synced := newMetricsSweeper(namespaces, gateways)
	require.True(t, synced(), "the sweeper should run once both caches have synced")

	for _, name := range []string{"live", "deleted"} {
		collectReconciliationMetrics("gateway", types.NamespacedName{Namespace: "ns", Name: name})(nil)
	}
	sweeper.Sweep()

	require.Equal(t, []string{"ns/live"}, reconciledGateways(t),
		"the sweep should keep the series of a Gateway in the cache and remove the others")
}
