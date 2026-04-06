//go:build e2e

package faultinjection

import (
	"context"
	"net/http"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of tests for testing fault injection policies
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// TestFaultInjectionAbortOnRoute verifies that a TrafficPolicy with 100% abort at HTTP 503
// returns the configured status code on the targeted route.
func (s *testingSuite) TestFaultInjectionAbortOnRoute() {
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusServiceUnavailable,
		},
		curl.WithPort(80),
		curl.WithPath("/fault/status/200"),
		curl.WithHostHeader("example.com"),
	)
}

// TestFaultInjectionAbortDoesNotAffectOtherRoutes verifies that a fault injection policy
// attached to one route does not affect other routes without the policy.
func (s *testingSuite) TestFaultInjectionAbortDoesNotAffectOtherRoutes() {
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		},
		curl.WithPort(80),
		curl.WithPath("/no-fault/status/200"),
		curl.WithHostHeader("example.com"),
	)
}

// TestFaultInjectionDelayOnRoute verifies that a TrafficPolicy with delay configured
// still returns a successful response (delay is applied but request is not aborted).
// It also verifies the response takes at least the configured delay duration.
func (s *testingSuite) TestFaultInjectionDelayOnRoute() {
	start := time.Now()

	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		},
		curl.WithPort(80),
		curl.WithPath("/fault/status/200"),
		curl.WithHostHeader("example.com"),
	)

	elapsed := time.Since(start)
	s.GreaterOrEqual(elapsed, 100*time.Millisecond, "expected response to take at least 100ms due to fault delay injection")
}

// TestFaultInjectionAbortOnGateway verifies that a TrafficPolicy with abort attached
// to a Gateway returns the configured status code on a route through that gateway.
func (s *testingSuite) TestFaultInjectionAbortOnGateway() {
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusServiceUnavailable,
		},
		curl.WithPort(80),
		curl.WithPath("/fault/status/200"),
		curl.WithHostHeader("example.com"),
	)
}

// TestFaultInjectionAbortOnGatewayAffectsAllRoutes verifies that a fault injection
// policy attached to a Gateway affects all routes on that gateway, not just one.
func (s *testingSuite) TestFaultInjectionAbortOnGatewayAffectsAllRoutes() {
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusServiceUnavailable,
		},
		curl.WithPort(80),
		curl.WithPath("/no-fault/status/200"),
		curl.WithHostHeader("example.com"),
	)
}

// TestFaultInjectionDisableOverridesGatewayPolicy verifies that a route-level
// TrafficPolicy with faultInjection.disable overrides a gateway-level fault
// injection policy, allowing the route to respond normally.
func (s *testingSuite) TestFaultInjectionDisableOverridesGatewayPolicy() {
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		},
		curl.WithPort(80),
		curl.WithPath("/no-fault/status/200"),
		curl.WithHostHeader("example.com"),
	)
}

// TestFaultInjectionDisableDoesNotAffectOtherRoutes verifies that a route-level
// disable does not affect other routes that should still have the gateway-level
// fault injection applied.
func (s *testingSuite) TestFaultInjectionDisableDoesNotAffectOtherRoutes() {
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusServiceUnavailable,
		},
		curl.WithPort(80),
		curl.WithPath("/fault/status/200"),
		curl.WithHostHeader("example.com"),
	)
}
