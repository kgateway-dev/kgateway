package example

// This file is an example for developers.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/gloo/pkg/utils/envutils"
	"github.com/solo-io/gloo/test/testutils"

	"github.com/solo-io/skv2/codegen/util"
	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/example"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
)

// TestInstallationWithInfoLogLevel is the function which executes a series of tests against a given installation
func TestInstallationWithInfoLogLevel(t *testing.T) {
	ctx := context.Background()
	installNs, nsEnvPredefined := envutils.LookupOrDefault(testutils.InstallNamespace, "info-log-test")
	testInstallation := e2e.CreateTestInstallation(
		t,
		&gloogateway.Context{
			InstallNamespace:          installNs,
			ProfileValuesManifestFile: filepath.Join(util.MustGetThisDir(), "manifests", "example-profile.yaml"),
			ValuesManifestFile:        filepath.Join(util.MustGetThisDir(), "manifests", "info-example.yaml"),
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

	testInstallation.InstallGlooGatewayWithTestHelper(ctx, testHelper, 5*time.Minute)

	// The name here is important for debuggability
	// When tests are logged, they follow the shape TestSuiteName/SubtestName/TestName
	// In this case, the output would be:
	// TestInstallationWithDebugLogLevel/Example/{test name}
	// We prefer to follow CamelCase convention for names of these subtests
	t.Run("Example", func(t *testing.T) {
		suite.Run(t, example.NewTestingSuite(ctx, testInstallation))
	})
}
