//go:build e2e

package tests_test

import (
	"context"
	"os"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/features/parallelcontrollers"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// TestParallelControllers tests the parallel controller architecture that can support running one or both controllers at the same time.
// This test verifies three scenarios:
// 1. Only kgateway chart installed (Envoy only)
// 2. Only agentgateway chart installed (Agentgateway only)
// 3. Both charts installed (both controllers running)
func TestParallelControllers(t *testing.T) {
	ctx := context.Background()
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "kgateway-test")

	// Create test installation without automatic chart installation
	// The suite will manage chart installations for each test scenario
	testInstallation := e2e.CreateTestInstallation(
		t,
		&install.Context{
			InstallNamespace:          installNs,
			ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
			ValuesManifestFile:        e2e.EmptyValuesManifestPath,
			ExtraHelmArgs: []string{
				"--set", "controller.extraEnv.KGW_GLOBAL_POLICY_NAMESPACE=" + installNs,
			},
		},
	)

	// Set the env to the install namespace if it is not already set
	if !nsEnvPredefined {
		os.Setenv(testutils.InstallNamespace, installNs)
	}

	// We register the cleanup function _before_ we actually perform the installation.
	testutils.Cleanup(t, func() {
		if !nsEnvPredefined {
			os.Unsetenv(testutils.InstallNamespace)
		}

		// Cleanup will be handled by the suite's cleanup methods
	})

	// Run the parallelcontrollers test suite
	// The suite will manage chart installations and cleanup
	runner := e2e.NewSuiteRunner(false)
	runner.Register("ParallelControllers", parallelcontrollers.NewTestingSuite)
	runner.Run(ctx, t, testInstallation)
}
