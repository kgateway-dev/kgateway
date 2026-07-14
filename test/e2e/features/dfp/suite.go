//go:build e2e

package dfp

import (
	"context"
	"net/http"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of tests for Dynamic Forward Proxy functionality
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	setup := base.TestCase{
		Manifests: []string{gatewayWithRouteManifest},
	}
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, nil),
	}
}

// TestDynamicForwardProxyBackend verifies that requests are dynamically forwarded to the
// upstream resolved from the request's host header
func (s *testingSuite) TestDynamicForwardProxyBackend() {
	testCases := []struct {
		name                         string
		headers                      map[string]string
		hostname                     string
		expectedStatus               int
		expectedUpstreamBodyContents string
	}{
		{
			name: "request forwarded upstream",
			headers: map[string]string{
				"x-header": "header-value",
			},
			// the DFP backend resolves the host header via DNS, so point it at the
			// shared base echo backend which reflects request headers in its response
			hostname:                     "backend.kgateway-base.svc.cluster.local",
			expectedStatus:               http.StatusOK,
			expectedUpstreamBodyContents: "X-Header",
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
				opts...,
			)
		})
	}
}
