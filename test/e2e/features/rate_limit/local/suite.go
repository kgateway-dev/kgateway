//go:build e2e

package local_rate_limit

import (
	"context"
	"net/http"

	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of basic routing / "happy path" tests
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	testCases := map[string]*base.TestCase{
		"TestLocalRateLimitForRoute": {
			Manifests: []string{httpRoutesManifest, routeLocalRateLimitManifest},
		},
		"TestLocalRateLimitForGateway": {
			Manifests: []string{httpRoutesManifest, gwLocalRateLimitManifest},
		},
		"TestLocalRateLimitForGatewayAndRoute": {
			Manifests: []string{httpRoutesManifest, gwLocalRateLimitManifest, routeLocalRateLimitManifest},
		},
		"TestLocalRateLimitDisabledForRoute": {
			Manifests: []string{httpRoutesManifest, gwLocalRateLimitManifest, disabledRouteLocalRateLimitManifest},
		},
		"TestLocalRateLimitForRouteUsingExtensionRef": {
			Manifests: []string{extensionRefManifest},
		},
	}
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, base.TestCase{}, testCases),
	}
}

// Test cases for local rate limit on a route (/path1)
func (s *testingSuite) TestLocalRateLimitForRoute() {
	// First request should be successful
	s.assertResponse("/path1")

	// Consecutive requests should be rate limited
	s.assertConsistentResponse("/path1", http.StatusTooManyRequests)

	// Second route shouldn't be rate limited
	s.assertConsistentResponse("/path2", http.StatusOK)
}

// Test cases for local rate limit on a gateway
func (s *testingSuite) TestLocalRateLimitForGateway() {
	// First request should be successful (to any route)
	s.assertResponse("/path1")

	// Consecutive requests should be rate limited
	s.assertConsistentResponse("/path1", http.StatusTooManyRequests)

	// Also verify that the second route is rate limited
	s.assertConsistentResponse("/path2", http.StatusTooManyRequests)
}

// Test cases for local rate limit on a gateway and route (/path1)
func (s *testingSuite) TestLocalRateLimitForGatewayAndRoute() {
	// First request should be successful (to any route)
	s.assertResponse("/path1")

	// Consecutive requests should be rate limited
	s.assertConsistentResponse("/path1", http.StatusTooManyRequests)

	// Also verify that the second route is rate limited
	s.assertConsistentResponse("/path2", http.StatusTooManyRequests)

	// Verify that the rate limit is removed after a token has been added to the bucket (10s)
	// while GW rate limit is configured to add token every 300s. Therefore, the route
	// rate limit configuration takes precedence.
	s.assertEventualResponse("/path1", http.StatusOK)
}

// Test cases for local rate limit on a gateway and route (/path1) with disabled
// local rate limit
func (s *testingSuite) TestLocalRateLimitDisabledForRoute() {
	// First request should be successful (to any route)
	s.assertResponse("/path1")

	// Consecutive requests should not be rate limited (disaled for this path)
	s.assertConsistentResponse("/path1", http.StatusOK)

	// Also verify that the second route is rate limited
	s.assertConsistentResponse("/path2", http.StatusTooManyRequests)
}

// Test cases for local rate limit on a route (/path2) using extensionref in the HTTPRoute
func (s *testingSuite) TestLocalRateLimitForRouteUsingExtensionRef() {
	// First request should be successful
	s.assertResponse("/path2")

	// Consecutive requests should be rate limited
	s.assertConsistentResponse("/path2", http.StatusTooManyRequests)

	// Second route shouldn't be rate limited
	s.assertConsistentResponse("/path1", http.StatusOK)
}

func (s *testingSuite) assertResponse(path string) {
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		},
		curl.WithPath(path),
		curl.WithHostHeader("example.com"),
		curl.WithPort(80),
	)
}

func (s *testingSuite) assertConsistentResponse(path string, expectedStatus int) {
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: expectedStatus,
		},
		curl.WithPath(path),
		curl.WithHostHeader("example.com"),
		curl.WithPort(80),
	)
}

func (s *testingSuite) assertEventualResponse(path string, expectedStatus int) {
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: expectedStatus,
		},
		curl.WithPath(path),
		curl.WithHostHeader("example.com"),
		curl.WithPort(80),
	)
}
