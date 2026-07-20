//go:build e2e

package tests

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/waypoint"
)

// TestKgatewayWaypoint tests kgateway acting as an ambient mesh waypoint.
func TestKgatewayWaypoint(t *testing.T) {
	waypoint.Run(t, e2e.DefaultInstallationFactory)
}
