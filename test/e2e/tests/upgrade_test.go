//go:build e2e

package tests_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/features/upgrade"
	. "github.com/kgateway-dev/kgateway/v2/test/e2e/tests"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

func TestUpgrade(t *testing.T) {
	latestTag, err := upgrade.FetchLatestRelease(t.Context())
	if err != nil {
		t.Fatalf("failed to get latest patch version: %v", err)
	}
	t.Run(fmt.Sprintf("TestUpgradeFromLatestRelease [%s]", latestTag), func(t *testing.T) {
		testUpgrade(t, latestTag)
	})

	previousMinor, err1 := upgrade.FetchPreviousMinorRelease(t.Context())
	if err1 != nil {
		t.Fatalf("failed to get previous minor release: %v", err1)
	}
	t.Run(fmt.Sprintf("TestUpgradeFromPreviousMinor[%s]", previousMinor), func(t *testing.T) {
		testUpgrade(t, previousMinor)
	})
}

func testUpgrade(t *testing.T, fromVersion string) {
	ctx := context.Background()
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "kgateway-upgrade")
	testInstallation := e2e.CreateTestInstallation(
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
		if !nsEnvPredefined {
			os.Unsetenv(testutils.InstallNamespace)
		}
		if t.Failed() {
			testInstallation.PreFailHandler(ctx)
		}
		testInstallation.UninstallKgateway(ctx)
	})

	// Install the released version from the remote OCI registry.
	testInstallation.InstallKgatewayFromRelease(ctx, t, fromVersion)

	UpgradeSuiteRunner().Run(ctx, t, testInstallation)
}
