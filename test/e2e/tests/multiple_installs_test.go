//go:build e2e

package tests_test

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/multipleinstalls"
)

// TestMultipleInstalls installs kgateway twice into different namespaces and
// runs the multiinstall feature suite against each.
func TestMultipleInstalls(t *testing.T) {
	multipleinstalls.Run(t, e2e.DefaultInstallationFactory)
}
