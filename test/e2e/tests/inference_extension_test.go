//go:build e2e

package tests_test

import (
	"context"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	. "github.com/kgateway-dev/kgateway/v2/test/e2e/tests"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/cluster"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	testruntime "github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/runtime"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var (
	// infExtNs is the namespace to install kgateway
	infExtNs = "inf-ext-e2e"
)

// TestInferenceExtension tests Inference Extension functionality
func TestInferenceExtension(t *testing.T) {
	ctx := context.Background()

	runtimeContext := testruntime.NewContext()
	clusterContext := cluster.MustKindContextWithScheme(runtimeContext.ClusterName, schemes.InferExtScheme())

	installContext := &install.Context{
		InstallNamespace:          infExtNs,
		ProfileValuesManifestFile: e2e.ManifestPath("inference-extension-helm.yaml"),
		ValuesManifestFile:        e2e.EmptyValuesManifestPath,
	}

	testInstallation := e2e.CreateTestInstallationForCluster(
		t,
		runtimeContext,
		clusterContext,
		installContext,
	)

	// We register the cleanup function _before_ we actually perform the installation.
	// This allows us to uninstall kgateway, in case the original installation only completed partially
	testutils.Cleanup(t, func() {
		if t.Failed() {
			testInstallation.PreFailHandler(ctx)
		}

		testInstallation.UninstallKgateway(ctx)

		// Uninstall InferencePool v1 CRD
		err := testInstallation.Actions.Kubectl().DeleteFile(ctx, e2e.InferenceCrdManifest)
		testInstallation.Assertions.Require.NoError(err, "can delete manifest %s", e2e.InferenceCrdManifest)
	})

	// Install InferencePool v1 CRD
	err := testInstallation.Actions.Kubectl().ApplyFile(ctx, e2e.InferenceCrdManifest)
	testInstallation.Assertions.Require.NoError(err, "can apply manifest %s", e2e.InferenceCrdManifest)

	// Install kgateway
	testInstallation.InstallKgatewayFromLocalChart(ctx)
	testInstallation.Assertions.EventuallyNamespaceExists(ctx, infExtNs)

	// Run the e2e tests
	InferenceExtensionSuiteRunner().Run(ctx, t, testInstallation)
}
