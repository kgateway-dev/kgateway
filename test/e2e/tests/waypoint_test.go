//go:build e2e

package tests_test

import (
	"context"
	"os"
	"testing"

	"github.com/Masterminds/semver/v3"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	. "github.com/kgateway-dev/kgateway/v2/test/e2e/tests"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	testruntime "github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/runtime"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var (
	// the min istio version required to run the waypoint tests
	minIstioVersion = semver.MustParse("1.25.1")
)

func TestKgatewayWaypoint(t *testing.T) {
	ctx := context.Background()

	// Set Istio version if not already set
	if os.Getenv(testruntime.IstioVersionEnv) == "" {
		os.Setenv(testruntime.IstioVersionEnv, "1.25.1") // Using minimum required version that supports multiple TargetRef types for Istio Authz policies.
	}

	if shouldSkip(t) {
		t.Skip("Skipping waypoint tests due to istio version requirements")
		return
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
	testutils.Cleanup(t, func() {
		if !nsEnvPredefined {
			os.Unsetenv(testutils.InstallNamespace)
		}
		if t.Failed() {
			testInstallation.PreFailHandler(ctx, t)
		}

		testInstallation.UninstallKgateway(ctx, t)
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
	testInstallation.InstallKgatewayFromLocalChart(ctx, t)

	WaypointSuiteRunner().Run(ctx, t, testInstallation)
}

func shouldSkip(t *testing.T) bool {
	istioVersion, ok := os.LookupEnv(testruntime.IstioVersionEnv)
	if !ok {
		t.Fatalf("required environment variable %s not set", testruntime.IstioVersionEnv)
	}

	istioVersionSemver, err := semver.NewVersion(istioVersion)
	if err != nil {
		t.Fatalf("failed to parse istio version %s as semver: %v", istioVersion, err)
	}

	if istioVersionSemver.LessThan(minIstioVersion) {
		return true
	}
	return false
}
