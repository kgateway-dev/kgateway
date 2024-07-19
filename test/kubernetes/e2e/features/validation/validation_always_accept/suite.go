package validation_always_accept

import (
	"context"
	"net/http"

	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kubernetes/e2e/features/validation/validation_types"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
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

// TestPersistInvalidVirtualService tests behaviors when Gloo allows invalid VirtualServices to be persisted
func (s *testingSuite) TestPersistInvalidVirtualService() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation_types.ValidVS)
		s.NoError(err, "can delete validVS")
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation_types.ExampleUpstream)
		s.Assert().NoError(err, "can delete Upstreams manifest")
	})

	// First apply valid VirtualService and Upstream
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation_types.ValidVS)
	s.Assert().NoError(err, "can apply gloo.solo.io ValidVS manifest")
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation_types.ExampleUpstream)
	s.Assert().NoError(err, "can apply gloo.solo.io Upstreams manifest")

	// Check valid works as expected
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		validation_types.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			// The host header must match the domain in the VirtualService
			curl.WithHostHeader("valid1.com"),
			curl.WithPort(80),
		},
		validation_types.ExpectedUpstreamResp)

	// apply invalid VS
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation_types.InvalidVS)
	s.Assert().NoError(err, "can apply gloo.solo.io invalidVS manifest")

	helpers.EventuallyResourceRejected(func() (resources.InputResource, error) {
		return s.testInstallation.ResourceClients.VirtualServiceClient().Read(s.testInstallation.Metadata.InstallNamespace, validation_types.InvalidVsName, clients.ReadOpts{
			Ctx: s.ctx,
		})
	})
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		validation_types.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			// The host header must match the domain in the VirtualService
			curl.WithHostHeader("invalid.com"),
			curl.WithPort(80),
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusNotFound})

	// make the invalid vs valid and the valid vs invalid
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation_types.SwitchVS)
	s.Assert().NoError(err, "can apply gloo.solo.io switchVS manifest")

	// the fixed virtual service should also work
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		validation_types.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			// The host header must match the domain in the VirtualService
			curl.WithHostHeader("valid1.com"),
			curl.WithPort(80),
		},
		validation_types.ExpectedUpstreamResp)
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		validation_types.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			// The host header must match the domain in the VirtualService
			curl.WithHostHeader("all-good-in-the-hood.com"),
			curl.WithPort(80),
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusNotFound})
}

// TestMissingUpstream tests behaviors when Gloo allows invalid VirtualServices to be persisted
func (s *testingSuite) TestMissingUpstream() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation_types.ValidVS)
		s.Assert().NoError(err, "can delete gloo.solo.io validVS manifest")
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, validation_types.ExampleUpstream)
		s.Assert().NoError(err, "can delete gloo.solo.io Upstreams manifest")
	})

	// First apply valid VirtualService, and no Upstream
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation_types.ValidVS)
	s.Assert().NoError(err, "can apply gloo.solo.io validVS manifest")

	// missing Upstream ref in VirtualService
	helpers.EventuallyResourceWarning(func() (resources.InputResource, error) {
		return s.testInstallation.ResourceClients.VirtualServiceClient().Read(s.testInstallation.Metadata.InstallNamespace, validation_types.ValidVsName, clients.ReadOpts{})
	})

	// Apply upstream
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, validation_types.ExampleUpstream)
	s.Assert().NoError(err, "can apply gloo.solo.io Upstreams manifest")

	// Check valid works as expected
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		validation_types.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.testInstallation.Metadata.InstallNamespace})),
			// The host header must match the domain in the VirtualService
			curl.WithHostHeader("valid1.com"),
			curl.WithPort(80),
		},
		validation_types.ExpectedUpstreamResp)

	helpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
		return s.testInstallation.ResourceClients.VirtualServiceClient().Read(s.testInstallation.Metadata.InstallNamespace, validation_types.ValidVsName, clients.ReadOpts{})
	})
}
