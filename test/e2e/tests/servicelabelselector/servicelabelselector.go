//go:build e2e

// Package servicelabelselector holds the body of the TestServiceLabelSelector e2e test.
package servicelabelselector

import (
	"context"
	"os"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	servicelabelselectorfeature "github.com/kgateway-dev/kgateway/v2/test/e2e/features/servicelabelselector"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// Run executes the TestServiceLabelSelector scenario against the Installation produced by the
// supplied factory. It needs an installation of its own because the Service label selector
// applies to every Gateway the controller manages.
func Run(t *testing.T, factory e2e.InstallationFactory) {
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "kgateway-service-label-selector-test")
	testInstallation := factory(
		t,
		&install.Context{
			InstallNamespace:          installNs,
			ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
			ValuesManifestFile:        e2e.ManifestPath("service-label-selector-helm.yaml"),
			ExtraHelmArgs: []string{
				"--set", "controller.extraEnv.KGW_GLOBAL_POLICY_NAMESPACE=" + installNs,
			},
		},
	)

	// Set the env to the install namespace if it is not already set
	if !nsEnvPredefined {
		os.Setenv(testutils.InstallNamespace, installNs)
	}

	// Register cleanup before installing so a partial installation is still uninstalled.
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

	testInstallation.InstallKgatewayFromLocalChart(t.Context(), t)

	SuiteRunner().Run(t.Context(), t, testInstallation.Underlying())
}

// SuiteRunner returns the suite runner for the TestServiceLabelSelector scenario.
func SuiteRunner() e2e.SuiteRunner {
	runner := e2e.NewSuiteRunner(false)
	runner.Register("ServiceLabelSelector", servicelabelselectorfeature.NewTestingSuite)
	return runner
}
