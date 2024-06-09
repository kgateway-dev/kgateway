package tests_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/stateful_session"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
	"github.com/solo-io/skv2/codegen/util"
	"github.com/stretchr/testify/suite"
)

// TestGlooGatewayEdgeGateway is the function which executes a series of tests against a given installation where
// the k8s Gateway controller is disabled. A default gloo gateway is installed to allow the install checks to pass,
// but the expectation is that each test in this suite will define its own gateway, as the default gateway may have been deleted
func TestGlooNoDefaultGateway(t *testing.T) {
	ctx := context.Background()
	testInstallation := e2e.CreateTestInstallation(
		t,
		&gloogateway.Context{
			InstallNamespace:   "gloo-gateway-edge-test",
			ValuesManifestFile: filepath.Join(util.MustGetThisDir(), "manifests", "edge-gateway-test-helm.yaml"),
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

	// Install Gloo Gateway with only Gloo Edge Gateway APIs enabled
	testInstallation.InstallGlooGateway(ctx, func(ctx context.Context) error {
		return testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", testInstallation.Metadata.ValuesManifestFile))
	})

	t.Run("StatefulSession", func(t *testing.T) {
		suite.Run(t, stateful_session.NewTestingSuite(ctx, testInstallation))
	})
}
