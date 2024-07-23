package validation_strict

import (
	"context"
	"fmt"
	"os"
	"strings"

	gloo_defaults "github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	testdefaults "github.com/solo-io/gloo/test/kubernetes/e2e/defaults"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/validation"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// TestInvalidUpstreamMissingPort tests behaviors when Gloo rejects an invalid upstream with a missing port
func (s *testingSuite) TestInvalidUpstreamMissingPort() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, testdefaults.NginxPodManifest)
		s.Assert().NoError(err, "can delete "+testdefaults.NginxPodManifest)

		err = s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, validation.ExampleVS, "-n", s.testInstallation.Metadata.InstallNamespace)
		s.Assert().NoError(err, "can delete "+validation.ExampleVS)

		err = s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, validation.ExampleUpstream, "-n", s.testInstallation.Metadata.InstallNamespace)
		s.Assert().NoError(err, "can delete "+validation.ExampleUpstream)
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.NginxPodManifest)
	s.Assert().NoError(err)
	// Check that test resources are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.NginxPod.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=nginx",
	})

	// Upstream is only rejected when the upstream plugin is run when a valid cluster is present
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.ExampleUpstream, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err, "can apply valid upstream")
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.UpstreamClient().Read(s.testInstallation.Metadata.InstallNamespace, validation.ExampleUpstreamName, clients.ReadOpts{Ctx: s.ctx})
		},
		core.Status_Accepted,
		gloo_defaults.GlooReporter,
	)
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.ExampleVS, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err, "can apply valid virtual service")
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.VirtualServiceClient().Read(s.testInstallation.Metadata.InstallNamespace, validation.ExampleVsName, clients.ReadOpts{Ctx: s.ctx})
		},
		core.Status_Accepted,
		gloo_defaults.GlooReporter,
	)

	output, err := s.testInstallation.Actions.Kubectl().ApplyFileWithOutput(s.ctx, validation.InvalidUpstreamNoPort, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().Contains(output, fmt.Sprintf(`admission webhook "gloo.%s.svc" denied the request`, s.testInstallation.Metadata.InstallNamespace))
	s.Assert().Contains(output, fmt.Sprintf(`Validating *v1.Upstream failed: validating *v1.Upstream name:"invalid-us" namespace:"%s": 1 error occurred`, s.testInstallation.Metadata.InstallNamespace))
	s.Assert().Contains(output, "port cannot be empty for host")
}

/*
TestVirtualServiceWithSecretDeletion tests behaviors when Gloo rejects a VirtualService with a secret that is deleted

To create the private key and certificate to use:

	openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
	   -keyout tls.key -out tls.crt -subj "/CN=*"

To create the Kubernetes secrets to hold this cert:

	kubectl create secret tls upstream-tls --key tls.key \
	   --cert tls.crt --namespace gloo-system
*/
func (s *testingSuite) TestVirtualServiceWithSecretDeletion() {
	// VS with secret should be accepted, need to substitute the secret ns
	secretVS, err := os.ReadFile(validation.SecretVSTemplate)
	s.Assert().NoError(err)
	// Replace environment variables placeholders with their values
	substitutedSecretVS := os.ExpandEnv(string(secretVS))

	s.T().Cleanup(func() {
		// Can delete resources in correct order
		err := s.testInstallation.Actions.Kubectl().Delete(s.ctx, []byte(substitutedSecretVS), "-n", s.testInstallation.Metadata.InstallNamespace)
		s.Assert().NoError(err, "can delete virtual service with secret")

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation.ExampleUpstream, "-n", s.testInstallation.Metadata.InstallNamespace)
		s.Assert().NoError(err, "can delete "+validation.ExampleUpstream)

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation.Secret, "-n", s.testInstallation.Metadata.InstallNamespace)
		s.Assert().NoError(err, "can delete "+validation.Secret)

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, testdefaults.NginxPodManifest)
		s.Assert().NoError(err, "can delete "+testdefaults.NginxPodManifest)
	})

	// apply example app
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.NginxPodManifest)
	s.Assert().NoError(err)
	// Check that test resources are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.NginxPod.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=nginx",
	})

	// Secrets should be accepted
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.Secret, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.UnusedSecret, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)

	// Upstream should be accepted
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.ExampleUpstream, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.UpstreamClient().Read(s.testInstallation.Metadata.InstallNamespace, validation.ExampleUpstreamName, clients.ReadOpts{Ctx: s.ctx})
		},
		core.Status_Accepted,
		gloo_defaults.GlooReporter,
	)
	// Apply VS with secret after Upstream and Secret exist
	err = s.testInstallation.Actions.Kubectl().Apply(s.ctx, []byte(substitutedSecretVS))
	s.Assert().NoError(err)
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.VirtualServiceClient().Read(s.testInstallation.Metadata.InstallNamespace, validation.ExampleVsName, clients.ReadOpts{Ctx: s.ctx})
		},
		core.Status_Accepted,
		gloo_defaults.GlooReporter,
	)

	// failing to delete a secret that is in use
	output, err := s.testInstallation.Actions.Kubectl().DeleteFileWithOutput(s.ctx, validation.Secret, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().Error(err)
	s.Assert().Contains(output, fmt.Sprintf(`admission webhook "gloo.%s.svc" denied the request`, s.testInstallation.Metadata.InstallNamespace))
	s.Assert().Contains(output, fmt.Sprintf("failed validating the deletion of resource"))
	s.Assert().Contains(output, fmt.Sprintf("SSL secret not found: list did not find secret %s.tls-secret", s.testInstallation.Metadata.InstallNamespace))

	// deleting a secret that is not in use works
	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation.UnusedSecret, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)
}

