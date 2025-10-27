//go:build e2e

package tests_test

import (
	"context"
	"os"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	. "github.com/kgateway-dev/kgateway/v2/test/e2e/tests"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/cluster"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	testruntime "github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/runtime"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

func TestAgentgatewayIntegration(t *testing.T) {
	ctx := context.Background()

	// Create the test installation
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "agent-gateway-test")
	runtimeContext := testruntime.NewContext()
	clusterContext := cluster.MustKindContextWithScheme(runtimeContext.ClusterName, schemes.InferExtScheme())
	installContext := &install.Context{
		InstallNamespace:          installNs,
		ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
		ValuesManifestFile:        e2e.ManifestPath("agent-gateway-integration.yaml"),
	}
	testInstallation := e2e.CreateTestInstallationForCluster(
		t,
		testruntime.NewContext(),
		clusterContext,
		installContext,
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
			testInstallation.PreFailHandler(ctx)
		}

		testInstallation.UninstallKgateway(ctx)

		// Uninstall Inference CRDs
		err := testInstallation.Actions.Kubectl().DeleteFile(ctx, e2e.InferenceCrdManifest)
		testInstallation.Assertions.Require.NoError(err, "can delete manifest %s", e2e.InferenceCrdManifest)
	})

	// Install Inference CRDs
	err := testInstallation.Actions.Kubectl().ApplyFile(ctx, e2e.InferenceCrdManifest)
	testInstallation.Assertions.Require.NoError(err, "can apply manifest %s", e2e.InferenceCrdManifest)

	// Install kgateway
	testInstallation.InstallKgatewayFromLocalChart(ctx)

	AgentgatewaySuiteRunner().Run(ctx, t, testInstallation)
}
