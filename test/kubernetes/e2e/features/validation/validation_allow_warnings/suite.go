package validation_allow_warnings

import (
	"context"

	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/validation"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
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
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.ExampleUpstream, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.ExampleUpstream, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)
}

func (s *testingSuite) TestVirtualServiceWithSecret() {
	s.T().Cleanup(func() {
		// Can delete resources in correct order
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation.SecretVS, "-n", s.testInstallation.Metadata.InstallNamespace)
		s.Assert().NoError(err)

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation.ExampleUpstream, "-n", s.testInstallation.Metadata.InstallNamespace)
		s.Assert().NoError(err)
	})

	// Secrets should be accepted
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.Secret, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.UnusedSecret, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)

	// Upstream should be accepted
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.ExampleUpstream, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)
	helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
		return s.testInstallation.ResourceClients.UpstreamClient().Read(s.testInstallation.Metadata.InstallNamespace, validation.ExampleUpstreamName, clients.ReadOpts{Ctx: s.ctx})
	})
	// VS with secret should be accepted
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.SecretVS, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)
	helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
		return s.testInstallation.ResourceClients.VirtualServiceClient().Read(s.testInstallation.Metadata.InstallNamespace, validation.ExampleVsName, clients.ReadOpts{Ctx: s.ctx})
	})

	// when allowWarnings=true, should be able to delete a secret that is in use
	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation.Secret, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)

	// Now VirtualService should have warning
	helpers.EventuallyResourceWarning(func() (resources.InputResource, error) {
		return s.testInstallation.ResourceClients.VirtualServiceClient().Read(s.testInstallation.Metadata.InstallNamespace, validation.ExampleVsName, clients.ReadOpts{Ctx: s.ctx})
	})

	// deleting a secret that is not in use should also work
	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation.UnusedSecret, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)

}

// TODO: map behavior for other cases (strict should reject, permissive should accept?)
// TestRejectTransformation checks webhook rejects invalid transformation when server_enabled=true
func (s *testingSuite) TestRejectTransformation() {
	// reject invalid inja template in transformation
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.VSTransformationHeaderText, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().Error(err)
	s.Assert().Contains(err.Error(), "Failed to parse response template: Failed to parse "+
		"header template ':status': [inja.exception.parser_error] (at 1:92) expected statement close, got '%'")

	// Extract mode -- rejects invalid subgroup in transformation
	// note that the regex has no subgroups, but we are trying to extract the first subgroup
	// this should be rejected
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.VSTransformationHeaderText, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().Error(err)
	s.Assert().Contains(err.Error(), "envoy validation mode output: error initializing configuration '': Failed to parse response template: group 1 requested for regex with only 0 sub groups")

	// Single replace mode -- rejects invalid subgroup in transformation
	// note that the regex has no subgroups, but we are trying to extract the first subgroup
	// this should be rejected
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.VSTransformationSingleReplace, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().Error(err)
	s.Assert().Contains(err.Error(), "envoy validation mode output: error initializing configuration '': Failed to parse response template: group 1 requested for regex with only 0 sub groups")
}
