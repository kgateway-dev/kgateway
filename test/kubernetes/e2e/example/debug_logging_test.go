package example

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/gloo/test/kube2e/helper"

	"github.com/solo-io/skv2/codegen/util"
	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/example"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
)

// TestInstallationWithDebugLogLevel is the function which executes a series of tests against a given installation
func TestInstallationWithDebugLogLevel(t *testing.T) {
	ctx := context.Background()
	testCluster := e2e.MustTestCluster()
	testInstallation := testCluster.RegisterTestInstallation(
		t,
		&gloogateway.Context{
			SkipGlooInstall:    e2e.SkipGlooInstall,
			InstallNamespace:   "debug-example",
			ValuesManifestFile: filepath.Join(util.MustGetThisDir(), "manifests", "debug-example.yaml"),
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
		testCluster.UnregisterTestInstallation(testInstallation)
	})

	testInstallation.InstallGlooGateway(ctx, func(ctx context.Context) error {
		return testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", testInstallation.Metadata.ValuesManifestFile))
	})

	t.Run("Example", func(t *testing.T) {
		suite.Run(t, example.NewTestingSuite(ctx, testInstallation))
	})
}