// TestRejectsInvalidGatewayResources tests behaviors when Gloo rejects invalid Edge Gateway resources
func (s *testingSuite) TestRejectsInvalidGatewayResources() {
	output, err := s.testInstallation.Actions.Kubectl().ApplyFileWithOutput(s.ctx, validation.InvalidGateway, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().Error(err)
	s.Assert().Contains(output, fmt.Sprintf(`admission webhook "gloo.%s.svc" denied the request`, s.testInstallation.Metadata.InstallNamespace))
	s.Assert().Contains(output, fmt.Sprintf(`Validating *v1.Gateway failed: validating *v1.Gateway name:"gateway-without-type" namespace:"%s": 1 error occurred`, s.testInstallation.Metadata.InstallNamespace))
	s.Assert().Contains(output, "invalid gateway: gateway must contain gatewayType")
}

// TestRejectsInvalidRatelimitConfigResources tests behaviors when Gloo rejects invalid RateLimitConfig resources due to missing enterprise features
func (s *testingSuite) TestRejectsInvalidRatelimitConfigResources() {
	output, _ := s.testInstallation.Actions.Kubectl().ApplyFileWithOutput(s.ctx, validation.InvalidRLC, "-n", s.testInstallation.Metadata.InstallNamespace)
	// We don't expect an error exit code here because this is a warning
	s.Assert().Contains(output, fmt.Sprintf(`admission webhook "gloo.%s.svc" denied the request`, s.testInstallation.Metadata.InstallNamespace))
	s.Assert().Contains(output, fmt.Sprintf(`Validating *v1alpha1.RateLimitConfig failed: validating *v1alpha1.RateLimitConfig name:"rlc" namespace:"%s": 1 error occurred`, s.testInstallation.Metadata.InstallNamespace))
	s.Assert().Contains(output, "The Gloo Advanced Rate limit API feature 'RateLimitConfig' is enterprise-only, please upgrade or use the Envoy rate-limit API instead")
}

// TestRejectsInvalidVSMethodMatcher tests behaviors when Gloo rejects invalid VirtualService resources due to incorrect matchers
func (s *testingSuite) TestRejectsInvalidVSMethodMatcher() {
	output, err := s.testInstallation.Actions.Kubectl().ApplyFileWithOutput(s.ctx, validation.InvalidVirtualServiceMatcher, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().Error(err)
	s.Assert().Contains(output, fmt.Sprintf(`admission webhook "gloo.%s.svc" denied the request`, s.testInstallation.Metadata.InstallNamespace))
	s.Assert().Contains(output, fmt.Sprintf(`Validating *v1.VirtualService failed: validating *v1.VirtualService name:"method-matcher" namespace:"%s": 1 error occurred`, s.testInstallation.Metadata.InstallNamespace))
	s.Assert().Contains(output, "invalid route: routes with delegate actions must use a prefix matcher")
}

// TestRejectsInvalidVSMissingUpstream tests behaviors when Gloo rejects invalid VirtualService resources due to missing upstream
func (s *testingSuite) TestRejectsInvalidVSMissingUpstream() {
	output, err := s.testInstallation.Actions.Kubectl().ApplyFileWithOutput(s.ctx, validation.InvalidVirtualMissingUpstream, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().Error(err)
	s.Assert().Contains(output, fmt.Sprintf(`admission webhook "gloo.%s.svc" denied the request`, s.testInstallation.Metadata.InstallNamespace))
	s.Assert().Contains(output, fmt.Sprintf(`Validating *v1.VirtualService failed: validating *v1.VirtualService name:"no-upstream-vs" namespace:"%s": 1 error occurred`, s.testInstallation.Metadata.InstallNamespace))
	s.Assert().Contains(output, fmt.Sprintf(`Route Warning: InvalidDestinationWarning. Reason: *v1.Upstream { %s.does-not-exist } not found`, s.testInstallation.Metadata.InstallNamespace))
}

// TestRejectsInvalidVSTypo tests behaviors when Gloo rejects invalid VirtualService resources due to typos
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
