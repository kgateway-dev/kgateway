//go:build e2e

package tests

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/controlplanetls"
)

// TestControlPlaneTLS tests the TLS control plane integration functionality.
// This test requires a dedicated installation with TLS enabled for xDS communication.
func TestControlPlaneTLS(t *testing.T) {
	controlplanetls.Run(t, e2e.DefaultInstallationFactory)
}
