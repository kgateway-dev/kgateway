//go:build ignore

package tests_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/envutils"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	. "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/testutils/install"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// TestKgatewayAws is the function which executes a series of tests against a given installation
// with Kubernetes Gateway integration enabled and AWS lambda options configured
func TestKgatewayAws(t *testing.T) {
	ctx := context.Background()
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "aws-test")
	testInstallation := e2e.CreateTestInstallation(
		t,
		&install.Context{
			InstallNamespace:          installNs,
			ProfileValuesManifestFile: e2e.CommonRecommendationManifest,
			ValuesManifestFile:        e2e.ManifestPath("aws-lambda-helm.yaml"),
			// these should correspond to the `settings.aws.*` values in the aws-lambda-helm.yaml manifest
			AwsOptions: &kgateway.AwsOptions{
				EnableServiceAccountCredentials: true,
				StsCredentialsRegion:            "us-west-2",
			},
		},
	)

	testHelper := e2e.MustTestHelper(ctx, testInstallation)

	// Set the env to the install namespace if it is not already set
	if !nsEnvPredefined {
		os.Setenv(testutils.InstallNamespace, installNs)
	}

	// We register the cleanup function _before_ we actually perform the installation.
	// This allows us to uninstall Gloo Gateway, in case the original installation only completed partially
	t.Cleanup(func() {
		if !nsEnvPredefined {
			os.Unsetenv(testutils.InstallNamespace)
		}
		if t.Failed() {
			testInstallation.PreFailHandler(ctx)
		}

		testInstallation.UninstallGlooGatewayWithTestHelper(ctx, testHelper)
	})

	// Install Gloo Gateway
	testInstallation.InstallGlooGatewayWithTestHelper(ctx, testHelper, 5*time.Minute)

	KubeGatewayAwsSuiteRunner().Run(ctx, t, testInstallation)
}
