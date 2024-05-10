package glooctl

import (
	"context"
	"fmt"

	"github.com/onsi/gomega"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
)

// checkSuite contains the set of tests to validate the behavior of `glooctl check`
// These tests attempt to mirror: https://github.com/solo-io/gloo/blob/v1.16.x/test/kube2e/glooctl/check_test.go
type checkSuite struct {
	suite.Suite

	ctx              context.Context
	testInstallation *e2e.TestInstallation
}

func NewCheckSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &checkSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *checkSuite) TestCheck() {
	output, err := s.testInstallation.Actions.Glooctl().Check(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace, "-x", "xds-metrics")
	s.NoError(err)

	for _, expectedOutput := range checkOutputByKey {
		gomega.Expect(output).To(expectedOutput.include)
	}
}

func (s *checkSuite) TestCheckExclude() {
	for excludeKey, expectedOutput := range checkOutputByKey {
		output, err := s.testInstallation.Actions.Glooctl().Check(s.ctx,
			"-n", s.testInstallation.Metadata.InstallNamespace, "-x", fmt.Sprintf("xds-metrics,%s", excludeKey))
		s.NoError(err)
		gomega.Expect(output).To(expectedOutput.exclude)
	}
}

func (s *checkSuite) TestCheckReadOnly() {
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

func (s *checkSuite) TestCheckKubeContext() {
	// When passing an invalid kube-context, `glooctl check` should succeed
	_, err := s.testInstallation.Actions.Glooctl().Check(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace, "--kube-context", "invalid-context")
	s.Error(err)
	s.Contains(err.Error(), "Could not get kubernetes client: Error retrieving Kubernetes configuration: context \"invalid-context\" does not exist")

	// When passing the kube-context of the running cluster, `glooctl check` should succeed
	_, err = s.testInstallation.Actions.Glooctl().Check(s.ctx,
		"-n", s.testInstallation.Metadata.InstallNamespace, "--kube-context", s.testInstallation.ClusterContext.KubeContext)
	s.NoError(err)
}
