//go:build e2e

// Package bootstrapoverlays holds the body of the TestBootstrapOverlays e2e test.
package bootstrapoverlays

import (
	"context"
	"os"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	bootstrapoverlaysfeature "github.com/kgateway-dev/kgateway/v2/test/e2e/features/bootstrapoverlays"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// Run executes the TestBootstrapOverlays scenario against the Installation produced by
// the supplied factory. The scenario has its own installation because it upgrades the
// controller to change KGW_BOOTSTRAP_OVERLAY_MANAGED_PATHS.
func Run(t *testing.T, factory e2e.InstallationFactory) {
	// Describe the installation.
	ctx := t.Context()
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "bootstrap-overlays-test")
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

// SuiteRunner returns the suite runner for the TestBootstrapOverlays scenario.
func SuiteRunner() e2e.SuiteRunner {
	runner := e2e.NewSuiteRunner(false)
	runner.Register("BootstrapOverlays", bootstrapoverlaysfeature.NewTestingSuite)
	return runner
}
