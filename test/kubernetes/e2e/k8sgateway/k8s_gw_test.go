package k8sgateway_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/gloo/test/kube2e"
	"github.com/solo-io/gloo/test/kube2e/helper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/skv2/codegen/util"
	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/deployer"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/route_options"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
)

// TestK8sGateway is the function which executes a series of tests against a given installation
func TestK8sGateway(t *testing.T) {
	RegisterFailHandler(Fail)

	ctx := context.Background()
	testCluster := e2e.MustTestCluster()
	testInstallation := testCluster.RegisterTestInstallation(
		t,
		&gloogateway.Context{
			InstallNamespace:   "k8s-gw-test",
			ValuesManifestFile: filepath.Join(util.MustGetThisDir(), "manifests", "k8s-gateway-test-helm.yaml"),
		},
	)

	testHelper, err := kube2e.GetTestHelper(ctx, testInstallation.Metadata.InstallNamespace)
	testInstallation.Assertions.Require.NoError(err, "Could not construct TestHelper for test")

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

	t.Run("InstallGateway", func(t *testing.T) {
		testInstallation.InstallGlooGateway(ctx, func(ctx context.Context) error {
			return testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", testInstallation.Metadata.ValuesManifestFile))
		})
	})

	t.Run("Deployer", func(t *testing.T) {
		suite.Run(t, deployer.NewTestingSuite(ctx, testInstallation))
	})

	t.Run("RouteOptions", func(t *testing.T) {
		suite.Run(t, route_options.NewTestingSuite(ctx, testInstallation))
	})
}
