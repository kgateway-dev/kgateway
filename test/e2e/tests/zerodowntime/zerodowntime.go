//go:build e2e

// Package zerodowntime holds the body of the TestZeroDowntimeRollout e2e
// test.
package zerodowntime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/features/zero_downtime_rollout"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// Run executes the TestZeroDowntimeRollout scenario against the Installation
// produced by the supplied factory.
func Run(t *testing.T, factory e2e.InstallationFactory) {
	ctx := t.Context()
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "zero-downtime")
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

	// We register the cleanup function _before_ we actually perform the installation.
	// This allows us to uninstall, in case the original installation only completed partially.
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

	testInstallation.InstallKgatewayFromLocalChart(ctx, t)

	common.SetupBaseConfig(ctx, t, testInstallation.Underlying(),
		filepath.Join(fsutils.MustGetThisDir(), "../../features/zero_downtime_rollout/testdata", "gateway.yaml"),
	)
	common.SetupBaseGateway(ctx, t, testInstallation.Underlying(), types.NamespacedName{
		Namespace: "default",
		Name:      "gw",
	})

	SuiteRunner().Run(ctx, t, testInstallation.Underlying())
}

// SuiteRunner returns the suite runner for the TestZeroDowntimeRollout
// scenario.
func SuiteRunner() e2e.SuiteRunner {
	zeroDowntimeSuiteRunner := e2e.NewSuiteRunner(false)
	zeroDowntimeSuiteRunner.Register("ZeroDowntimeRollout", zero_downtime_rollout.NewTestingSuiteKgateway)
	return zeroDowntimeSuiteRunner
}
