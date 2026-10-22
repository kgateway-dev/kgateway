package metrics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
)

func gwLabels(namespace, name string) []Label {
	return []Label{{Name: "namespace", Value: namespace}, {Name: "gateway", Value: name}}
}

func TestSweepRemovesSeriesOfDeletedGateways(t *testing.T) {
	setupTestRegistry()
	ResetScoped()

	counter := NewCounter(CounterOpts{Name: "sweep_gateway_total", Help: "test"}, []string{"gateway", "namespace"})
	histogram := NewHistogram(HistogramOpts{Name: "sweep_gateway_seconds", Help: "test"}, []string{"gateway", "namespace"})
	RegisterGatewayScoped(counter, "namespace", "gateway")
	RegisterGatewayScoped(histogram, "namespace", "gateway")

	for _, gw := range [][]Label{gwLabels("ns", "live"), gwLabels("ns", "deleted"), gwLabels("other", "live")} {
		counter.Inc(gw...)
		histogram.Observe(1, gw...)
	}

	live := map[string]bool{"ns/live": true, "other/live": true}
	sweeper := Sweeper{
		GatewayExists: func(namespace, name string) bool { return live[namespace+"/"+name] },
	}

	assert.Equal(t, 2, sweeper.Sweep(), "the counter and histogram series of ns/deleted should be removed")

	gathered := metricstest.MustGatherMetrics(t)
	gathered.AssertMetricsLabels("kgateway_sweep_gateway_total", [][]Label{gwLabels("ns", "live"), gwLabels("other", "live")})
	gathered.AssertMetricsLabels("kgateway_sweep_gateway_seconds", [][]Label{gwLabels("ns", "live"), gwLabels("other", "live")})

	// A late writer re-creates the series; the next pass removes it again.
	counter.Inc(gwLabels("ns", "deleted")...)
	assert.Equal(t, 1, sweeper.Sweep(), "a series re-created after a pass should be removed by the next one")
	assert.Equal(t, 0, sweeper.Sweep(), "nothing should be left to remove")
}

func TestRegisterScopedPanicsOnMissingLabel(t *testing.T) {
	setupTestRegistry()
	ResetScoped()

	counter := NewCounter(CounterOpts{Name: "sweep_bad_total", Help: "test"}, []string{"namespace"})

	assert.PanicsWithValue(t, "metric does not have label gateway", func() {
		RegisterGatewayScoped(counter, "namespace", "gateway")
	})
}
