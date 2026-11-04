//go:build e2e

// Package automtlsistio holds the body of the TestKgatewayIstioAutoMtls e2e
// test.
package automtlsistio

import (
	"context"
	"os"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/features/istio"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	testruntime "github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/runtime"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

const (
	minAutomtlsIstioVersion = "1.24.2"
)

// Run executes the TestKgatewayIstioAutoMtls scenario against the
// Installation produced by the supplied factory.
func Run(t *testing.T, factory e2e.InstallationFactory) {
	ctx := t.Context()

	// Set Istio version if not already set
	if os.Getenv(testruntime.IstioVersionEnv) == "" {
		os.Setenv(testruntime.IstioVersionEnv, testruntime.DefaultIstioVersion) // Using default istio version
	}

	if testruntime.ShouldSkipIstioVersion(t, minAutomtlsIstioVersion) {
		t.Skip("Skipping due to https://github.com/istio/istio/issues/53846 which is fixed in Istio >= 1.24.2")
		return
	}

	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "automtls-istio-test")
	testInstallation := factory(
		t,
		&install.Context{
			InstallNamespace:          installNs,
			ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
			ValuesManifestFile:        e2e.ManifestPath("istio-automtls-enabled-helm.yaml"),
		},
	)

	err := testInstallation.Underlying().AddIstioctl(ctx)
	if err != nil {
		t.Fatalf("failed to add istioctl: %v", err)
	}

	// Set the env to the install namespace if it is not already set
	if !nsEnvPredefined {
		os.Setenv(testutils.InstallNamespace, installNs)
	}

	// We register the cleanup function _before_ we actually perform the installation.
	// This allows us to uninstall kgateway, in case the original installation only completed partially
	testutils.Cleanup(t, func() {
		ctx := context.Background() // t.Context() is canceled before t's cleanup runs
		if !nsEnvPredefined {
			os.Unsetenv(testutils.InstallNamespace)
		}
		if t.Failed() {
			testInstallation.PreFailHandler(ctx, t)

			// Generate istioctl bug report
			testInstallation.Underlying().CreateIstioBugReport(ctx)
		}

		// Uninstall kgateway
		testInstallation.UninstallKgateway(ctx, t)

		// Uninstall Istio
		if err := testInstallation.Underlying().UninstallIstio(); err != nil {
			t.Errorf("failed to uninstall istio: %v\n", err)
		}
	})

	// Install Istio before kgateway to make sure istiod is present before kgateway for sds
	err = testInstallation.Underlying().InstallMinimalIstio(ctx)
	if err != nil {
		t.Fatalf("failed to install istio: %v", err)
	}

	// Install kgateway
	testInstallation.InstallKgatewayFromLocalChart(ctx, t)

	SuiteRunner().Run(ctx, t, testInstallation.Underlying())
}

// SuiteRunner returns the suite runner for the TestKgatewayIstioAutoMtls
// scenario.
func SuiteRunner() e2e.SuiteRunner {
	automtlsIstioSuiteRunner := e2e.NewSuiteRunner(false)

	automtlsIstioSuiteRunner.Register("IstioIntegrationAutoMtls", istio.NewIstioAutoMtlsSuite)
	automtlsIstioSuiteRunner.Register("IstioIntegrationAutoMtlsDisabled", istio.NewIstioCustomMtlsSuite)

	return automtlsIstioSuiteRunner
}
