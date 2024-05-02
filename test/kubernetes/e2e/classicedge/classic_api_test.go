package classicedge_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"

	"github.com/solo-io/skv2/codegen/util"
	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
)

// TestClassicEdgeGateway is the function which executes a series of tests against a given installation where
// the k8s Gateway controller is disabled
func TestClassicEdgeGateway(t *testing.T) {
	ctx := context.Background()
	testCluster := e2e.MustTestCluster()
	testInstallation := testCluster.RegisterTestInstallation(
		t,
		&gloogateway.Context{
			InstallNamespace:   "classic-edge-test",
			ValuesManifestFile: filepath.Join(util.MustGetThisDir(), "manifests", "classic-gateway-test-helm.yaml"),
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

	// Install Gloo Gateway with only classic APIs enabled
	testInstallation.InstallGlooGateway(ctx, func(ctx context.Context) error {
		return testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", testInstallation.Metadata.ValuesManifestFile))
	})

	t.Run("HeadlessSvc", func(t *testing.T) {
		suite.Run(t, headless_svc.NewClassicHeadlessSvcSuite(ctx, testInstallation))
	})
}
