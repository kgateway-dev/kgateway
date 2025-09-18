package tests_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/crds"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	. "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/testutils/cluster"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/testutils/install"
	testruntime "github.com/kgateway-dev/kgateway/v2/test/kubernetes/testutils/runtime"
)

var (
	// poolCrdManifest defines the manifest file containing Inference Extension CRDs.
	// Created using command:
	//   kubectl kustomize "https://github.com/kubernetes-sigs/gateway-api-inference-extension/config/crd/?ref=$COMMIT_SHA" \
	//   > internal/kgateway/crds/inference-crds.yaml
	poolCrdManifest = filepath.Join(crds.AbsPathToCrd("inference-crds.yaml"))
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
	t.Cleanup(func() {
		if t.Failed() {
			testInstallation.PreFailHandler(ctx)
		}

		testInstallation.UninstallKgateway(ctx)

		// Uninstall InferencePool v1 CRD
		err := testInstallation.Actions.Kubectl().DeleteFile(ctx, poolCrdManifest)
		testInstallation.Assertions.Require.NoError(err, "can delete manifest %s", poolCrdManifest)
	})

	// Install InferencePool v1 CRD
	err := testInstallation.Actions.Kubectl().ApplyFile(ctx, poolCrdManifest)
	testInstallation.Assertions.Require.NoError(err, "can apply manifest %s", poolCrdManifest)

	// Install kgateway
	testInstallation.InstallKgatewayFromLocalChart(ctx)
	testInstallation.Assertions.EventuallyNamespaceExists(ctx, infExtNs)

	// Run the e2e tests
	InferenceExtensionSuiteRunner().Run(ctx, t, testInstallation)
}
