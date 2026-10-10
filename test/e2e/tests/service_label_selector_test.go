//go:build e2e

package tests_test

import (
	"context"
	"os"
	"testing"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	. "github.com/kgateway-dev/kgateway/v2/test/e2e/tests"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// TestServiceLabelSelector uses an isolated controller installation because Service discovery
// selectors apply globally to every Gateway managed by that controller.
func TestServiceLabelSelector(t *testing.T) {
	ctx := context.Background()
	installNamespace, namespacePredefined := envutils.LookupOrDefault(
		testutils.InstallNamespace,
		"kgateway-service-label-selector-test",
	)
	testInstallation := e2e.CreateTestInstallation(
		t,
		&install.Context{
			InstallNamespace:          installNamespace,
			ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
			ValuesManifestFile:        e2e.ManifestPath("service-label-selector-helm.yaml"),
			ExtraHelmArgs: []string{
				"--set", "controller.extraEnv.KGW_GLOBAL_POLICY_NAMESPACE=" + installNamespace,
			},
		},
	)

	if !namespacePredefined {
		if err := os.Setenv(testutils.InstallNamespace, installNamespace); err != nil {
			t.Fatalf("failed to set %s: %v", testutils.InstallNamespace, err)
		}
	}

	testutils.Cleanup(t, func() {
		if !namespacePredefined {
			if err := os.Unsetenv(testutils.InstallNamespace); err != nil {
				t.Errorf("failed to unset %s: %v", testutils.InstallNamespace, err)
			}
		}
		testInstallation.UninstallKgateway(context.Background(), t)
	})

	testInstallation.InstallKgatewayFromLocalChart(ctx, t)
	ServiceLabelSelectorSuiteRunner().Run(ctx, t, testInstallation)
}
