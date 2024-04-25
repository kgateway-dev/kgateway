package example

import (
	"path/filepath"
	"testing"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/example"
	"github.com/solo-io/gloo/test/kubernetes/testutils/gloogateway"
	"github.com/solo-io/skv2/codegen/util"
	"github.com/stretchr/testify/suite"
)

func (s *ClusterSuite) TestBasicInstallation() {

	var testInstallation *e2e.TestInstallation

	s.T().Run("setup", func(t *testing.T) {
		testInstallation = s.testCluster.RegisterTestInstallation(
			s.T(),
			&gloogateway.Context{
				InstallNamespace:   "basic-example",
				ValuesManifestFile: filepath.Join(util.MustGetThisDir(), "manifests", "basic-example.yaml"),
			},
		)
		testInstallation.InstallGlooGateway(s.ctx, testInstallation.Actions.Glooctl().NewTestHelperInstallAction())
	})

	s.T().Cleanup(func() {
		testInstallation.UninstallGlooGateway(s.ctx, testInstallation.Actions.Glooctl().NewTestHelperUninstallAction())
		s.testCluster.UnregisterTestInstallation(testInstallation)
	})

	s.T().Run("example feature", func(t *testing.T) {
		suite.Run(t, example.NewFeatureSuite(s.ctx, testInstallation))
	})

}
