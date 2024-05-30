package tests_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/gloo/pkg/utils/env"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/deployer"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/glooctl"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/headless_svc"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/port_routing"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/route_delegation"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/route_options"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/upstreams"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/virtualhost_options"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/solo-io/skv2/codegen/util"
	"github.com/stretchr/testify/suite"
)

// TestK8sGateway is the function which executes a series of tests against a given installation
func TestK8sGateway(t *testing.T) {
	ctx := context.Background()
	testInstallation := e2e.CreateTestInstallation(
		t,
		&gloogateway.Context{
			InstallNamespace:       env.GetOrDefault(testutils.InstallNamespace, "k8s-gw-test"),
			ValuesManifestFile:     filepath.Join(util.MustGetThisDir(), "manifests", "k8s-gateway-test-helm.yaml"),
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

	t.Run("Deployer", func(t *testing.T) {
		suite.Run(t, deployer.NewTestingSuite(ctx, testInstallation))
	})

	t.Run("RouteOptions", func(t *testing.T) {
		suite.Run(t, route_options.NewTestingSuite(ctx, testInstallation))
	})

	t.Run("VirtualHostOptions", func(t *testing.T) {
		suite.Run(t, virtualhost_options.NewTestingSuite(ctx, testInstallation))
	})

	t.Run("Upstreams", func(t *testing.T) {
		suite.Run(t, upstreams.NewTestingSuite(ctx, testInstallation))
	})

	t.Run("HeadlessSvc", func(t *testing.T) {
		suite.Run(t, headless_svc.NewK8sGatewayHeadlessSvcSuite(ctx, testInstallation))
	})

	t.Run("PortRouting", func(t *testing.T) {
		suite.Run(t, port_routing.NewTestingSuite(ctx, testInstallation))
	})

	t.Run("RouteDelegation", func(t *testing.T) {
		suite.Run(t, route_delegation.NewTestingSuite(ctx, testInstallation))
	})

	t.Run("Glooctl", func(t *testing.T) {
		t.Run("Check", func(t *testing.T) {
			suite.Run(t, glooctl.NewCheckSuite(ctx, testInstallation))
		})

		t.Run("Debug", func(t *testing.T) {
			suite.Run(t, glooctl.NewDebugSuite(ctx, testInstallation))
		})

		t.Run("GetProxy", func(t *testing.T) {
			suite.Run(t, glooctl.NewGetProxySuite(ctx, testInstallation))
		})
	})
}
