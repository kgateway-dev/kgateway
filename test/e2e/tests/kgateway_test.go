//go:build e2e

package tests_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	. "github.com/kgateway-dev/kgateway/v2/test/e2e/tests"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

func TestKgateway(t *testing.T) {
	ctx := context.Background()
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "kgateway-test")
	testInstallation := e2e.CreateTestInstallation(
		t,
		&install.Context{
			InstallNamespace:          installNs,
			ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
			ValuesManifestFile:        e2e.EmptyValuesManifestPath,
			ExtraHelmArgs: []string{
				"--set", "controller.extraEnv.KGW_GLOBAL_POLICY_NAMESPACE=" + installNs,
			},
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

		testInstallation.UninstallKgateway(ctx, t)
	})

	// Install kgateway
	testInstallation.InstallKgatewayFromLocalChart(ctx, t)

	// The base gateway has two variants: the ListenerSet-capable one sets
	// spec.allowedListeners, which only exists once ListenerSets are available. On older
	// Gateway API versions that field would be pruned, so pick the variant that matches the
	// installed API version (mirrors the per-suite version gating in test/e2e/tests/base).
	channel, version, err := base.DetectGwApiInfo(ctx, testInstallation.ClusterContext.Client)
	if err != nil {
		t.Fatalf("failed to detect Gateway API version: %v", err)
	}
	gatewayManifest := "kgateway-base-gateway.yaml"
	if base.SupportsListenerSets(channel, version) {
		gatewayManifest = "kgateway-base-gateway-listenersets.yaml"
	}

	common.SetupBaseConfig(ctx, t, testInstallation,
		filepath.Join("manifests", "kgateway-base.yaml"),
		filepath.Join("manifests", gatewayManifest),
	)
	common.SetupBaseGateway(ctx, t, testInstallation, types.NamespacedName{
		Namespace: "kgateway-base",
		Name:      "gateway",
	})

	KubeGatewaySuiteRunner().Run(ctx, t, testInstallation)
}
