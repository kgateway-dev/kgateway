package edge_api_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/cue/cmd/cue/cmd"
	"github.com/solo-io/gloo/test/kube2e/helper"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/istio"

	"github.com/solo-io/skv2/codegen/util"
	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
)

// TestGlooctlIstioInjectEdgeApiGateway is the function which executes a series of tests against a given installation where
// the k8s Gateway controller is disabled
func TestGlooctlIstioInjectEdgeApiGateway(t *testing.T) {
	ctx := context.Background()
	testCluster := e2e.MustTestCluster()
	testInstallation := testCluster.RegisterTestInstallation(
		t,
		&gloogateway.Context{
			InstallNamespace:   "edge-api-test",
			ValuesManifestFile: filepath.Join(util.MustGetThisDir(), "manifests", "edge-api-gateway-test-helm.yaml"),
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

	// Install Gloo Gateway with only Edge APIs enabled
	testInstallation.InstallGlooGateway(ctx, func(ctx context.Context) error {
		return testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", testInstallation.Metadata.ValuesManifestFile))
	})
	// Inject istio with glooctl
	injectCmd, err := cmd.New([]string{testHelper.GlooctlExecName, "istio", "inject", "--install-namespace", testInstallation.Metadata.InstallNamespace})
	if err != nil {
		t.Fatalf("Failed to create inject command: %v", err)
	}
	if err := injectCmd.Execute(); err != nil {
		t.Fatalf("Failed to inject istio: %v", err)
	}

	t.Run("IstioIntegration", func(t *testing.T) {
		suite.Run(t, istio.NewTestingSuite(ctx, testInstallation))
	})
}
