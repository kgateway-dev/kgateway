//go:build e2e

package cors

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of tests for testing CORS policies
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// Test cors on specific route in a traffic policy
// The policy has the following allowOrigins:
// - https://notexample.com
// - https://a.b.*
// - https://*.edu
func (s *testingSuite) TestTrafficPolicyCorsForRoute() {
	testCases := []struct {
		name   string
		origin string
	}{
		{
			name:   "exact_match_origin",
			origin: "https://notexample.com",
		},
		{
			name:   "prefix_match_origin",
			origin: "https://a.b.c.d",
		},
		{
			name:   "regex_match_origin",
			origin: "https://test.cors.edu",
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			requestHeaders := map[string]string{
				"Origin":                        tc.origin,
				"Access-Control-Request-Method": "GET",
			}

			expectedHeaders := map[string]any{
				"Access-Control-Allow-Origin":  tc.origin,
				"Access-Control-Allow-Methods": "GET, POST, DELETE",
				"Access-Control-Allow-Headers": "x-custom-header",
			}

			// Verify that the route with cors is responding to the OPTIONS request with the expected cors headers
			s.assertResponse("/path1", requestHeaders, expectedHeaders, []string{})

			// Verify that the route without cors is not affected by the cors traffic policy (i.e. no cors headers are returned)
			s.assertResponse("/path2", requestHeaders, nil, []string{
				"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers",
			})
		})
	}

	// Negative test cases - origins that should NOT match the patterns
	negativeTestCases := []struct {
		name   string
		origin string
	}{
		{
			name:   "wildcard_subdomain_should_not_match_different_domain",
			origin: "https://notedu.com",
		},
		{
			name:   "wildcard_subdomain_should_not_match_different_tld",
			origin: "https://api.example.org",
		},
		{
			name:   "wildcard_subdomain_should_not_match_without_subdomain",
			origin: "https://edu",
		},
		{
			name:   "prefix_match_should_not_match_different_scheme",
			origin: "http://a.b.c.d",
		},
		{
			name:   "exact_match_should_not_match_similar_domain",
			origin: "https://notexample.org",
		},
		{
			name:   "exact_match_should_not_match_with_subdomain",
			origin: "https://api.notexample.com",
		},
		{
			name:   "prefix_match_should_not_match_invalid_url",
			origin: "https:/a.b",
		},
	}

	for _, tc := range negativeTestCases {
		s.T().Run("negative_"+tc.name, func(t *testing.T) {
			requestHeaders := map[string]string{
				"Origin":                        tc.origin,
				"Access-Control-Request-Method": "GET",
			}

			// For negative cases, we expect no CORS headers to be returned
			// since the origin doesn't match any of the allowed patterns
			s.assertResponse("/path1", requestHeaders, nil, []string{
				"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers",
			})

			// Verify that the route without cors is also not affected
			s.assertResponse("/path2", requestHeaders, nil, []string{
				"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers",
			})
		})
	}
}

// Test cors at the gateway level which configures cors policy in the virtual host and therefore affects all routes
func (s *testingSuite) TestTrafficPolicyCorsAtGatewayLevel() {
	requestHeaders := map[string]string{
		"Origin":                        "https://notexample.com",
		"Access-Control-Request-Method": "GET",
	}

	expectedHeaders := map[string]any{
		"Access-Control-Allow-Origin":  "https://notexample.com",
		"Access-Control-Allow-Methods": "GET, POST",
		"Access-Control-Allow-Headers": "Content-Type, Authorization",
	}

	s.assertResponse("/path1", requestHeaders, expectedHeaders, []string{})
	s.assertResponse("/path2", requestHeaders, expectedHeaders, []string{})
}

// Test different cors policies at the route level override the gateway level cors policy
func (s *testingSuite) TestTrafficPolicyRouteCorsOverrideGwCors() {
	requestHeaders := map[string]string{
		"Origin":                        "https://notexample.com",
		"Access-Control-Request-Method": "GET",
	}

	expectedHeadersPath1 := map[string]any{
		"Access-Control-Allow-Origin":  "https://notexample.com",
		"Access-Control-Allow-Methods": "GET, POST, DELETE",
		"Access-Control-Allow-Headers": "x-custom-header",
	}

	expectedHeadersPath2 := map[string]any{
		"Access-Control-Allow-Origin":  "https://notexample.com",
		"Access-Control-Allow-Methods": "GET, POST",
		"Access-Control-Allow-Headers": "Content-Type, Authorization",
	}

	s.assertResponse("/path1", requestHeaders, expectedHeadersPath1, []string{})
	s.assertResponse("/path2", requestHeaders, expectedHeadersPath2, []string{})

	// Assert that the route with CORS disabled does not return CORS headers
	s.assertResponse("/cors-disabled", requestHeaders, nil,
		[]string{"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers"})
}

