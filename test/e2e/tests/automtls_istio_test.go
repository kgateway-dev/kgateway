//go:build e2e

package tests

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/automtlsistio"
)

// TestKgatewayIstioAutoMtls tests Istio auto-mTLS integration.
func TestKgatewayIstioAutoMtls(t *testing.T) {
	automtlsistio.Run(t, e2e.DefaultInstallationFactory)
}
