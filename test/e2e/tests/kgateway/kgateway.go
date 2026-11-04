//go:build e2e

// Package kgateway holds the body of the TestKgateway e2e test.
package kgateway

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/testutils/localstack"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// Run executes the TestKgateway scenario against the Installation produced by
// the supplied factory.
func Run(t *testing.T, factory e2e.InstallationFactory) {
	ctx := t.Context()
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "kgateway-test")
	installContext := &install.Context{
		InstallNamespace:          installNs,
		ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
		ValuesManifestFile:        e2e.EmptyValuesManifestPath,
		ExtraHelmArgs: []string{
			"--set", "controller.enableAwsEc2Discovery=true",
			"--set", "controller.extraEnv.KGW_GLOBAL_POLICY_NAMESPACE=" + installNs,
			"--set", "policyMerge.trafficPolicy.extProc=DeepMerge",
		},
	}
	testInstallation := factory(t, installContext)

	localstackEndpoint, localstackFound, err := localstack.EndpointURL(ctx, testInstallation.Cluster().Client)
	if err != nil {
		t.Fatalf("failed to resolve localstack endpoint: %v", err)
	}
	if localstackFound {
		installContext.ExtraHelmArgs = append(
			installContext.ExtraHelmArgs,
			"--set-string", "controller.extraEnv.AWS_ENDPOINT_URL_EC2="+localstackEndpoint,
		)
	}
	if validationMode := os.Getenv("VALIDATION_MODE"); validationMode != "" {
		installContext.ExtraHelmArgs = append(installContext.ExtraHelmArgs, "--set", "validation.level="+validationMode)
	}

	// Set the env to the install namespace if it is not already set
	if !nsEnvPredefined {
		os.Setenv(testutils.InstallNamespace, installNs)
	}

	// We register the cleanup function _before_ we actually perform the installation.
	// This allows us to uninstall kgateway, in case the original installation only completed partially
	testutils.Cleanup(t, func() {
		ctx := context.Background() // t.Context() is canceled before t's cleanup runs
		if !nsEnvPredefined {
			os.Unsetenv(testutils.InstallNamespace)
		}

		testInstallation.UninstallKgateway(ctx, t)
	})

	// Install kgateway
	testInstallation.InstallKgatewayFromLocalChart(ctx, t)

	// The base gateway has two variants: one with "allowedListeners" to support ListenerSets to and one without.
	// The gateway to apply is selected based on whether the installed Gateway API version supports ListenerSets.
	// If the gateway with "allowedListeners"  is applied with a Gateway API version that doesn't support it, the resource will be rejected.

	channel, version, err := base.DetectGwApiInfo(ctx, testInstallation.Cluster().Client)
	if err != nil {
		t.Fatalf("failed to detect Gateway API version: %v", err)
	}
	gatewayManifest := "kgateway-base-gateway.yaml"
	if base.SupportsListenerSets(channel, version) {
		gatewayManifest = "kgateway-base-gateway-listenersets.yaml"
	}

	// Apply the base gateway once, then the shared nginx backend that suites reference
	// instead of each deploying their own. Manifest paths are anchored to this
	// package's directory so external callers can run this from anywhere.
	manifestsDir := filepath.Join(fsutils.MustGetThisDir(), "..", "manifests")
	common.SetupBaseConfig(ctx, t, testInstallation.Underlying(),
		filepath.Join(manifestsDir, "kgateway-base.yaml"),
		filepath.Join(manifestsDir, gatewayManifest),
	)
	common.SetupSharedNginxBackend(ctx, t, testInstallation.Underlying())
	common.SetupBaseGateway(ctx, t, testInstallation.Underlying(), types.NamespacedName{
		Namespace: "kgateway-base",
		Name:      "gateway",
	})

	SuiteRunner().Run(ctx, t, testInstallation.Underlying())
}
