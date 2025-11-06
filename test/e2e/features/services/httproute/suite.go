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
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

// testingSuite is the entire Suite of tests for testing K8s Service-specific features/fixes
type testingSuite struct {
	*base.BaseTestingSuite
}

var (
	// No setup since we handle manifests manually in each test
	setup = base.TestCase{}

	testCases = map[string]*base.TestCase{
		"TestConfigureHTTPRouteBackingDestinationsWithService": {},
		// If the TCPRoute CRD is not installed, TestConfigureHTTPRouteBackingDestinationsWithService implicitly tests that HTTPRoute services still work without the CRD
		"TestConfigureHTTPRouteBackingDestinationsWithServiceAndWithoutTCPRoute": {
			MinGwApiVersion: base.GwApiRequireTcpRoutes,
		},
	}
)

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

func (s *testingSuite) TestConfigureHTTPRouteBackingDestinationsWithService() {
	testutils.Cleanup(s.T(), func() {
		err := s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, routeWithServiceManifest)
		s.NoError(err, "can delete manifest")
		err = s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, serviceManifest)
		s.NoError(err, "can delete manifest")
		s.TestInstallation.Assertions.EventuallyObjectsNotExist(s.Ctx, proxyService, proxyDeployment)
	})

	err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, routeWithServiceManifest)
	s.Assert().NoError(err, "can apply manifest")

	// apply the service manifest separately, after the route table is applied, to ensure it can be applied after the route table
	err = s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, serviceManifest)
	s.Assert().NoError(err, "can apply manifest")

	s.TestInstallation.Assertions.EventuallyObjectsExist(s.Ctx, proxyService, proxyDeployment)
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx, nginxMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.WellKnownAppLabel + "=nginx",
	})
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.WellKnownAppLabel + "=gw",
	})

	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedSvcResp)
}

func (s *testingSuite) TestConfigureHTTPRouteBackingDestinationsWithServiceAndWithoutTCPRoute() {
	// Get the current TCPRoute CRD before deleting it. We don't know until runtime which version is installed.
	tcpRouteCrdYaml, _, err := s.TestInstallation.Actions.Kubectl().Execute(s.Ctx, "get", "crd", "tcproutes.gateway.networking.k8s.io", "-o", "yaml")
	s.Assert().NoError(err, "can get TCPRoute CRD")

	testutils.Cleanup(s.T(), func() {
		err := s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, routeWithServiceManifest)
		s.Assert().NoError(err, "can delete manifest")
		err = s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, serviceManifest)
		s.Assert().NoError(err, "can delete manifest")
		s.TestInstallation.Assertions.EventuallyObjectsNotExist(s.Ctx, proxyService, proxyDeployment)

		// Restore the TCPRoute CRD using the saved content
		err = s.TestInstallation.Actions.Kubectl().Apply(s.Ctx, []byte(tcpRouteCrdYaml))
		s.NoError(err, "can apply TCPRoute CRD")
		s.TestInstallation.Assertions.EventuallyObjectsExist(s.Ctx, &wellknown.TCPRouteCRD)
	})

	// Remove the TCPRoute CRD to assert HTTPRoute services still work.
	err = s.TestInstallation.Actions.Kubectl().DeleteFile(s.Ctx, tcpRouteCrdManifest)
	s.Assert().NoError(err, "can delete manifest")

	err = s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, routeWithServiceManifest)
	s.Assert().NoError(err, "can apply manifest")

	// apply the service manifest separately, after the route table is applied, to ensure it can be applied after the route table
	err = s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, serviceManifest)
	s.Assert().NoError(err, "can apply manifest")

	s.TestInstallation.Assertions.EventuallyObjectsExist(s.Ctx, proxyService, proxyDeployment)
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx, nginxMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.WellKnownAppLabel + "=nginx",
	})
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.WellKnownAppLabel + "=gw",
	})

	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
		},
		expectedSvcResp)
}
