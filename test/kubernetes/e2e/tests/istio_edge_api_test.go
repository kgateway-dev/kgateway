package tests_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/gloo/pkg/utils/envutils"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	. "github.com/solo-io/gloo/test/kubernetes/e2e/tests"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
	"github.com/solo-io/gloo/test/kubernetes/testutils/helper"
	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/skv2/codegen/util"
)

// TestIstioEdgeApiGateway is the function which executes a series of tests against a given installation where
// the k8s Gateway controller is disabled
func TestIstioEdgeApiGateway(t *testing.T) {
	ctx := context.Background()
	installNs, overrodeNs := envutils.LookupOrDefault(testutils.InstallNamespace, "istio-edge-api-gateway-test")
	testInstallation := e2e.CreateTestInstallation(
		t,
		&gloogateway.Context{
			InstallNamespace:   installNs,
			ValuesManifestFile: filepath.Join(util.MustGetThisDir(), "manifests", "istio-edge-gateway-test-helm.yaml"),
		},
	)

	testHelper := e2e.MustTestHelper(ctx, testInstallation)

	err := testInstallation.AddIstioctl(ctx)
	if err != nil {
		t.Errorf("failed to add istioctl: %v\n", err)
	}

	// Set the env to the install namespace if it is not already set
	if os.Getenv(testutils.InstallNamespace) == "" {
		os.Setenv(testutils.InstallNamespace, installNs)
	}

	// We register the cleanup function _before_ we actually perform the installation.
	// This allows us to uninstall Gloo Gateway, in case the original installation only completed partially
	t.Cleanup(func() {
		if overrodeNs {
			os.Unsetenv(testutils.InstallNamespace)
		}
		if t.Failed() {
			testInstallation.PreFailHandler(ctx)

			// Generate istioctl bug report
			testInstallation.CreateIstioBugReport(ctx)
		}

		testInstallation.UninstallGlooGateway(ctx, func(ctx context.Context) error {
			return testHelper.UninstallGlooAll()
		})

		// Uninstall Istio
		err = testInstallation.UninstallIstio()
		if err != nil {
			t.Errorf("failed to add istioctl: %v\n", err)
		}
	})

	// Install Istio before Gloo Gateway to make sure istiod is present before istio-proxy
	err = testInstallation.InstallMinimalIstio(ctx)
	if err != nil {
		t.Errorf("failed to add istioctl: %v\n", err)
	}

	// Install Gloo Gateway with only Edge APIs enabled
	testInstallation.InstallGlooGateway(ctx, func(ctx context.Context) error {
		return testHelper.InstallGloo(ctx, 5*time.Minute, helper.WithExtraArgs("--values", testInstallation.Metadata.ValuesManifestFile))
	})

	IstioEdgeApiSuiteRunner().Run(ctx, t, testInstallation)
}
