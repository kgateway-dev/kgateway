//go:build e2e

// Package bootstrappatches holds the body of the TestBootstrapPatches e2e test.
package bootstrappatches

import (
	"context"
	"os"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	bootstrappatchesfeature "github.com/kgateway-dev/kgateway/v2/test/e2e/features/bootstrappatches"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// Run executes the TestBootstrapPatches scenario against the Installation produced by
// the supplied factory. The scenario has its own installation because it upgrades the
// controller to change KGW_BOOTSTRAP_PATCH_MANAGED_PATHS.
func Run(t *testing.T, factory e2e.InstallationFactory) {
	// Describe the installation.
	ctx := t.Context()
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "bootstrap-patches-test")
	testInstallation := factory(
		t,
		&install.Context{
			InstallNamespace:          installNs,
			ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
			ValuesManifestFile:        e2e.EmptyValuesManifestPath,
		},
	)

	// Set the env to the install namespace if it is not already set.
	if !nsEnvPredefined {
		os.Setenv(testutils.InstallNamespace, installNs)
	}

	// Register the uninstall before installing, so that a partial installation is
	// removed too.
	testutils.Cleanup(t, func() {
		ctx := context.Background() // t.Context() is canceled before t's cleanup runs
		if !nsEnvPredefined {
			os.Unsetenv(testutils.InstallNamespace)
		}
		if t.Failed() {
			testInstallation.PreFailHandler(ctx, t)
		}

		testInstallation.UninstallKgateway(ctx, t)
	})

	// Install kgateway and run the suite.
	testInstallation.InstallKgatewayFromLocalChart(ctx, t)
	SuiteRunner().Run(ctx, t, testInstallation.Underlying())
}

// SuiteRunner returns the suite runner for the TestBootstrapPatches scenario.
func SuiteRunner() e2e.SuiteRunner {
	runner := e2e.NewSuiteRunner(false)
	runner.Register("BootstrapPatches", bootstrappatchesfeature.NewTestingSuite)
	return runner
}
