package tests_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/crds"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	. "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var (
	// Inference Extension CRDs.
	poolCrdManifest  = filepath.Join(crds.AbsPathToCrd("inferencepools.yaml"))
	modelCrdManifest = filepath.Join(crds.AbsPathToCrd("inferencemodels.yaml"))
)

// TestInferenceExtension tests Inference Extension functionality
func TestInferenceExtension(t *testing.T) {
	ctx := context.Background()
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "inf-ext-e2e")
	testInstallation := e2e.CreateTestInstallation(
		t,
		&install.Context{
			InstallNamespace:          installNs,
			ProfileValuesManifestFile: e2e.InfExtValuesManifestPath,
			ValuesManifestFile:        e2e.EmptyValuesManifestPath,
		},
	)

	// Set the env to the install namespace if not already set
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
	testInstallation.Assertions.EventuallyNamespaceExists(ctx, installNs)

	// Run the e2e tests
	InferenceExtensionSuiteRunner().Run(ctx, t, testInstallation)
}
