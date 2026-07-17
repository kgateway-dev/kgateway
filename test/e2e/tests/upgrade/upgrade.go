//go:build e2e

// Package upgrade holds the body of the TestUpgrade e2e test.
package upgrade

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	upgradefeature "github.com/kgateway-dev/kgateway/v2/test/e2e/features/upgrade"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// Run executes the TestUpgrade scenario against Installations produced by the
// supplied factory: it upgrades from the latest release and from the previous
// minor release to the locally-built chart.
func Run(t *testing.T, factory e2e.InstallationFactory) {
	latestTag, err := upgradefeature.FetchLatestRelease(t.Context())
	if err != nil {
		t.Fatalf("failed to get latest patch version: %v", err)
	}
	t.Run(fmt.Sprintf("TestUpgradeFromLatestRelease [%s]", latestTag), func(t *testing.T) {
		RunFromVersion(t, factory, latestTag)
	})

	previousMinor, err1 := upgradefeature.FetchPreviousMinorRelease(t.Context())
	if err1 != nil {
		t.Fatalf("failed to get previous minor release: %v", err1)
	}
	t.Run(fmt.Sprintf("TestUpgradeFromPreviousMinor[%s]", previousMinor), func(t *testing.T) {
		RunFromVersion(t, factory, previousMinor)
	})
}

// RunFromVersion installs the given released version and runs the upgrade
// suite against it.
func RunFromVersion(t *testing.T, factory e2e.InstallationFactory, fromVersion string) {
	ctx := t.Context()
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "kgateway-upgrade")
	testInstallation := factory(
		t,
		&install.Context{
			InstallNamespace:          installNs,
			ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
			ValuesManifestFile:        e2e.EmptyValuesManifestPath,
			ExtraHelmArgs:             []string{"--wait", "--wait-for-jobs"},
		},
	)

	if !nsEnvPredefined {
		os.Setenv(testutils.InstallNamespace, installNs)
	}

	// Register cleanup before installation so partial installs are also cleaned up.
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

	// Install the released version from the remote OCI registry.
	testInstallation.InstallKgatewayFromRelease(ctx, t, fromVersion)

	SuiteRunner(fromVersion).Run(ctx, t, testInstallation.Underlying())
}

// SuiteRunner returns the suite runner for the TestUpgrade scenario,
// parameterized by the version being upgraded from.
func SuiteRunner(fromVersion string) e2e.SuiteRunner {
	upgradeSuiteRunner := e2e.NewSuiteRunner(false)
	upgradeSuiteRunner.Register("Upgrade", upgradefeature.NewTestingSuite(fromVersion))
	return upgradeSuiteRunner
}
