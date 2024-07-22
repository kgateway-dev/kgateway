package validation_strict

import (
	"context"
	"strings"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/validation"
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

//// TestInvalidUpstream tests behaviors when Gloo rejects an invalid upstream with
//func (s *testingSuite) TestInvalidUpstream() {
//	output, err := s.testInstallation.Actions.Kubectl().ApplyFileWithOutput(s.ctx, validation.InvalidUpstreamNoPort, "-n", s.testInstallation.Metadata.InstallNamespace)
//	s.Assert().Error(err, "admission webhook error exists")
//	s.Assert().Contains(output, "admission webhook error")
//	s.Assert().Contains(output, "port cannot be empty for host")
//
//	// TODO(npolshak): why is no-valid-host getting accepted? ***
//}

//func (s *testingSuite) TestVirtualServiceWithSecretDeletion() {
//	s.T().Cleanup(func() {
//		// Can delete resources in correct order
//		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation.SecretVS, "-n", s.testInstallation.Metadata.InstallNamespace)
//		s.Assert().NoError(err)
//
//		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation.ExampleUpstream, "-n", s.testInstallation.Metadata.InstallNamespace)
//		s.Assert().NoError(err)
//
//		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation.Secret, "-n", s.testInstallation.Metadata.InstallNamespace)
//		s.Assert().NoError(err)
//	})
//
//	// Secrets should be accepted
//	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.Secret, "-n", s.testInstallation.Metadata.InstallNamespace)
//	s.Assert().NoError(err)
//	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.UnusedSecret, "-n", s.testInstallation.Metadata.InstallNamespace)
//	s.Assert().NoError(err)
//
//	// Upstream should be accepted
//	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.ExampleUpstream, "-n", s.testInstallation.Metadata.InstallNamespace)
//	s.Assert().NoError(err)
//	helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
//		return s.testInstallation.ResourceClients.UpstreamClient().Read(s.testInstallation.Metadata.InstallNamespace, validation.ExampleUpstreamName, clients.ReadOpts{Ctx: s.ctx})
//	})
//	// VS with secret should be accepted
//	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.SecretVS)
//	s.Assert().NoError(err)
//	helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
//		return s.testInstallation.ResourceClients.VirtualServiceClient().Read(s.testInstallation.Metadata.InstallNamespace, validation.ExampleVsName, clients.ReadOpts{Ctx: s.ctx})
//	})
//
//	// failing to delete a secret that is in use
//	output, err := s.testInstallation.Actions.Kubectl().DeleteFileWithOutput(s.ctx, validation.Secret)
//	s.Assert().Error(err)
//	s.Assert().Contains(output, testmatchers.ContainSubstrings([]string{"admission webhook", "SSL secret not found", validation.SecretName}))
//
//	// deleting a secret that is not in use works
//	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation.UnusedSecret, "-n", s.testInstallation.Metadata.InstallNamespace)
//	s.Assert().NoError(err)
//}

func (s *testingSuite) TestRejectsInvalidGatewayResources() {
	output, err := s.testInstallation.Actions.Kubectl().ApplyFileWithOutput(s.ctx, validation.InvalidGateway, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().Error(err)
	s.Assert().Contains(output, `admission webhook "gloo.validation-strict-test.svc" denied the request`)
	s.Assert().Contains(output, `Validating *v1.Gateway failed: validating *v1.Gateway name:"gateway-without-type" namespace:"validation-strict-test": 1 error occurred`)
	s.Assert().Contains(output, "invalid gateway: gateway must contain gatewayType")
}

func (s *testingSuite) TestRejectsInvalidRatelimitConfigResources() {
	output, _ := s.testInstallation.Actions.Kubectl().ApplyFileWithOutput(s.ctx, validation.InvalidRLC, "-n", s.testInstallation.Metadata.InstallNamespace)
	// We don't expect an error exit code here because this is a warning
	s.Assert().Contains(output, `admission webhook "gloo.validation-strict-test.svc" denied the request`)
	s.Assert().Contains(output, `Validating *v1alpha1.RateLimitConfig failed: validating *v1alpha1.RateLimitConfig name:"rlc" namespace:"validation-strict-test": 1 error occurred`)
	s.Assert().Contains(output, "The Gloo Advanced Rate limit API feature 'RateLimitConfig' is enterprise-only, please upgrade or use the Envoy rate-limit API instead")
}

func (s *testingSuite) TestRejectsInvalidVSMethodMatcher() {
	output, err := s.testInstallation.Actions.Kubectl().ApplyFileWithOutput(s.ctx, validation.InvalidVirtualServiceMatcher, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().Error(err)
	s.Assert().Contains(output, `admission webhook "gloo.validation-strict-test.svc" denied the request`)
	s.Assert().Contains(output, `Validating *v1.VirtualService failed: validating *v1.VirtualService name:"method-matcher" namespace:"validation-strict-test": 1 error occurred`)
	s.Assert().Contains(output, "invalid route: routes with delegate actions must use a prefix matcher")
}

func (s *testingSuite) TestRejectsInvalidVSTypo() {
	output, err := s.testInstallation.Actions.Kubectl().ApplyFileWithOutput(s.ctx, validation.InvalidVirtualServiceTypo, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().Error(err)
	println(output)

	// This is handled by validation schemas now
	// We support matching on number of options, in order to support our nightly tests,
	// which are run using our earliest and latest supported versions of Kubernetes
	s.Assert().Condition(func() (success bool) {
		return strings.Contains(
			// This is the error returned when running Kubernetes <1.25
			output, `ValidationError(VirtualService.spec): unknown field "virtualHoost" in io.solo.gateway.v1.VirtualService.spec`) ||
			// This is the error returned when running Kubernetes >= 1.25
			strings.Contains(output, `VirtualService in version "v1" cannot be handled as a VirtualService: strict decoding error: unknown field "spec.virtualHoost"`)

	}, "rejects invalid VirtualService with typo")
}
