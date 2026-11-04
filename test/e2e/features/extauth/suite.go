//go:build e2e

package extauth

import (
	"context"
	"net/http"
	"strings"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of tests for ExtAuth functionality
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	setup := base.TestCase{
		Manifests: []string{
			routeManifest,
			gatewayWithRouteManifest,
			extAuthManifest,
		},
	}
	testCases := map[string]*base.TestCase{
		"TestExtAuthPolicy": {
			Manifests: []string{securedGatewayPolicyManifest, insecureRouteManifest},
		},
		"TestRouteTargetedExtAuthPolicy": {
			Manifests: []string{securedRouteManifest, insecureRouteManifest},
		},
		// The buffer filter's placement is a property of the whole filter chain, so the staged and
		// default-staged routes cannot share a listener: they run as separate test cases.
		"TestBufferFilterStageEnforcesAheadOfExtAuth": {
			Manifests: []string{bufferedRouteManifest},
		},
		"TestDefaultBufferStageDoesNotEnforceBehindExtAuth": {
			Manifests: []string{bufferedRouteDefaultStageManifest},
		},
	}
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// TestExtAuthPolicy tests the basic ExtAuth functionality with header-based allow/deny
// Checks for gateway level auth with route level opt out
func (s *testingSuite) TestExtAuthPolicy() {
	testCases := []struct {
		name                         string
		headers                      map[string]string
		hostname                     string
		expectedStatus               int
		expectedUpstreamBodyContents string
	}{
		{
			name: "request allowed with allow header",
			headers: map[string]string{
				"x-ext-authz": "allow",
			},
			hostname:                     "example.com",
			expectedStatus:               http.StatusOK,
			expectedUpstreamBodyContents: "X-Ext-Authz-Check-Result",
		},
		{
			name:           "request denied without allow header",
			headers:        map[string]string{},
			hostname:       "example.com",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:     "request denied with deny header",
			hostname: "example.com",
			headers: map[string]string{
				"x-ext-authz": "deny",
			},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "request allowed on insecure route",
			hostname:       "insecureroute.com",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Build curl options
			opts := []curl.Option{
				curl.WithHostHeader(tc.hostname),
				curl.WithPort(80),
			}

			// Add test-specific headers
			for k, v := range tc.headers {
				opts = append(opts, curl.WithHeader(k, v))
			}

			// Test the request
			common.BaseGateway.Send(
				s.T(),
				&testmatchers.HttpResponse{
					StatusCode: tc.expectedStatus,
					Body:       gomega.ContainSubstring(tc.expectedUpstreamBodyContents),
				},
				opts...)
		})
	}
}

// TestRouteTargetedExtAuthPolicy tests route level only extauth
func (s *testingSuite) TestRouteTargetedExtAuthPolicy() {
	testCases := []struct {
		name                         string
		headers                      map[string]string
		hostname                     string
		expectedStatus               int
		expectedUpstreamBodyContents string
	}{
		{
			name:           "request allowed by default",
			headers:        map[string]string{},
			hostname:       "example.com",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "request allowed on insecure route",
			hostname:       "insecureroute.com",
			expectedStatus: http.StatusOK,
		},
		{
			name: "request allowed with allow header on secured route",
			headers: map[string]string{
				"x-ext-authz": "allow",
			},
			hostname:                     "secureroute.com",
			expectedStatus:               http.StatusOK,
			expectedUpstreamBodyContents: "X-Ext-Authz-Check-Result",
		},
		{
			name:           "request denied without header on secured route",
			hostname:       "secureroute.com",
			headers:        map[string]string{},
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Build curl options
			opts := []curl.Option{
				curl.WithHostHeader(tc.hostname),
				curl.WithPort(80),
			}

			// Add test-specific headers
			for k, v := range tc.headers {
				opts = append(opts, curl.WithHeader(k, v))
			}

			// Test the request
			common.BaseGateway.Send(
				s.T(),
				&testmatchers.HttpResponse{
					StatusCode: tc.expectedStatus,
					Body:       gomega.ContainSubstring(tc.expectedUpstreamBodyContents),
				},
				opts...)
		})
	}
}

// bufferedRequest is a request against one of the buffered routes: `maxRequestSize` is 1024 on
// both, and ext_authz reads the body with an 8192-byte limit of its own, so the only filter that
// can reject a body between those sizes is the buffer filter.
type bufferedRequest struct {
	name           string
	hostname       string
	headers        map[string]string
	bodySize       int
	expectedStatus int
}

func (s *testingSuite) sendBufferedRequests(testCases []bufferedRequest) {
	for _, tc := range testCases {
		s.Run(tc.name, func() {
			opts := []curl.Option{
				curl.WithHostHeader(tc.hostname),
				curl.WithPort(80),
				curl.WithBody(strings.Repeat("x", tc.bodySize)),
			}
			for k, v := range tc.headers {
				opts = append(opts, curl.WithHeader(k, v))
			}

			common.BaseGateway.Send(
				s.T(),
				&testmatchers.HttpResponse{
					StatusCode: tc.expectedStatus,
				},
				opts...)
		})
	}
}

// TestBufferFilterStageEnforcesAheadOfExtAuth checks that `buffer.filterStage` makes
// `maxRequestSize` enforce ahead of an ext_authz check that reads the request body: an oversized
// body gets a 413 whether or not the auth service would have allowed it, which is what proves the
// buffer filter runs first.
func (s *testingSuite) TestBufferFilterStageEnforcesAheadOfExtAuth() {
	s.sendBufferedRequests([]bufferedRequest{
		{
			name:           "small body with allow header reaches the backend",
			hostname:       "bufferedroute.com",
			headers:        map[string]string{"x-ext-authz": "allow"},
			bodySize:       512,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "small body without allow header is denied by auth",
			hostname:       "bufferedroute.com",
			bodySize:       512,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "oversized body with allow header is rejected by the buffer filter",
			hostname:       "bufferedroute.com",
			headers:        map[string]string{"x-ext-authz": "allow"},
			bodySize:       1500,
			expectedStatus: http.StatusRequestEntityTooLarge,
		},
		{
			// A 403 here would mean ext_authz ran first.
			name:           "oversized body is rejected before auth gets to deny it",
			hostname:       "bufferedroute.com",
			bodySize:       1500,
			expectedStatus: http.StatusRequestEntityTooLarge,
		},
	})
}

// TestDefaultBufferStageDoesNotEnforceBehindExtAuth pins the behavior `buffer.filterStage` exists
// to work around: with ext_authz reading the body ahead of the buffer filter's default placement,
// `maxRequestSize` is inert and an oversized body reaches the backend. See
// buffered-route-default-stage.yaml.
func (s *testingSuite) TestDefaultBufferStageDoesNotEnforceBehindExtAuth() {
	s.sendBufferedRequests([]bufferedRequest{
		{
			name:           "oversized body with allow header reaches the backend",
			hostname:       "bufferedroute-default.com",
			headers:        map[string]string{"x-ext-authz": "allow"},
			bodySize:       1500,
			expectedStatus: http.StatusOK,
		},
		{
			// Auth runs first at the default placement, so the oversized body is denied, not 413'd.
			name:           "oversized body without allow header is denied by auth",
			hostname:       "bufferedroute-default.com",
			bodySize:       1500,
			expectedStatus: http.StatusForbidden,
		},
	})
}
