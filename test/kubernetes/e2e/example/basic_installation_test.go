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

func TestBasicInstallation(t *testing.T) {
	RegisterFailHandler(Fail)
	var testInstallation *e2e.TestInstallation

	testCluster := e2e.NewTestCluster()
	ctx := context.TODO()

	testInstallation = testCluster.RegisterTestInstallation(
		t,
		&gloogateway.Context{
			InstallNamespace:   "basic-example",
			ValuesManifestFile: filepath.Join(util.MustGetThisDir(), "manifests", "basic-example.yaml"),
		},
	)

	t.Run("install gateway", func(t *testing.T) {
		testInstallation.InstallGlooGateway(ctx, testInstallation.Actions.Glooctl().NewTestHelperInstallAction())
	})

	t.Cleanup(func() {
		testInstallation.UninstallGlooGateway(ctx, testInstallation.Actions.Glooctl().NewTestHelperUninstallAction())
		testCluster.UnregisterTestInstallation(testInstallation)
	})

	t.Run("example feature", func(t *testing.T) {
		suite.Run(t, example.NewFeatureSuite(ctx, testInstallation))
	})
}
