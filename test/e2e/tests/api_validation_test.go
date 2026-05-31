//go:build e2e

package tests

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/apivalidation"
)

// TestAPIValidation exercises CRD-level validation rules.
func TestAPIValidation(t *testing.T) {
	apivalidation.Run(t, e2e.DefaultInstallationFactory)
}
