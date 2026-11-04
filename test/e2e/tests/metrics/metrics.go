//go:build e2e

// Package metrics holds the body of the TestKgatewayMetrics e2e test.
package metrics

import (
	"context"
	"os"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	metricsfeature "github.com/kgateway-dev/kgateway/v2/test/e2e/features/metrics"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// Run executes the TestKgatewayMetrics scenario against the Installation
// produced by the supplied factory.
func Run(t *testing.T, factory e2e.InstallationFactory) {
	ctx := t.Context()
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "kgateway-test")
	testInstallation := factory(
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
	// This allows us to uninstall kgateway, in case the original installation only completed partially
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

	// Install kgateway
	testInstallation.InstallKgatewayFromLocalChart(ctx, t)

	// The suite references the shared nginx backend cross-namespace (with its own
	// ReferenceGrant) instead of deploying its own.
	common.SetupSharedNginxBackend(ctx, t, testInstallation.Underlying())

	// Metrics tests are run on their own in order to avoid metrics values being affected by other tests.
	SuiteRunner().Run(ctx, t, testInstallation.Underlying())
}

// SuiteRunner returns the suite runner for the TestKgatewayMetrics scenario.
func SuiteRunner() e2e.SuiteRunner {
	metricsSuiteRunner := e2e.NewSuiteRunner(false)

	metricsSuiteRunner.Register("Metrics", metricsfeature.NewTestingSuite)

	return metricsSuiteRunner
}
