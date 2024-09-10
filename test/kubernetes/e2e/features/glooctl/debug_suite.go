package glooctl

import (
	"context"
	"os"
	"path/filepath"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
)

var _ e2e.NewSuiteFunc = NewDebugSuite

// debugSuite contains the set of tests to validate the behavior of `glooctl debug`
// These tests attempt to mirror: https://github.com/solo-io/gloo/blob/v1.16.x/test/kube2e/glooctl/debug_test.go
type debugSuite struct {
	suite.Suite

	tmpDir string

	ctx              context.Context
	testInstallation *e2e.TestInstallation
}

func NewDebugSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &debugSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *debugSuite) SetupSuite() {
	var err error

	s.tmpDir, err = os.MkdirTemp(s.testInstallation.GeneratedFiles.TempDir, "debug-suite-dir")
	s.Require().NoError(err)
}

func (s *debugSuite) TearDownSuite() {
	_ = os.RemoveAll(s.tmpDir)
}

func (s *debugSuite) TestLogsNoPanic() {
	// check logs to stdout do not crash
	err := s.testInstallation.Actions.Glooctl().DebugLogs(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace)
	s.NoError(err)
}

func (s *debugSuite) TestLogsZipFile() {
	outputFile := filepath.Join(s.tmpDir, "log.tgz")

	err := s.testInstallation.Actions.Glooctl().DebugLogs(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace, "--file", outputFile, "--zip", "true")
	s.NoError(err)

	_, err = os.Stat(outputFile)
	s.NoError(err, "Output file should have been generated")
}

func (s *debugSuite) TestLogsFile() {
	outputFile := filepath.Join(s.tmpDir, "log.txt")

	err := s.testInstallation.Actions.Glooctl().DebugLogs(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace, "--file", outputFile, "--zip", "false")
	s.NoError(err)

	_, err = os.Stat(outputFile)
	s.NoError(err, "Output file should have been generated")
}
