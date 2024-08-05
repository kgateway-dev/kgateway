package tests_test

import (
	"context"
	"log"
	"path/filepath"
	"testing"

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

	// Get the last released patch of the minor version prior to the one being tested.
	lastPatchPreviousMinorVersion, _, err := helper.GetUpgradeVersions(ctx, "gloo")
	require.NoError(t, err)

	testInstallation := e2e.CreateTestInstallation(
		t,
		&gloogateway.Context{
			InstallNamespace:       "upgrade-from-last-patch-previous-minor",
			ValuesManifestFile:     filepath.Join(util.MustGetThisDir(), "manifests", "upgrade-base-test-helm.yaml"),
			ValidationAlwaysAccept: false,
			ReleasedVersion:        lastPatchPreviousMinorVersion.String(),
		},
	)

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
			ReleasedVersion:        currentPatchMostRecentMinorVersion.String(),
		},
	)

	UpgradeSuiteRunner().Run(ctx, t, testInstallation)
}
