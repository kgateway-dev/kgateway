package rbac

import (
	"context"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of tests for rbac functionality
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// TestRBACHeaderAuthorization tests header based rbac
func (s *testingSuite) TestRBACHeaderAuthorization() {
	// Verify HTTPRoute is accepted before running the test
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(s.Ctx, "httpbin-route", "default", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	// TODO(npolshak): re-enable once sectionName is supported for TrafficPolicy in agentgateway once https://github.com/agentgateway/agentgateway/pull/323 is pulled in
	//statusReqCurlOpts := []curl.Option{
	//	curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
	//	curl.WithHostHeader("httpbin"),
	//	curl.WithPort(8080),
	//	curl.WithPath("/status/200"),
	//}
	//// missing header, no rbac on route, should succeed
	//s.T().Log("The /status route has no rbac")
	//s.TestInstallation.Assertions.AssertEventualCurlResponse(
	//	s.Ctx,
	//	testdefaults.CurlPodExecOpt,
	//	statusReqCurlOpts,
	//	expectStatus200Success,
	//)

	getReqCurlOpts := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
		curl.WithHostHeader("httpbin"),
		curl.WithPort(8080),
		curl.WithPath("/get"),
	}
	// missing header, should fail
	s.T().Log("The /get route has an rbac policy applied at the route level, should fail when the header is missing")
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getReqCurlOpts,
		expectRBACDenied,
	)
	// has header, should succeed
	s.T().Log("The /get route has an rbac policy applied at the route level, should succeed when the header is present")
	getWithHeaderCurlOpts := append(getReqCurlOpts, curl.WithHeader("x-my-header", "cool-beans"))
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getWithHeaderCurlOpts,
		expectStatus200Success,
	)
}
