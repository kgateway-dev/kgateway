//go:build e2e

package tests_test

import (
	"context"
	"os"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/features/dualcontroller"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// TestDualController tests the dual controller architecture with both controllers starting disabled
// This is a separate test from TestKgateway because it needs custom installation flags
func TestDualController(t *testing.T) {
	ctx := context.Background()
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "kgateway-test")
	testInstallation := e2e.CreateTestInstallation(
		t,
		&install.Context{
			InstallNamespace:          installNs,
			ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
			ValuesManifestFile:        e2e.EmptyValuesManifestPath,
			ExtraHelmArgs: []string{
				"--set", "controller.extraEnv.KGW_GLOBAL_POLICY_NAMESPACE=" + installNs,
				// Start with Envoy enabled - tests will change controller configs via helm upgrade
				"--set", "envoy.enabled=true",
				"--set", "agentgateway.enabled=false",
			},
		},
	)

	// Set the env to the install namespace if it is not already set
	if !nsEnvPredefined {
		os.Setenv(testutils.InstallNamespace, installNs)
	}

	// We register the cleanup function _before_ we actually perform the installation.
	// This allows us to uninstall kgateway, in case the original installation only completed partially
	testutils.Cleanup(t, func() {
		if !nsEnvPredefined {
			os.Unsetenv(testutils.InstallNamespace)
		}

		testInstallation.UninstallKgateway(ctx)
	})

	// Install kgateway with both controllers disabled
	testInstallation.InstallKgatewayFromLocalChart(ctx)

	// Run the dual controller test suite
	runner := e2e.NewSuiteRunner(false)
	runner.Register("DualController", dualcontroller.NewTestingSuite)
	runner.Run(ctx, t, testInstallation)
}
