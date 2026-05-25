//go:build e2e

package tests

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/customgwp"
)

// TestCustomGWP tests that the helm chart's gatewayClassParametersRefs
// configures the default GatewayClass parametersRef correctly.
func TestCustomGWP(t *testing.T) {
	customgwp.Run(t, e2e.DefaultInstallationFactory)
}
