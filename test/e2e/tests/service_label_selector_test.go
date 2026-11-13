//go:build e2e

package tests

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/servicelabelselector"
)

// TestServiceLabelSelector tests the Service label selector in a dedicated installation,
// because the selector applies to every Gateway managed by that controller.
func TestServiceLabelSelector(t *testing.T) {
	servicelabelselector.Run(t, e2e.DefaultInstallationFactory)
}
