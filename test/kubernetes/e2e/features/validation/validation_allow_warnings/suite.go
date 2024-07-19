package validation_allow_warnings

import (
	"context"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/validation/validation_types"
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

/*
TestSecretDeletion tests behaviors when a secret is deleted

To create the private key and certificate to use:

	openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
	   -keyout tls.key -out tls.crt -subj "/CN=*"

To create the Kubernetes secrets to hold this cert:

	kubectl create secret tls upstream-tls --key tls.key \
	   --cert tls.crt --namespace gloo-system
*/
func (s *testingSuite) TestInvalidUpstream() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation_types.ExampleUpstream)
	s.Assert().NoError(err)
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation_types.ExampleUpstream)
	s.Assert().NoError(err)
}

func (s *testingSuite) TestVirtualServiceWithSecret() {
	// VS with secret should be accepted
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation_types.ExampleUpstream)
	s.Assert().NoError(err)
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation_types.SecretVS)
	s.Assert().NoError(err)

	// Rejecting resource patches due to existing warnings

	// when allowWarnings=true, should be able to delete a secret that is in use

	// deleting a secret that is not in use
}

// TODO: map behavior for other cases
// TestRejectTransformation checks webhook rejects invalid transformation when server_enabled=true
func (s *testingSuite) TestRejectTransformation() {
	// accepts invalid inja template in transformation

	// accepts invalid subgroup in transformation

	// accepts invalid subgroup in transformation
}
