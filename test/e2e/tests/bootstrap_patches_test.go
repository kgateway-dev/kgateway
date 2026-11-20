//go:build e2e

package tests

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/bootstrappatches"
)

// TestBootstrapPatches tests GatewayParameters bootstrap patches against running proxies.
func TestBootstrapPatches(t *testing.T) {
	bootstrappatches.Run(t, e2e.DefaultInstallationFactory)
}
