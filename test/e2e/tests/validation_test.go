//go:build e2e

package tests

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/routereplacement"
)

// TestRouteReplacement tests route replacement under strict validation.
func TestRouteReplacement(t *testing.T) {
	routereplacement.Run(t, e2e.DefaultInstallationFactory)
}
