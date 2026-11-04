//go:build e2e

package tests

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/kgateway"
)

// TestKgateway runs the main kgateway feature suites against a single
// installation.
func TestKgateway(t *testing.T) {
	kgateway.Run(t, e2e.DefaultInstallationFactory)
}
