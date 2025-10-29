//go:build e2e

package httproute

import (
	"context"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// testingSuite is the entire Suite of tests for testing K8s Service-specific features/fixes
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of kgateway
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) TestConfigureHTTPRouteBackingDestinationsWithService() {
	testutils.Cleanup(s.T(), func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, routeWithServiceManifest)
		s.NoError(err, "can delete manifest")
		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, serviceManifest)
		s.NoError(err, "can delete manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
	})

	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, routeWithServiceManifest)
	s.Assert().NoError(err, "can apply manifest")

	// apply the service manifest separately, after the route table is applied, to ensure it can be applied after the route table
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, serviceManifest)
	s.Assert().NoError(err, "can apply manifest")

	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, nginxMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.WellKnownAppLabel + "=nginx",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.WellKnownAppLabel + "=gw",
	})

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
	testutils.Cleanup(s.T(), func() {
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
	s.Assert().NoError(err, "can apply manifest")

	// apply the service manifest separately, after the route table is applied, to ensure it can be applied after the route table
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, serviceManifest)
	s.Assert().NoError(err, "can apply manifest")

	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, nginxMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.WellKnownAppLabel + "=nginx",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.WellKnownAppLabel + "=gw",
	})

	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedSvcResp)
}
