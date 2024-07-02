package glooctl

import (
	"context"
	"os"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
)

var _ e2e.NewSuiteFunc = NewDebugSuite

// checkCrdsSuite contains the set of tests to validate the behavior of `glooctl check-crds`
type checkCrdsSuite struct {
	suite.Suite

	tmpDir string

	ctx              context.Context
	testInstallation *e2e.TestInstallation
}

func NewCheckCrdsSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &checkCrdsSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *checkCrdsSuite) SetupSuite() {
	var err error

	s.tmpDir, err = os.MkdirTemp(s.testInstallation.GeneratedFiles.TempDir, "debug-suite-dir")
	s.Require().NoError(err)
}

func (s *checkCrdsSuite) TearDownSuite() {
	_ = os.RemoveAll(s.tmpDir)
}

// TODO: get chart uri
//func (s *checkCrdsSuite) TestValidatesCorrectCrds() {
//	// TODO(npolshak): We currently can't run against a released version because our testInstallation does not store ReleaseVersion
//	// Need this info to run `glooctl check-crds --version <chart version>
//
//	chartUri := filepath.Join(util.GetModuleRoot(), s.testInstallation.TestAssetDir, testHelper.HelmChartName+"-"+testHelper.ChartVersion()+".tgz")
//	err := s.testInstallation.Actions.Glooctl().CheckCrds(s.ctx,
//		"-n", s.testInstallation.Metadata.InstallNamespace, "--local-chart", chartUri)
//	s.NoError(err)
//}

func (s *checkCrdsSuite) TestCrdMismatch() {
	err := s.testInstallation.Actions.Glooctl().CheckCrds(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace, "--file",
		"--version", "1.9.0")
	s.Error(err)
	s.Contains(err.Error(), "One or more CRDs are out of date")
}