// Test cors in route rules of a HTTPRoute
// The route has the following allowOrigins:
// - https://notexample.com
// - https://a.b.*
// - https://*.edu
func (s *testingSuite) TestHttpRouteCorsInRouteRules() {
	testCases := []struct {
		name   string
		origin string
	}{
		{
			name:   "exact_match_origin",
			origin: "https://notexample.com",
		},
		{
			name:   "prefix_match_origin",
			origin: "https://a.b.c.d",
		},
		{
			name:   "regex_match_origin",
			origin: "https://test.cors.edu",
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			requestHeaders := map[string]string{
				"Origin":                        tc.origin,
				"Access-Control-Request-Method": "GET",
			}

			expectedHeaders := map[string]any{
				"Access-Control-Allow-Origin":  tc.origin,
				"Access-Control-Allow-Methods": "GET",
				"Access-Control-Allow-Headers": "x-custom-header",
			}

			// Verify that the route with cors is responding to the OPTIONS request with the expected cors headers
			s.assertResponse("/path1", requestHeaders, expectedHeaders, []string{})

			// Verify that the route without cors is not affected by the cors in the HTTPRoute (i.e. no cors headers are returned)
			s.assertResponse("/path2", requestHeaders, nil, []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers"})
		})
	}

	// Negative test cases - origins that should NOT match the patterns
	negativeTestCases := []struct {
		name   string
		origin string
	}{
		{
			name:   "wildcard_subdomain_should_not_match_different_domain",
			origin: "https://notedu.com",
		},
		{
			name:   "wildcard_subdomain_should_not_match_different_tld",
			origin: "https://api.example.org",
		},
		{
			name:   "wildcard_subdomain_should_not_match_without_subdomain",
			origin: "https://edu",
		},
		{
			name:   "prefix_match_should_not_match_different_scheme",
			origin: "http://a.b.c.d",
		},
		{
			name:   "exact_match_should_not_match_similar_domain",
			origin: "https://notexample.org",
		},
		{
			name:   "exact_match_should_not_match_with_subdomain",
			origin: "https://api.notexample.com",
		},
		{
			name:   "prefix_match_should_not_match_invalid_url",
			origin: "https:/a.b",
		},
	}

	for _, tc := range negativeTestCases {
		s.T().Run("negative_"+tc.name, func(t *testing.T) {
			requestHeaders := map[string]string{
				"Origin":                        tc.origin,
				"Access-Control-Request-Method": "GET",
			}

			// For negative cases, we expect no CORS headers to be returned
			// since the origin doesn't match any of the allowed patterns
			s.assertResponse("/path1", requestHeaders, nil, []string{
				"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers",
			})

			// Verify that the route without cors is also not affected
			s.assertResponse("/path2", requestHeaders, nil, []string{
				"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers",
			})
		})
	}
}

// Test a combination of cors in route rules of a HTTPRoute and cors in a traffic policy
// applied at the gateway level.
// We expect the cors in the route rules to override the cors in the traffic policy for /path1 but
// for /path2 the cors in the traffic policy should be applied.
func (s *testingSuite) TestHttpRouteAndTrafficPolicyCors() {
	requestHeaders := map[string]string{
		"Origin":                        "https://notexample.com",
		"Access-Control-Request-Method": "GET",
	}

	// HTTPRoute for /path1 should have this cors response headers
	expectedHeadersPath1 := map[string]any{
		"Access-Control-Allow-Origin":  "https://notexample.com",
		"Access-Control-Allow-Methods": "GET",
		"Access-Control-Allow-Headers": "x-custom-header",
	}

	// CORS at the vhost level translated from the TrafficPolicy should have
	// this cors response headers for all other routes
	expectedHeadersPath2 := map[string]any{
		"Access-Control-Allow-Origin":  "https://notexample.com",
		"Access-Control-Allow-Methods": "GET, POST",
		"Access-Control-Allow-Headers": "Content-Type, Authorization",
	}

	s.assertResponse("/path1", requestHeaders, expectedHeadersPath1, []string{})
	s.assertResponse("/path2", requestHeaders, expectedHeadersPath2, []string{})
}

func (s *testingSuite) assertResponse(path string, requestHeaders map[string]string, expectedHeaders map[string]any, notExpectedHeaders []string) {
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithMethod(http.MethodOptions),
			curl.WithPath(path),
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPort(8080),
			curl.WithHeaders(requestHeaders),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Headers:    expectedHeaders,
			NotHeaders: notExpectedHeaders,
		})
}
