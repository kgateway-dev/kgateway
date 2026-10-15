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
		NamespaceExists: func(string) bool { return true },
		GatewayExists:   func(namespace, name string) bool { return live[namespace+"/"+name] },
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

func TestSweepOnlyMatchingSeries(t *testing.T) {
	setupTestRegistry()
	ResetScoped()

	counter := NewCounter(CounterOpts{Name: "sweep_controller_total", Help: "test"}, []string{"controller", "name", "namespace"})
	RegisterGatewayScoped(counter, "namespace", "name", Label{Name: "controller", Value: "gateway"})

	gateway := []Label{{Name: "controller", Value: "gateway"}, {Name: "name", Value: "gw"}, {Name: "namespace", Value: "ns"}}
	class := []Label{{Name: "controller", Value: "gatewayclass"}, {Name: "name", Value: "gc"}, {Name: "namespace", Value: ""}}
	otherController := []Label{{Name: "controller", Value: "other"}, {Name: "name", Value: "gw"}, {Name: "namespace", Value: "ns"}}
	counter.Inc(gateway...)
	counter.Inc(class...)
	counter.Inc(otherController...)

	sweeper := Sweeper{
		NamespaceExists: func(string) bool { return true },
		GatewayExists:   func(string, string) bool { return false },
	}

	assert.Equal(t, 1, sweeper.Sweep(), "only the gateway controller's series should be removed")
	metricstest.MustGatherMetrics(t).AssertMetricsLabels("kgateway_sweep_controller_total", [][]Label{class, otherController})
}

func TestSweepRemovesSeriesOfDeletedNamespaces(t *testing.T) {
	setupTestRegistry()
	ResetScoped()

	gauge := NewGauge(GaugeOpts{Name: "sweep_namespace", Help: "test"}, []string{"namespace", "parent"})
	RegisterNamespaceScoped(gauge, "namespace")

	nsLabels := func(namespace, parent string) []Label {
		return []Label{{Name: "namespace", Value: namespace}, {Name: "parent", Value: parent}}
	}
	gauge.Set(1, nsLabels("live", "a")...)
	gauge.Set(1, nsLabels("deleted", "a")...)
	gauge.Set(1, nsLabels("deleted", "b")...)
	gauge.Set(1, nsLabels("", "cluster")...)

	sweeper := Sweeper{
		NamespaceExists: func(namespace string) bool { return namespace == "live" },
		GatewayExists:   func(string, string) bool { return true },
	}

	assert.Equal(t, 2, sweeper.Sweep(), "both series of the deleted namespace should be removed")
	metricstest.MustGatherMetrics(t).AssertMetricsLabels("kgateway_sweep_namespace", [][]Label{nsLabels("", "cluster"), nsLabels("live", "a")})
}

func TestRegisterScopedPanicsOnMissingLabel(t *testing.T) {
	setupTestRegistry()
	ResetScoped()

	counter := NewCounter(CounterOpts{Name: "sweep_bad_total", Help: "test"}, []string{"namespace"})

	assert.PanicsWithValue(t, "metric does not have label gateway", func() {
		RegisterGatewayScoped(counter, "namespace", "gateway")
	})
	assert.PanicsWithValue(t, "metric does not have label ns", func() {
		RegisterNamespaceScoped(counter, "ns")
	})
}
