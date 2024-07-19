package validation_strict

import (
	"context"

	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/stretchr/testify/suite"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is the entire Suite of tests for the webhook validation alwaysAccept=false feature
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

// TestInvalidUpstream tests behaviors when Gloo rejects an invalid upstream
func (s *testingSuite) TestInvalidUpstream() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, invalidUpstream)
	s.Assert().Error(err, "admission webhook error exists")
	s.Assert().Contains(err.Error(), "admission webhook error")
	s.Assert().Contains(err.Error(), "port cannot be empty for host")
}

func (s *testingSuite) TestVirtualServiceWithSecretDeletion() {
	// VS with secret should be accepted
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, upstream)
	s.Assert().NoError(err)
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, vs)
	s.Assert().NoError(err)

	// Rejecting resource patches due to existing warnings

	// failing to delete a secret that is in use
	s.Assert().Error(err, "expect failure when deleting secret in use")
	s.Assert().Contains(err.Error(), testmatchers.ContainSubstrings([]string{"admission webhook", "SSL secret not found", secretName}))

	// deleting a secret that is not in use
}

func (s *testingSuite) TestRejectsInvalidGatewayResources() {

}
