//go:build e2e

package tests

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/metrics"
)

// TestKgatewayMetrics tests kgateway metrics in a dedicated installation so
// metric values are not affected by other tests.
func TestKgatewayMetrics(t *testing.T) {
	metrics.Run(t, e2e.DefaultInstallationFactory)
}
