package glooctl

import (
	"context"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
	"os"
	"path/filepath"
)

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

	s.tmpDir, err = os.MkdirTemp("", "debug-suite-dir")
	s.Require().NoError(err)
}

func (s *debugSuite) TearDownSuite() {
	_ = os.RemoveAll(s.tmpDir)
}

func (s *debugSuite) TestLogsNoPanic() {
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

/**

It("should create a tar file at location specified in --file when --zip is enabled", func() {
				outputFile := filepath.Join(tmpDir, "log.tgz")

				_, err := GlooctlOut("debug", "logs", "-n", testHelper.InstallNamespace, "--file", outputFile, "--zip", "true")
				Expect(err).NotTo(HaveOccurred(), "glooctl command should have succeeded")

				_, err = os.Stat(outputFile)
				Expect(err).NotTo(HaveOccurred(), "Output file should have been generated")
			})

			It("should create a text file at location specified in --file when --zip is not enabled", func() {
				outputFile := filepath.Join(tmpDir, "log.txt")

				_, err := GlooctlOut("debug", "logs", "-n", testHelper.InstallNamespace, "--file", outputFile, "--zip", "false")
				Expect(err).NotTo(HaveOccurred(), "glooctl command should have succeeded")

				_, err = os.Stat(outputFile)
				Expect(err).NotTo(HaveOccurred(), "Output file should have been generated")
			})
*/
