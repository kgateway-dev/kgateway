package local_rate_limit

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

// testingSuite is a suite of tests for local rate limiting functionality
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	// Define the setup TestCase for common resources
	setupTestCase := base.TestCase{
		Manifests: []string{
			testdefaults.CurlPodManifest,
			simpleServiceManifest,
			commonManifest,
		},
	}

	// Define test-specific TestCases
	testCases := map[string]*base.TestCase{
		"TestSimpleLocalRateLimit": {
			Manifests: []string{
				rateLimitManifest,
			},
		},
	}

	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setupTestCase, testCases),
	}
}

// TestSimpleLocalRateLimit tests basic local rate limiting functionality
func (s *testingSuite) TestSimpleLocalRateLimit() {
	// The BaseTestingSuite automatically handles setup and cleanup of test-specific resources

	// First request should be successful
	s.assertResponse("/test", http.StatusOK)

	// Second request should be rate limited (429)
	s.assertResponse("/test", http.StatusTooManyRequests)
}

func (s *testingSuite) assertResponse(path string, expectedStatus int) {
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithPath(path),
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: expectedStatus,
		})
}
