package metrics

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientmetrics "k8s.io/client-go/tools/metrics"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func TestSetRegistryBuiltinKeepsInitRegisteredMetrics(t *testing.T) {
	oldRegistry, oldCtrlRegistry := registry, ctrlmetrics.Registry
	t.Cleanup(func() {
		registry, ctrlmetrics.Registry = oldRegistry, oldCtrlRegistry
	})

	// Recreate the package init state, since other tests replace registry.
	local := prometheus.NewRegistry()
	local.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	local.MustRegister(collectors.NewGoCollector())
	registry = local
	g := NewGauge(GaugeOpts{Name: "registry_test_info", Help: "test"}, []string{"label"})
	g.Set(1, Label{Name: "label", Value: "value"})

	// controller-runtime registers rest_client_requests_total at init.
	clientmetrics.RequestResult.Increment(context.Background(), "200", "GET", "example.com")

	SetRegistry(true, nil)

	assert.Same(t, local, Registry(), "kgateway metrics should keep using the local registry")

	families, err := ctrlmetrics.Registry.Gather()
	require.NoError(t, err)
	names := map[string]bool{}
	for _, f := range families {
		names[f.GetName()] = true
	}
	assert.True(t, names["kgateway_registry_test_info"], "kgateway metric registered before SetRegistry should be served")
	assert.True(t, names["rest_client_requests_total"], "controller-runtime metrics should be served")
	assert.True(t, names["go_goroutines"], "go runtime metrics should be served")
	assert.True(t, names["process_cpu_seconds_total"], "process metrics should be served")
}
