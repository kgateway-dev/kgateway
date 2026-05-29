//go:build e2e

package rbac

import (
	"context"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
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

// TestRBACHeaderAuthorization tests header based rbac with RBAC applied at the route level
func (s *testingSuite) TestRBACHeaderAuthorizationWithRouteLevelRBAC() {
	// Verify HTTPRoute is accepted before running the test
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "httpbin-route", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	// missing header, no rbac on route, should succeed
	s.T().Log("The /status route has no rbac")
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
	)

	// missing header, should fail
	s.T().Log("The /get route has an rbac policy applied at the route level, should fail when the header is missing")
	common.BaseGateway.Send(
		s.T(),
		expectRBACDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
	)

	// has header, should succeed
	s.T().Log("The /get route has an rbac policy applied at the route level, should succeed when the header is present")
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
		curl.WithHeader("x-my-header", "cool-beans"),
	)
}

// TestRBACDenyActionAllowsNonMatchingTraffic verifies that a Deny RBAC policy only denies
// traffic that matches the CEL expression, and allows everything else through.
// This is a regression test for a bug where on_no_match was hardcoded to DENY, causing
// all traffic to be denied regardless of whether it matched the policy.
func (s *testingSuite) TestRBACDenyActionAllowsNonMatchingTraffic() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "httpbin-route", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	// No x-deny-me header: does not match the Deny expression, so traffic should be allowed.
	s.T().Log("Request without x-deny-me header should be allowed (non-matching Deny policy)")
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
	)

	// x-deny-me: true: matches the Deny expression, so traffic should be denied.
	s.T().Log("Request with x-deny-me: true should be denied (matching Deny policy)")
	common.BaseGateway.Send(
		s.T(),
		expectRBACDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
		curl.WithHeader("x-deny-me", "true"),
	)
}

// TestRBACHeaderAuthorization tests header based rbac
func (s *testingSuite) TestRBACHeaderAuthorization() {
	// Verify HTTPRoute is accepted before running the test
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "httpbin-route", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	// rbac applied to all routes, missing header, should fail
	s.T().Log("The /status route has rbac applied to all routes, should fail")
	common.BaseGateway.Send(
		s.T(),
		expectRBACDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
	)

	// has header, should succeed
	s.T().Log("The /status route has rbac applied to all routes, should succeed when the header is present")
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("x-my-header", "cool-beans"),
	)

	// missing header, should fail
	s.T().Log("The /get route has an rbac policy applied at the route level, should fail when the header is missing")
	common.BaseGateway.Send(
		s.T(),
		expectRBACDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
	)

	// has header, should succeed
	s.T().Log("The /get route has an rbac policy applied at the route level, should succeed when the header is present")
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
		curl.WithHeader("x-my-header", "cool-beans"),
	)
}
