package gloo_gateway_edge_test

import (
	"context"
	"fmt"
	"os"
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

// TestGlooGatewayEdgeGateway is the function which executes a series of tests against a given installation where
// the k8s Gateway controller is disabled
func TestGlooGatewayEdgeGateway(t *testing.T) {
	ctx := context.Background()
	testCluster := e2e.MustTestCluster()
	testInstallation := testCluster.RegisterTestInstallation(
		t,
		&gloogateway.Context{
			InstallNamespace:   "gloo-gateway-edge-test",
			ValuesManifestFile: filepath.Join(util.MustGetThisDir(), "manifests", "gloo-gateway-test-helm.yaml"),
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

	// Install Gloo Gateway with only Gloo Gateway APIs enabled
	testInstallation.InstallGlooGateway(ctx, func(ctx context.Context) error {
		return testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", testInstallation.Metadata.ValuesManifestFile))
	})

	t.Run("HeadlessSvc", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", fmt.Sprintf("headless-svc-%s", testInstallation.Metadata.InstallNamespace))
		if err != nil {
			t.Fatalf("Failed to create temporary directory: %v", err)
		}
		defer func() {
			// Delete the temporary directory after the test completes
			if err := os.RemoveAll(tempDir); err != nil {
				t.Errorf("Failed to remove temporary directory: %v", err)
			}
		}()
		suite.Run(t, headless_svc.NewGlooGatewayHeadlessSvcSuite(ctx, testInstallation, tempDir))
	})
}
