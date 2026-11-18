//go:build e2e

package tests

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/waypoint"
)

// TestKgatewayWaypoint tests kgateway ingress traffic to Istio ambient mesh waypoints.
func TestKgatewayWaypoint(t *testing.T) {
	waypoint.Run(t, e2e.DefaultInstallationFactory)
}
