package tests_test

import (
	"context"
	"log"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/skv2/codegen/util"
	"github.com/stretchr/testify/require"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	. "github.com/solo-io/gloo/test/kubernetes/e2e/tests"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
	"github.com/solo-io/gloo/test/kubernetes/testutils/helper"
)

// The previous kube2e upgrade tests ran no meaningful assertions against the installation
// before upgrading, so we do not here either.

// TestUpgradeFromLastPatchPreviousMinor executes tests against an installation of Gloo Gateway, executes an upgrade ,
// and finally executes tests against the upgraded version.
func TestUpgradeFromLastPatchPreviousMinor(t *testing.T) {
	ctx := context.Background()
	testInstallation := e2e.CreateTestInstallation(
		t,
		&gloogateway.Context{
			InstallNamespace:       "upgrade-from-last-patch-previous-minor",
			ValuesManifestFile:     filepath.Join(util.MustGetThisDir(), "manifests", "upgrade-base-test-helm.yaml"),
			ValidationAlwaysAccept: false,
		},
	)

	// Get the last released patch of the minor version prior to the one being tested.
	lastPatchPreviousMinorVersion, _, err := helper.GetUpgradeVersions(ctx, "gloo")
	testInstallation.Assertions.Require.NoError(err)

	testHelper := e2e.MustTestHelper(ctx, testInstallation)

	testHelper.ReleasedVersion = lastPatchPreviousMinorVersion.String()

	// We register the cleanup function _before_ we actually perform the installation.
	// This allows us to uninstall Gloo Gateway, in case the original installation only completed partially
	t.Cleanup(func() {
		if t.Failed() {
			testInstallation.PreFailHandler(ctx)
		}

		testInstallation.UninstallGlooGateway(ctx, func(ctx context.Context) error {
			return testHelper.UninstallGlooAll()
		})
	})

	// Install Gloo Gateway
	testInstallation.InstallGlooGateway(ctx, func(ctx context.Context) error {
		return testHelper.InstallGloo(ctx, 5*time.Minute, helper.WithExtraArgs("--values", testInstallation.Metadata.ValuesManifestFile))
	})

	// If specific upgrade cases need to be tested, values overrides should be defined in manifests/ and passed into the upgrade fn here.
	testInstallation.UpgradeGlooGateway(ctx, testHelper.ChartVersion(), func(ctx context.Context) (func() error, error) {
		return testHelper.UpgradeGloo(ctx, 5*time.Minute, testHelper.ChartVersion(), true, filepath.Join(testHelper.RootDir, "install", "helm", "gloo", "crds"))
	})

	UpgradeSuiteRunner().Run(ctx, t, testInstallation)
}

// TestUpgrade executes tests against an installation of Gloo Gateway, executes an upgrade,
// and finally executes tests against the upgraded version. This will be skipped if there
// has not yet been a patch release for the most current minor version.
func TestUpgradeFromCurrentPatchLatestMinor(t *testing.T) {
	ctx := context.Background()

	// Get the last released patch of the minor version being tested.
	_, currentPatchMostRecentMinorVersion, err := helper.GetUpgradeVersions(ctx, "gloo")
	require.NoError(t, err)
	if currentPatchMostRecentMinorVersion == nil {
		logMsg := "This test case is not valid because there are no released patch versions of the minor we are currently branched from."
		log.Println(logMsg)
		return
	}

	testInstallation := e2e.CreateTestInstallation(
		t,
		&gloogateway.Context{
			InstallNamespace:       "upgrade-from-current-patch-latest-minor",
			ValuesManifestFile:     filepath.Join(util.MustGetThisDir(), "manifests", "upgrade-base-test-helm.yaml"),
			ValidationAlwaysAccept: false,
		},
	)

	testHelper := e2e.MustTestHelper(ctx, testInstallation)

	testHelper.ReleasedVersion = currentPatchMostRecentMinorVersion.String()
}
