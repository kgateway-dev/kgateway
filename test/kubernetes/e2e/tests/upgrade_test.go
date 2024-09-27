package tests_test

import (
	"context"
	"log"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	. "github.com/solo-io/gloo/test/kubernetes/e2e/tests"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
	"github.com/solo-io/gloo/test/kubernetes/testutils/helper"
)

// The upgrade tests delegate the installs, upgrades and deletions to each individual test within the suite

func TestUpgradeFromLastPatchPreviousMinor(t *testing.T) {
	ctx := context.Background()

	// Get the last released patch of the minor version prior to the one being tested.
	lastPatchPreviousMinorVersion, _, err := helper.GetUpgradeVersions(ctx, "gloo")
	require.NoError(t, err)

	testInstallation := e2e.CreateTestInstallation(
		t,
		&gloogateway.Context{
			InstallNamespace:          "upgrade-from-last-patch-previous-minor",
			ProfileValuesManifestFile: e2e.FullGatewayProfilePath,
			ValuesManifestFile:        e2e.EmptyValuesManifestPath,
			ValidationAlwaysAccept:    false,
			ReleasedVersion:           lastPatchPreviousMinorVersion.String(),
		},
	)

	UpgradeSuiteRunner().Run(ctx, t, testInstallation)
}

// This will be skipped if there has not yet been a patch release for the most current minor version.
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
			InstallNamespace:          "upgrade-from-current-patch-latest-minor",
			ProfileValuesManifestFile: e2e.FullGatewayProfilePath,
			ValuesManifestFile:        e2e.EmptyValuesManifestPath,
			ValidationAlwaysAccept:    false,
			ReleasedVersion:           currentPatchMostRecentMinorVersion.String(),
		},
	)

	UpgradeSuiteRunner().Run(ctx, t, testInstallation)
}
