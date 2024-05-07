package glooctl

import (
	"context"
	"fmt"
	"github.com/onsi/gomega"
	"os"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
)

type testingSuite struct {
	suite.Suite

	tmpDir string

	ctx              context.Context
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	var err error
	s.tmpDir, err = os.MkdirTemp("", "glooctl-test")
	s.NoError(err)
}

func (s *testingSuite) TearDownSuite() {
	_ = os.RemoveAll(s.tmpDir)
}

func (s *testingSuite) TestCheckCRDsErrorsForMismatch() {
	err := s.testInstallation.Actions.Glooctl().CheckCrds(s.ctx, "--version", "1.9.0")
	s.Error(err, "crds should be out of date")
	s.Contains(err.Error(), "One or more CRDs are out of date")
}

func (s *testingSuite) TestCheck() {
	output, err := s.testInstallation.Actions.Glooctl().Check(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace, "-x", "xds-metrics")
	s.NoError(err)

	for _, expectedOutput := range checkOutputByKey {
		gomega.Expect(output).To(expectedOutput.include)
	}
}

func (s *testingSuite) TestCheckExclude() {
	for excludeKey, expectedOutput := range checkOutputByKey {
		output, err := s.testInstallation.Actions.Glooctl().Check(s.ctx,
			"-n", s.testInstallation.Metadata.InstallNamespace, "-x", fmt.Sprintf("xds-metrics,%s", excludeKey))
		s.NoError(err)
		gomega.Expect(output).To(expectedOutput.exclude)
	}
}

func (s *testingSuite) TestCheckReadOnly() {
	output, err := s.testInstallation.Actions.Glooctl().Check(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace, "--read-only")
	s.NoError(err)

	for _, expectedOutput := range checkOutputByKey {
		gomega.Expect(output).To(gomega.And(
			expectedOutput.include,
			expectedOutput.readOnly,
		))
	}
}

func (s *testingSuite) TestCheckKubeContext() {
	// When passing an invalid kube-context, `glooctl check` should succeed
	_, err := s.testInstallation.Actions.Glooctl().Check(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace, "--kube-context", "invalid-context")
	s.Error(err)
	s.Contains(err.Error(), "Could not get kubernetes client: Error retrieving Kubernetes configuration: context \"invalid-context\" does not exist")

	// When passing the kube-context of the running cluster, `glooctl check` should succeed
	_, err = s.testInstallation.Actions.Glooctl().Check(s.ctx, "--kube-context", s.testInstallation.TestCluster.ClusterContext.KubeContext)
	s.NoError(err)
}

func (s *testingSuite) TestDebugLogsNoPanic() {
	err := s.testInstallation.Actions.Glooctl().DebugLogs(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace)
	s.NoError(err)
}
