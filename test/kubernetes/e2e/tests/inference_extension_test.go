package tests_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/crds"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	. "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/testutils/install"
)

var (
	// Inference Extension CRDs.
	poolCrdManifest  = filepath.Join(crds.AbsPathToCrd("inferencepools.yaml"))
	modelCrdManifest = filepath.Join(crds.AbsPathToCrd("inferencemodels.yaml"))
	// infExtNs is the namespace to install kgateway
	infExtNs = "inf-ext-e2e"
)

// TestInferenceExtension tests Inference Extension functionality
func TestInferenceExtension(t *testing.T) {
	ctx := context.Background()
	testInstallation := e2e.CreateTestInstallation(
		t,
		&install.Context{
			InstallNamespace:          infExtNs,
			ProfileValuesManifestFile: e2e.ManifestPath("inference-extension-helm.yaml"),
			ValuesManifestFile:        e2e.EmptyValuesManifestPath,
		},
	)

	// We register the cleanup function _before_ we actually perform the installation.
	// This allows us to uninstall kgateway, in case the original installation only completed partially
	t.Cleanup(func() {
		if t.Failed() {
			testInstallation.PreFailHandler(ctx)
		}

		testInstallation.UninstallKgateway(ctx)

		// Uninstall CRDs
		for _, m := range []string{poolCrdManifest, modelCrdManifest} {
			err := testInstallation.Actions.Kubectl().DeleteFile(ctx, m)
			testInstallation.Assertions.Require.NoError(err, "can delete manifest %s", m)
		}
	})

	// Install CRDs
	for _, m := range []string{poolCrdManifest, modelCrdManifest} {
		err := testInstallation.Actions.Kubectl().ApplyFile(ctx, m)
		testInstallation.Assertions.Require.NoError(err, "can apply manifest %s", m)
	}

	// Install kgateway
	testInstallation.InstallKgatewayFromLocalChart(ctx)
	testInstallation.Assertions.EventuallyNamespaceExists(ctx, infExtNs)

	// Run the e2e tests
	InferenceExtensionSuiteRunner().Run(ctx, t, testInstallation)
}
