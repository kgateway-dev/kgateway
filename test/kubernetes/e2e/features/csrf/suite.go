//go:build e2e

package csrf

import (
	"context"
	"net/http"

	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of basic routing / "happy path" tests
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

func (s *testingSuite) TestRouteLevelCSRF() {
	// Request without origin header should be rejected
	s.assertPreflightResponse("/path1", http.StatusForbidden, []curl.Option{})

	// Request without origin header to route that doesn't have CSRF protection
	// should be allowed
	s.assertPreflightResponse("/path2", http.StatusOK, []curl.Option{})

	// Request with valid origin header should be allowed
	s.assertPreflightResponse("/path1", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "example.com"),
	})

	// Request with invalid origin header should be rejected
	s.assertPreflightResponse("/path1", http.StatusForbidden, []curl.Option{
		curl.WithHeader("Origin", "notexample.com"),
	})

	// Test suffix matching
	s.assertPreflightResponse("/path1", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "a.routetest.io"),
	})
}

func (s *testingSuite) TestGatewayLevelCSRF() {
	// Request without origin header should be rejected
	s.assertPreflightResponse("/path1", http.StatusForbidden, []curl.Option{})

	// Request without origin header should be rejected
	s.assertPreflightResponse("/path2", http.StatusForbidden, []curl.Option{})

	// Request with valid origin header should be allowed
	s.assertPreflightResponse("/path1", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "example.com"),
	})

	// Test suffix matching
	s.assertPreflightResponse("/path1", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "a.gwtest.io"),
	})

	// Request with valid origin header should be allowed
	s.assertPreflightResponse("/path2", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "example.com"),
	})

	// Test prefix matching
	s.assertPreflightResponse("/path2", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "sample.com"),
	})
}

func (s *testingSuite) TestMultiLevelsCSRF() {
	// Request without origin header should be rejected
	s.assertPreflightResponse("/path1", http.StatusForbidden, []curl.Option{})

	// Request without origin header should be rejected
	s.assertPreflightResponse("/path2", http.StatusForbidden, []curl.Option{})

	// Test suffix matching from route level policy (overrides the gateway additional origins)
	s.assertPreflightResponse("/path1", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "a.routetest.io"),
	})

	// Test suffix matching from gateway level policy as no route level policy is set
	s.assertPreflightResponse("/path2", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "a.gwtest.io"),
	})
}

func (s *testingSuite) TestShadowedRouteLevelCSRF() {
	// CSRF policies are being evaluated (not tested) but not enforced

	s.assertPreflightResponse("/path1", http.StatusOK, []curl.Option{})
	s.assertPreflightResponse("/path2", http.StatusOK, []curl.Option{})
	s.assertPreflightResponse("/path1", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "example.com"),
	})
	s.assertPreflightResponse("/path1", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "notexample.com"),
	})
}

// A safe http method is one that doesn't alter the state of the server (ie read only).
// A CSRF attack targets state changing requests, so the filter only acts on unsafe methods (ones that change state).
// We use POST as the unsafe method to test the filter.
func (s *testingSuite) assertPreflightResponse(path string, expectedStatus int, options []curl.Option) {
	allOptions := append([]curl.Option{
		curl.WithMethod("POST"),
		curl.WithPath(path),
		curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
		curl.WithHostHeader("example.com"),
		curl.WithPort(8080),
	}, options...)

	s.TestInstallation.Assertions.AssertEventuallyConsistentCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		allOptions,
		&testmatchers.HttpResponse{StatusCode: expectedStatus},
	)
}
