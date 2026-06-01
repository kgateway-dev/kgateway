//go:build e2e

// Package multipleinstalls holds the body of the TestMultipleInstalls
// e2e test.
package multipleinstalls

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

// Run installs kgateway twice into different namespaces using the supplied
// factory and runs the multiinstall feature suite against each install. CRDs
// are cluster-scoped and shared, so only the first Installation installs and
// uninstalls them.
func Run(t *testing.T, factory e2e.InstallationFactory) {
	ctx := t.Context()

	installs := []struct {
		namespace        string
		testInstallation e2e.Installation
	}{
		{
			namespace: "kgw-test-1",
			testInstallation: factory(
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
			testInstallation: factory(
				t,
				&install.Context{
					InstallNamespace:          "kgw-test-2",
					ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
					ValuesManifestFile:        e2e.ManifestPath("multiple_installs_values2.yaml"),
				},
			),
		},
	}

	// Register cleanup before installing so a partial install is still torn down.
	testutils.Cleanup(t, func() {
		ctx := context.Background()
		for _, inst := range installs {
			if t.Failed() {
				inst.testInstallation.PreFailHandler(ctx, t)
			}
			inst.testInstallation.UninstallKgatewayCore(ctx, t)
			cleanupPerInstall(ctx, inst.testInstallation, t)
		}
		installs[0].testInstallation.UninstallKgatewayCRDs(ctx, t)
	})

	for i, inst := range installs {
		if i == 0 {
			inst.testInstallation.InstallKgatewayCRDsFromLocalChart(ctx, t)
		}
		inst.testInstallation.InstallKgatewayCoreFromLocalChart(ctx, t)
		applyPerInstall(ctx, inst.testInstallation, t)
	}

	for _, inst := range installs {
		runner := multipleInstallsSuiteRunner(inst.namespace)
		runner.Run(ctx, t, inst.testInstallation.Underlying())
	}
}

func multipleInstallsSuiteRunner(namespace string) e2e.SuiteRunner {
	runner := e2e.NewSuiteRunner(false)
	runner.Register("Basic/"+namespace, multiinstall.NewTestingSuite)

	return runner
}

func applyPerInstall(ctx context.Context, ti e2e.Installation, t *testing.T) {
	namespace := ti.InstallContext().InstallNamespace

	err := ti.ActionsProvider().Kubectl().ApplyFile(ctx, multiinstall.BasicManifest, "-n", namespace)
	ti.Assertions(t).Require.NoError(err)
	for _, obj := range getPerInstallObjects(namespace) {
		ti.Assertions(t).EventuallyObjectsExist(ctx, obj)
	}

	err = ti.ActionsProvider().Kubectl().ApplyFile(ctx, defaults.CurlPodManifest)
	ti.Assertions(t).Require.NoError(err)
	ti.Assertions(t).EventuallyObjectsExist(ctx, defaults.CurlPod)
}

func cleanupPerInstall(ctx context.Context, ti e2e.Installation, t *testing.T) {
	namespace := ti.InstallContext().InstallNamespace

	err := ti.ActionsProvider().Kubectl().DeleteFileSafe(ctx, multiinstall.BasicManifest, "-n", namespace)
	ti.Assertions(t).Require.NoError(err)
	for _, obj := range getPerInstallObjects(namespace) {
		ti.Assertions(t).EventuallyObjectsNotExist(ctx, obj)
	}

	err = ti.ActionsProvider().Kubectl().DeleteFileSafe(ctx, defaults.CurlPodManifest)
	ti.Assertions(t).Require.NoError(err)
	ti.Assertions(t).EventuallyObjectsNotExist(ctx, defaults.CurlPod)
}

func getPerInstallObjects(ns string) []client.Object {
	return []client.Object{
		multiinstall.Gateway(ns), multiinstall.HttpbinRoute(ns), multiinstall.HttpbinDeployment(ns),
	}
}
