package example

// This file is an example for developers.

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/gloo/test/kubernetes/testutils/helper"

	"github.com/solo-io/skv2/codegen/util"
	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/example"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
)

// TestInstallationWithDebugLogLevel is the function which executes a series of tests against a given installation
func TestInstallationWithDebugLogLevel(t *testing.T) {
	ctx := context.Background()
	testInstallation := e2e.CreateTestInstallation(
		t,
		&gloogateway.Context{
			InstallNamespace:   "debug-example",
			ValuesManifestFile: filepath.Join(util.MustGetThisDir(), "manifests", "debug-example.yaml"),
		},
	)

	testHelper := e2e.MustTestHelper(ctx, testInstallation)

	// We register the cleanup function _before_ we actually perform the installation.
	// This allows us to uninstall Gloo Gateway, in case the original installation only completed partially
	t.Cleanup(func() {
		if t.Failed() {
			testInstallation.PreFailHandler(ctx)
		}

		testInstallation.UninstallGlooGateway(ctx, func(ctx context.Context) error {
			return testHelper.UninstallGlooAll()
		})
	})

	testInstallation.InstallGlooGateway(ctx, func(ctx context.Context) error {
		return testHelper.InstallGloo(ctx, 5*time.Minute, helper.WithExtraArgs("--values", testInstallation.Metadata.ValuesManifestFile))
	})

	// The name here is important for debuggability
	// When tests are logged, they follow the shape TestSuiteName/SubtestName/TestName
	// In this case, the output would be:
	// TestInstallationWithDebugLogLevel/Example/{test name}
	// We prefer to follow CamelCase convention for names of these subtests
	t.Run("Example", func(t *testing.T) {
		suite.Run(t, example.NewTestingSuite(ctx, testInstallation))
	})
}
