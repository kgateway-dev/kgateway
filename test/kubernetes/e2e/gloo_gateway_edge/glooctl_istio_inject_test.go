package gloo_gateway_edge_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/glooctl"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/istio"
	"github.com/solo-io/skv2/codegen/util"
	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
)

// TestGlooctlIstioInjectEdgeApiGateway is the function which executes a series of tests against a given installation where
// the k8s Gateway controller is disabled and glooctl istio inject is used to inject istio into the installation
func TestGlooctlIstioInjectEdgeApiGateway(t *testing.T) {
	ctx := context.Background()
	testCluster := e2e.MustTestCluster()
	testInstallation := testCluster.RegisterTestInstallation(
		t,
		&gloogateway.Context{
			InstallNamespace:   "glooctl-edge-api-test",
			ValuesManifestFile: filepath.Join(util.MustGetThisDir(), "manifests", "edge-api-gateway-test-helm.yaml"),
		},
	)

	testHelper := e2e.MustTestHelper(ctx, testInstallation)
	err := testInstallation.AddIstioctl(ctx)
	if err != nil {
		t.Fatalf("failed to get istioctl: %v", err)
	}
	glooctlPath := filepath.Join(testHelper.RootDir, testHelper.BuildAssetDir, testHelper.GlooctlExecName)

	// We register the cleanup function _before_ we actually perform the installation.
	// This allows us to uninstall Gloo Gateway, in case the original installation only completed partially
	t.Cleanup(func() {
		if t.Failed() {
			testInstallation.PreFailHandler(ctx)
		}

		testInstallation.UninstallGlooGateway(ctx, func(ctx context.Context) error {
			return testHelper.UninstallGlooAll()
		})

		// Uninstall Istio
		err = testInstallation.UninstallIstio()
		if err != nil {
			t.Fatalf("failed to uninstall istio: %v", err)
		}

		testCluster.UnregisterTestInstallation(testInstallation)
	})

	// Install Istio before Gloo Gateway to make sure istiod is present before istio-proxy
	err = testInstallation.InstallMinimalIstio(ctx)
	if err != nil {
		t.Fatalf("failed to install istio: %v", err)
	}

	// Install Gloo Gateway with only Edge APIs enabled
	testInstallation.InstallGlooGateway(ctx, func(ctx context.Context) error {
		return testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", testInstallation.Metadata.ValuesManifestFile))
	})

	// NOTE: Order of tests is important here because the tests are dependent on each other (e.g. the inject test must run before the istio test)
	t.Run("GlooctlIstioInject", func(t *testing.T) {
		suite.Run(t, glooctl.NewIstioInjectTestingSuite(ctx, testInstallation, glooctlPath))
	})

	t.Run("IstioIntegration", func(t *testing.T) {
		// create a tmp output directory
		tempDir, err := os.MkdirTemp("", fmt.Sprintf("istio-glooctl-inject-%s", testInstallation.Metadata.InstallNamespace))
		if err != nil {
			t.Fatalf("Failed to create temporary directory: %v", err)
		}
		defer func() {
			// Delete the temporary directory after the test completes
			if err := os.RemoveAll(tempDir); err != nil {
				t.Errorf("Failed to remove temporary directory: %v", err)
			}
		}()
		suite.Run(t, istio.NewGlooTestingSuite(ctx, testInstallation, tempDir))
	})

	t.Run("GlooctlIstioUninject", func(t *testing.T) {
		suite.Run(t, glooctl.NewIstioUninjectTestingSuite(ctx, testInstallation, glooctlPath))
	})
}
