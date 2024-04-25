package example

import (
	"context"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/solo-io/skv2/codegen/util"
	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/example"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
)

// TestComplexInstallation is the function which executes a series of tests against a given installation
func TestComplexInstallation(t *testing.T) {
	RegisterFailHandler(Fail)

	ctx := context.Background()
	testCluster := e2e.NewTestCluster()
	testInstallation := testCluster.RegisterTestInstallation(
		t,
		&gloogateway.Context{
			InstallNamespace:   "complex-example",
			ValuesManifestFile: filepath.Join(util.MustGetThisDir(), "manifests", "complex-example.yaml"),
		},
	)

	// We register the cleanup function _before_ we actually perform the installation.
	// This allows us to uninstall Gloo Gateway, in case the original installation only completed partially
	t.Cleanup(func() {
		if t.Failed() {
			testInstallation.PreFailHandler()
		}

		testInstallation.UninstallGlooGateway(ctx, testInstallation.Actions.Glooctl().NewTestHelperUninstallAction())
		testCluster.UnregisterTestInstallation(testInstallation)
	})

	t.Run("install gateway", func(t *testing.T) {
		testInstallation.InstallGlooGateway(ctx, testInstallation.Actions.Glooctl().NewTestHelperInstallAction())
	})

	t.Run("example feature", func(t *testing.T) {
		suite.Run(t, example.NewFeatureSuite(ctx, testInstallation))
	})
}
