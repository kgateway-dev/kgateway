package tests_test

import (
	"context"
	"os"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	. "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// TestKgatewayIstioAutoMtls is the function which executes a series of tests against a given installation
func TestKgatewayIstioAutoMtls(t *testing.T) {
	ctx := context.Background()

	// Set Istio version if not already set
	if os.Getenv("ISTIO_VERSION") == "" {
		os.Setenv("ISTIO_VERSION", "1.25.1") // Using default istio version
	}

	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "automtls-istio-test")
	testInstallation := e2e.CreateTestInstallation(
		t,
		&install.Context{
			InstallNamespace:          installNs,
			ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
			ValuesManifestFile:        e2e.ManifestPath("istio-automtls-enabled-helm.yaml"),
		},
	)

	err := testInstallation.AddIstioctl(ctx)
	if err != nil {
		t.Errorf("failed to add istioctl: %v\n", err)
	}

	// Set the env to the install namespace if it is not already set
	if !nsEnvPredefined {
		os.Setenv(testutils.InstallNamespace, installNs)
	}

	// We register the cleanup function _before_ we actually perform the installation.
	// This allows us to uninstall kgateway, in case the original installation only completed partially
	t.Cleanup(func() {
		if !nsEnvPredefined {
			os.Unsetenv(testutils.InstallNamespace)
		}
		if t.Failed() {
			testInstallation.PreFailHandler(ctx)

			// Generate istioctl bug report
			testInstallation.CreateIstioBugReport(ctx)
		}

		// Uninstall kgateway
		testInstallation.UninstallKgateway(ctx)

		// Uninstall Istio
		err = testInstallation.UninstallIstio()
		if err != nil {
			t.Errorf("failed to add istioctl: %v\n", err)
		}
	})

	// Install Istio before kgateway to make sure istiod is present before kgateway for sds
	err = testInstallation.InstallMinimalIstio(ctx)
	if err != nil {
		t.Errorf("failed to add istioctl: %v\n", err)
	}

	// Install kgateway
	testInstallation.InstallKgatewayFromLocalChart(ctx)

	AutomtlsIstioSuiteRunner().Run(ctx, t, testInstallation)
}
