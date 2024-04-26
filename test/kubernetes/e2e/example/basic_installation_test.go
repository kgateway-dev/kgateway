package example

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/solo-io/gloo/test/kube2e/helper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/skv2/codegen/util"
	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/example"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
)

// TestBasicInstallation is the function which executes a series of tests against a given installation
func TestBasicInstallation(t *testing.T) {
	RegisterFailHandler(Fail)

	ctx := context.Background()
	testCluster := e2e.MustTestCluster()
	testInstallation := testCluster.RegisterTestInstallation(
		t,
		&gloogateway.Context{
			SkipGlooInstall:    e2e.SkipGlooInstall,
			InstallNamespace:   "basic-example",
			ValuesManifestFile: filepath.Join(util.MustGetThisDir(), "manifests", "basic-example.yaml"),
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
		testCluster.UnregisterTestInstallation(testInstallation)
	})

	t.Run("InstallGateway", func(t *testing.T) {
		testInstallation.InstallGlooGateway(ctx, func(ctx context.Context) error {
			return testHelper.InstallGloo(ctx, helper.GATEWAY, 5*time.Minute, helper.ExtraArgs("--values", testInstallation.Metadata.ValuesManifestFile))
		})
	})

	// The name here is important for debuggability
	// When tests are logged, they follow the shape TestSuiteName/SubtestName/TestName
	// In this case, the output would be:
	// TestBasicInstallation/Example/{test name}
	// We prefer to follow CamelCase convention for names of these sub-tests
	t.Run("Example", func(t *testing.T) {
		suite.Run(t, example.NewTestingSuite(ctx, testInstallation))
	})
}
