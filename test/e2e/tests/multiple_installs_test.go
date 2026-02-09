//go:build e2e

package tests_test

import (
	"context"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/features/multiinstall"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

func TestMultipleInstalls(t *testing.T) {
	ctx := t.Context()

	installs := []struct {
		namespace        string
		testInstallation *e2e.TestInstallation
	}{
		{
			namespace: "kgw-test-1",
			testInstallation: e2e.CreateTestInstallation(
				t,
				&install.Context{
					InstallNamespace:          "kgw-test-1",
					ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
					ValuesManifestFile:        e2e.ManifestPath("multiple_installs_values1.yaml"),
				},
			),
		},
		{
			namespace: "kgw-test-2",
			testInstallation: e2e.CreateTestInstallation(
				t,
				&install.Context{
					InstallNamespace:          "kgw-test-2",
					ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
					ValuesManifestFile:        e2e.ManifestPath("multiple_installs_values2.yaml"),
				},
			),
		},
	}

	// We register the cleanup function _before_ we actually perform the installation.
	// This allows us to uninstall kgateway, in case the original installation only completed partially
	testutils.Cleanup(t, func() {
		ctx := context.Background()
		for _, install := range installs {
			if t.Failed() {
				install.testInstallation.PreFailHandler(ctx, t)
			}
			install.testInstallation.UninstallKgatewayCore(ctx, t)
			cleanupPerInstall(ctx, install.testInstallation, t)
		}
		installs[0].testInstallation.UninstallKgatewayCRDs(ctx, t)
	})

	// Install all kgateway instances first
	for i, install := range installs {
		if i == 0 {
			install.testInstallation.InstallKgatewayCRDsFromLocalChart(ctx, t)
		}
		// Install kgateway
		install.testInstallation.InstallKgatewayCoreFromLocalChart(ctx, t)
		applyPerInstall(ctx, install.testInstallation, t)
	}

	// Test each kgateway instance
	for _, install := range installs {
		runner := multipleInstallsSuiteRunner(install.namespace)
		runner.Run(ctx, t, install.testInstallation)
	}
}

func multipleInstallsSuiteRunner(namespace string) e2e.SuiteRunner {
	runner := e2e.NewSuiteRunner(false)
	runner.Register("Basic/"+namespace, multiinstall.NewTestingSuite)

	return runner
}

func applyPerInstall(ctx context.Context, ti *e2e.TestInstallation, t *testing.T) {
	namespace := ti.Metadata.InstallNamespace

	err := ti.Actions.Kubectl().ApplyFile(ctx, multiinstall.BasicManifest, "-n", namespace)
	ti.AssertionsT(t).Require.NoError(err)
	for _, obj := range getPerInstallObjects(namespace) {
		ti.AssertionsT(t).EventuallyObjectsExist(ctx, obj)
	}

	err = ti.Actions.Kubectl().ApplyFile(ctx, defaults.CurlPodManifest)
	ti.AssertionsT(t).Require.NoError(err)
	ti.AssertionsT(t).EventuallyObjectsExist(ctx, defaults.CurlPod)
}

func cleanupPerInstall(ctx context.Context, ti *e2e.TestInstallation, t *testing.T) {
	namespace := ti.Metadata.InstallNamespace

	err := ti.Actions.Kubectl().DeleteFileSafe(ctx, multiinstall.BasicManifest, "-n", namespace)
	ti.AssertionsT(t).Require.NoError(err)
	for _, obj := range getPerInstallObjects(namespace) {
		ti.AssertionsT(t).EventuallyObjectsNotExist(ctx, obj)
	}

	err = ti.Actions.Kubectl().DeleteFileSafe(ctx, defaults.CurlPodManifest)
	ti.AssertionsT(t).Require.NoError(err)
	ti.AssertionsT(t).EventuallyObjectsNotExist(ctx, defaults.CurlPod)
}

func getPerInstallObjects(ns string) []client.Object {
	return []client.Object{
		multiinstall.Gateway(ns), multiinstall.HttpbinRoute(ns), multiinstall.HttpbinDeployment(ns),
	}
}
