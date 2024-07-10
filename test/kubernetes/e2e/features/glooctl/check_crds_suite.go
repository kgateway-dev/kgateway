package glooctl

import (
	"context"
	"path/filepath"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/testutils"
	"github.com/stretchr/testify/suite"
)

var _ e2e.NewSuiteFunc = NewDebugSuite

// checkCrdsSuite contains the set of tests to validate the behavior of `glooctl check-crds`
type checkCrdsSuite struct {
	suite.Suite

	ctx              context.Context
	testInstallation *e2e.TestInstallation
}

func NewCheckCrdsSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &checkCrdsSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *checkCrdsSuite) TestValidatesCorrectCrds() {
	if s.testInstallation.Metadata.ReleasedVersion != "" {
		err := s.testInstallation.Actions.Glooctl().CheckCrds(s.ctx,
			"-n", s.testInstallation.Metadata.InstallNamespace,
			"--kube-context", s.testInstallation.ClusterContext.KubeContext,
			"--version", s.testInstallation.Metadata.ChartVersion)
		s.NoError(err)
	} else {
		chartUri := filepath.Join(testutils.GitRootDirectory(), s.testInstallation.Metadata.TestAssetDir, s.testInstallation.Metadata.HelmChartName+"-"+s.testInstallation.Metadata.ChartVersion+".tgz")
		err := s.testInstallation.Actions.Glooctl().CheckCrds(s.ctx,
			"-n", s.testInstallation.Metadata.InstallNamespace,
			"--kube-context", s.testInstallation.ClusterContext.KubeContext,
			"--local-chart", chartUri)
		s.NoError(err)
	}
}

func (s *checkCrdsSuite) TestCrdMismatch() {
	err := s.testInstallation.Actions.Glooctl().CheckCrds(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace,
		"--kube-context", s.testInstallation.ClusterContext.KubeContext,
		"--version", "1.9.0")
	s.Error(err)
	s.Contains(err.Error(), "One or more CRDs are out of date")
}
