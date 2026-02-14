//go:build e2e

package tests_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/crds"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	. "github.com/kgateway-dev/kgateway/v2/test/e2e/tests"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/cluster"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	testruntime "github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/runtime"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var (
	// poolCrdManifest defines the manifest file containing Inference Extension CRDs.
	// Created using command:
	//   kubectl kustomize "https://github.com/kubernetes-sigs/gateway-api-inference-extension/config/crd/?ref=$COMMIT_SHA" \
	//   > pkg/kgateway/crds/inference-crds.yaml
	poolCrdManifest = filepath.Join(crds.AbsPathToCrd("inference-crds.yaml"))
)

func TestAgentgatewayIntegration(t *testing.T) {
	ctx := context.Background()
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
		runtimeContext,
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

		// Uninstall InferencePool v1 CRD
		err := testInstallation.Actions.Kubectl().DeleteFileSafe(ctx, poolCrdManifest)
		testInstallation.Assertions.Require.NoError(err, "can delete manifest %s", poolCrdManifest)
	})

	// Install InferencePool v1 CRD
	err := testInstallation.Actions.Kubectl().ApplyFile(ctx, poolCrdManifest)
	testInstallation.Assertions.Require.NoError(err, "can apply manifest %s", poolCrdManifest)

	// Install kgateway
	testInstallation.InstallKgatewayFromLocalChart(ctx)
	testInstallation.Assertions.EventuallyNamespaceExists(ctx, installNs)

	AgentgatewaySuiteRunner().Run(ctx, t, testInstallation)

}
