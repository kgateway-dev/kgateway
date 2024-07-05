package tests_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/skv2/codegen/util"

	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	. "github.com/solo-io/gloo/test/kubernetes/e2e/tests"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
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

	testHelper := e2e.MustTestHelper(ctx, testInstallation)

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
		return testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", testInstallation.Metadata.ValuesManifestFile))
	})

	UpgradeSuiteRunner().Run(ctx, t, testInstallation)
}

// TestUpgrade executes tests against an installation of Gloo Gateway, executes an upgrade,
// and finally executes tests against the upgraded version. This will be skipped if there
// has not yet been a patch release for the most current minor version.
func TestUpgradeFromCurrentPatchLatestMinor(t *testing.T) {
	// TODO: skip if no patches
}
