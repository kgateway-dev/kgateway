//go:build e2e

package tests

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/bootstrapoverlays"
)

// TestBootstrapOverlays tests GatewayParameters bootstrap overlays against running proxies.
func TestBootstrapOverlays(t *testing.T) {
	bootstrapoverlays.Run(t, e2e.DefaultInstallationFactory)
}
