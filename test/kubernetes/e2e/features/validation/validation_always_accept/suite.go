package validation_always_accept

import (
	"context"
	"net/http"
	"os"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloo_defaults "github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/helpers"
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

// testingSuite is the entire Suite of tests for the webhook validation alwaysAccept=true feature
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

func (s *testingSuite) TestRejectsInvalidVSMethodMatcher() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.InvalidVirtualServiceMatcher, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.VirtualServiceClient().Read(s.testInstallation.Metadata.InstallNamespace, "method-matcher", clients.ReadOpts{Ctx: s.ctx})
		},
		core.Status_Rejected,
		gloo_defaults.GlooReporter,
	)
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesRejectedReasons(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.VirtualServiceClient().Read(s.testInstallation.Metadata.InstallNamespace, "method-matcher", clients.ReadOpts{Ctx: s.ctx})
		},
		[]string{"invalid route: routes with delegate actions must use a prefix matcher"},
		gloo_defaults.GlooReporter,
	)
}

func (s *testingSuite) TestAcceptInvalidRatelimitConfigResources() {
	if s.testInstallation.Metadata.IsEnterprise {
		s.T().Skip("RateLimitConfig is enterprise-only, skipping test when running enterprise helm chart")
	}
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.InvalidRLC, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)
	// We don't expect an error exit code here because alwaysAccept=true
	helpers.EventuallyResourceRejected(func() (resources.InputResource, error) {
		return s.testInstallation.ResourceClients.RateLimitConfigClient().Read(s.testInstallation.Metadata.InstallNamespace, "rlc", clients.ReadOpts{Ctx: s.ctx})
	})

	helpers.EventuallyResourceStatusHasReason(1,
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.RateLimitConfigClient().Read(s.testInstallation.Metadata.InstallNamespace, "rlc", clients.ReadOpts{Ctx: s.ctx})
		},
		"The Gloo Advanced Rate limit API feature 'RateLimitConfig' is enterprise-only, please upgrade or use the Envoy rate-limit API instead",
	)
}

func (s *testingSuite) TestAcceptsInvalidGatewayResources() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.InvalidGateway, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)

	// We don't expect an error exit code here because alwaysAccept=true
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.GatewayClient().Read(s.testInstallation.Metadata.InstallNamespace, "gateway-without-type", clients.ReadOpts{Ctx: s.ctx})
		},
		core.Status_Rejected,
		gloo_defaults.GlooReporter,
	)
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesRejectedReasons(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.GatewayClient().Read(s.testInstallation.Metadata.InstallNamespace, "gateway-without-type", clients.ReadOpts{Ctx: s.ctx})
		},
		[]string{"invalid gateway: gateway must contain gatewayType"},
		gloo_defaults.GlooReporter,
	)
}

/*
TestVirtualServiceWithSecretDeletion tests behaviors when Gloo accepts a VirtualService with a secret and the secret is deleted

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
		err := s.testInstallation.Actions.Kubectl().Delete(s.ctx, []byte(substitutedSecretVS))
		s.Assert().NoError(err, "can delete virtual service with secret")

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation.ExampleUpstream, "-n", s.testInstallation.Metadata.InstallNamespace)
		s.Assert().NoError(err, "can delete "+validation.ExampleUpstream)

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

	// can delete a secret that is in use without error
	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation.Secret, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)

	// deleting a secret that is not in use still works
	err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation.UnusedSecret, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err)

	/*
		TODO(npolshak): Consistently update subresource statuses: https://github.com/solo-io/solo-projects/issues/6633
		Note: the VirtualService subresource status will have rejection message when secret is deleted, but the status update
		is flakey and may not get triggered immediately. This example of the expected status update:

			status:
			  statuses:
			    validation-always-accept-test:
			      reportedBy: gloo
			      state: Accepted
			      subresourceStatuses:
			        '*v1.Proxy.gateway-proxy_validation-always-accept-test':
			          reason: "1 error occurred:\n\t* Listener Error: SSLConfigError. Reason:
			            SSL secret not found: list did not find secret validation-always-accept-test.tls-secret\n\n"
			          reportedBy: gloo
			          state: Rejected
	*/
}

// TestPersistInvalidVirtualService tests behaviors when Gloo allows invalid VirtualServices to be persisted
func (s *testingSuite) TestPersistInvalidVirtualService() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation.ExampleUpstream, "-n", s.testInstallation.Metadata.InstallNamespace)
		s.Assert().NoError(err, "can delete "+validation.ExampleUpstream)

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation.ValidVS, "-n", s.testInstallation.Metadata.InstallNamespace)
		s.NoError(err, "can delete "+validation.ValidVS)

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, testdefaults.NginxPodManifest)
		s.Assert().NoError(err)

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, testdefaults.CurlPodManifest)
		s.Assert().NoError(err)
	})

	// Apply setup
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.NginxPodManifest)
	s.Assert().NoError(err)
	// Check that test resources are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.NginxPod.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=nginx",
	})

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.CurlPodManifest)
	s.Assert().NoError(err)
	// Check that test resources are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=curl",
	})

	// First apply Upstream
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.ExampleUpstream, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err, "can apply "+validation.ExampleUpstream)
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.UpstreamClient().Read(s.testInstallation.Metadata.InstallNamespace, validation.ExampleUpstreamName, clients.ReadOpts{Ctx: s.ctx})
		},
		core.Status_Accepted,
		gloo_defaults.GlooReporter,
	)

	// Then apply VirtualService
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.ValidVS, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err, "can apply "+validation.ValidVS)
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.VirtualServiceClient().Read(s.testInstallation.Metadata.InstallNamespace, validation.ValidVsName, clients.ReadOpts{Ctx: s.ctx})
		},
		core.Status_Accepted,
		gloo_defaults.GlooReporter,
	)

	// Check valid works as expected
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			// The host header must match the domain in the VirtualService
			curl.WithHostHeader("valid1.com"),
			curl.WithPort(80),
		},
		validation.ExpectedUpstreamResp)

	// apply invalid VS
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.InvalidVS, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err, "can apply "+validation.InvalidVS)

	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.VirtualServiceClient().Read(s.testInstallation.Metadata.InstallNamespace, validation.InvalidVsName, clients.ReadOpts{Ctx: s.ctx})
		},
		core.Status_Warning,
		gloo_defaults.GlooReporter,
	)
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			// The host header must match the domain in the VirtualService
			curl.WithHostHeader("invalid.com"),
			curl.WithPort(80),
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusNotFound})

	// make the invalid vs valid and the valid vs invalid
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation.SwitchVS, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err, "can apply "+validation.SwitchVS)
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.VirtualServiceClient().Read(s.testInstallation.Metadata.InstallNamespace, validation.ValidVsName, clients.ReadOpts{Ctx: s.ctx})
		},
		core.Status_Warning,
		gloo_defaults.GlooReporter,
	)
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.VirtualServiceClient().Read(s.testInstallation.Metadata.InstallNamespace, validation.InvalidVsName, clients.ReadOpts{Ctx: s.ctx})
		},
		core.Status_Accepted,
		gloo_defaults.GlooReporter,
	)

	// the fixed virtual service should also work
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			// The host header must match the domain in the VirtualService
			curl.WithHostHeader("valid1.com"),
			curl.WithPort(80),
		},
		validation.ExpectedUpstreamResp)
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			// The host header must match the domain in the VirtualService
			curl.WithHostHeader("all-good-in-the-hood.com"),
			curl.WithPort(80),
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusNotFound})
}
