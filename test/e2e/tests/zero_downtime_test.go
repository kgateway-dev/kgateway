//go:build e2e

package tests

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/zerodowntime"
)

// TestZeroDowntimeRollout tests that proxy rollouts don't drop traffic.
func TestZeroDowntimeRollout(t *testing.T) {
	zerodowntime.Run(t, e2e.DefaultInstallationFactory)
}
