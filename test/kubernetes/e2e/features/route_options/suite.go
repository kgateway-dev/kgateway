package route_options

import (
	"context"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
)

// testingSuite is the entire Suite of tests for the "Route Options" feature
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

func (s *testingSuite) TestConfigureRouteOptionsWithTargetRef() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, targetRefManifest)
		s.NoError(err, "can delete manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, targetRefManifest)
	s.NoError(err, "can apply targetRefManifest")

	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedFaultInjectionResp)

	// Check status is accepted on RouteOption
	s.testInstallation.Assertions.EventuallyResourceStatusMatchesState(
		s.ctx,
		func() (resources.InputResource, error) {
			return s.testInstallation.ResourceClients.RouteOptionClient().Read(routeOptionMeta.GetNamespace(), routeOptionMeta.GetName(), clients.ReadOpts{})
		},
		gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"State":      gomega.Equal(core.Status_Accepted),
			"ReportedBy": gomega.Equal("gloo-kube-gateway"), // TODO: this should likely be a constant
		}))
}

func (s *testingSuite) TestConfigureRouteOptionsWithFilterExtension() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, filterExtensioManifest)
		s.NoError(err, "can delete manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, filterExtensioManifest)
	s.NoError(err, "can apply targetRefManifest")

	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedFaultInjectionResp)

	// TODO(npolshak): Statuses are not supported for filter extensions yet
}
