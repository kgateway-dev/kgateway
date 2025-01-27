package httproute

import (
	"context"

	"github.com/stretchr/testify/suite"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/defaults"
)

// testingSuite is the entire Suite of tests for testing K8s Service-specific features/fixes
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

func (s *testingSuite) TestConfigureHTTPRouteBackingDestinationsWithService() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, routeWithServiceManifest)
		s.NoError(err, "can delete manifest")
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, serviceManifest)
		s.NoError(err, "can delete manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, routeWithServiceManifest)
	s.Assert().NoError(err, "can apply gloo.solo.io Route manifest")

	// apply the service manifest separately, after the route table is applied, to ensure it can be applied after the route table
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, serviceManifest)
	s.Assert().NoError(err, "can apply gloo.solo.io Service manifest")

	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedSvcResp)
}

func (s *testingSuite) TestConfigureHTTPRouteBackingDestinationsWithServiceAndWithoutTCPRoute() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, routeWithServiceManifest)
		s.NoError(err, "can delete manifest")
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, serviceManifest)
		s.NoError(err, "can delete manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
		err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, tcpRouteCrdManifest)
		s.NoError(err, "can apply manifest")
		s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, &wellknown.TCPRouteCRD)
	})

	// Remove the TCPRoute CRD to assert HTTPRoute services still work.
	err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, tcpRouteCrdManifest)
	s.NoError(err, "can delete manifest")

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, routeWithServiceManifest)
	s.Assert().NoError(err, "can apply gloo.solo.io Route manifest")

	// apply the service manifest separately, after the route table is applied, to ensure it can be applied after the route table
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, serviceManifest)
	s.Assert().NoError(err, "can apply gloo.solo.io Service manifest")

	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedSvcResp)
}
