package k8sgateway_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/skv2/codegen/util"
	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/port_routing"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/route_options"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/virtualhost_options"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
)

// TestK8sGatewayNoValidation executes tests against a K8s Gateway gloo install with validation disabled
func TestK8sGatewayNoValidation(t *testing.T) {
	ctx := context.Background()
	testCluster := e2e.MustTestCluster()
	testInstallation := testCluster.RegisterTestInstallation(
		t,
		&gloogateway.Context{
			InstallNamespace:   "k8s-gw-test",
			ValuesManifestFile: filepath.Join(util.MustGetThisDir(), "manifests", "k8s-gateway-no-webhook-validation-test-helm.yaml"),
		},
	)

	testHelper := e2e.MustTestHelper(ctx, testInstallation)

	// create a tmp output directory for generated resources
	tempOutputDir, err := os.MkdirTemp("", testInstallation.Metadata.InstallNamespace)
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer func() {
		// Delete the temporary directory after the test completes
		if err := os.RemoveAll(tempOutputDir); err != nil {
			t.Errorf("Failed to remove temporary directory: %v", err)
		}
	}()

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

	// Install Gloo Gateway
	testInstallation.InstallGlooGateway(ctx, func(ctx context.Context) error {
		return testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", testInstallation.Metadata.ValuesManifestFile))
	})

	t.Run("RouteOptions", func(t *testing.T) {
		suite.Run(t, route_options.NewTestingSuite(ctx, testInstallation, false))
	})

	t.Run("VirtualHostOptions", func(t *testing.T) {
		suite.Run(t, virtualhost_options.NewTestingSuite(ctx, testInstallation, false))
	})

	t.Run("PortRouting", func(t *testing.T) {
		suite.Run(t, port_routing.NewTestingSuite(ctx, testInstallation))

	})
}
