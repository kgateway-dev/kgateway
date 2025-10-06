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

func TestKgatewayWaypoint(t *testing.T) {
	ctx := context.Background()

	// Set Istio version if not already set
	if os.Getenv("ISTIO_VERSION") == "" {
		os.Setenv("ISTIO_VERSION", "1.25.1") // Using minimum required version that supports multiple TargetRef types for Istio Authz policies.
	}

	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "kgateway-waypoint-test")
	testInstallation := e2e.CreateTestInstallation(
		t,
		&install.Context{
			InstallNamespace:          installNs,
			ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
			ValuesManifestFile:        e2e.ManifestPath("waypoint-enabled-helm.yaml"),
		},
	)

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
		}

		testInstallation.UninstallKgateway(ctx)
		testInstallation.UninstallIstio()
	})

	// Download the latest Istio
	err := testInstallation.AddIstioctl(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Install the ambient profile to enable zTunnel
	err = testInstallation.InstallRevisionedIstio(
		ctx, "kgateway-waypoint-rev", "ambient",
		// required for ServiceEntry usage
		// enabled by default in 1.25; we test as far back as 1.23
		"--set", "values.cni.ambient.dnsCapture=true",
	)
	if err != nil {
		t.Fatal(err)
	}

	// Install kgateway
	testInstallation.InstallKgatewayFromLocalChart(ctx)

	WaypointSuiteRunner().Run(ctx, t, testInstallation)
}
