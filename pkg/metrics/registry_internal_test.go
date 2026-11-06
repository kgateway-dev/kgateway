package metrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientmetrics "k8s.io/client-go/tools/metrics"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	// Registers controller-runtime's controller metrics, including its own go
	// and process collectors, as kgateway's manager does.
	_ "sigs.k8s.io/controller-runtime/pkg/controller"
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

	// Serve the registry the way controller-runtime's metrics server does.
	srv := httptest.NewServer(promhttp.HandlerFor(ctrlmetrics.Registry, promhttp.HandlerOpts{
		ErrorHandling: promhttp.HTTPErrorOnError,
	}))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "metrics endpoint should not fail: %s", body)

	for _, name := range []string{
		"kgateway_registry_test_info",
		"rest_client_requests_total",
		"go_goroutines",
		"process_cpu_seconds_total",
	} {
		assert.Contains(t, string(body), "\n# TYPE "+name+" ", "%s should be served", name)
	}
}
