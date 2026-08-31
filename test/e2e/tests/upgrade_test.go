//go:build e2e

package tests

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/upgrade"
)

// TestUpgrade tests upgrading kgateway from the latest release and from the
// previous minor release to the locally-built chart.
func TestUpgrade(t *testing.T) {
	upgrade.Run(t, e2e.DefaultInstallationFactory)
}
