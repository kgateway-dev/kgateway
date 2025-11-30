//go:build e2e

package compression

import (
	"context"
	"net/http"

	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// Verifies response compression is applied only to the route targeted by the TrafficPolicy
func (s *testingSuite) TestTrafficPolicyResponseCompressionForRoute() {
	// Compressed route: expect Content-Encoding
	s.assertHeaders("/html",
		map[string]string{"Accept-Encoding": "gzip"},
		map[string]any{"Content-Encoding": "gzip"},
		nil,
	)

	// Uncompressed route: header should be absent
	s.assertHeaders("/json",
		map[string]string{"Accept-Encoding": "gzip"},
		nil,
		[]string{"Content-Encoding"},
	)
}

// Verifies that without Accept-Encoding the compressed route does not return Content-Encoding
func (s *testingSuite) TestNoCompressionWithoutAcceptEncoding() {
	s.assertHeaders("/html", nil, nil, []string{"Content-Encoding"})
}

func (s *testingSuite) assertHeaders(path string, reqHeaders map[string]string, expectedHeaders map[string]any, notExpectedHeaders []string) {
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithPort(8080),
			curl.WithPath(path),
			curl.WithHostHeader("example.com"),
			curl.WithIgnoreBody(),
			curl.WithHeaders(reqHeaders),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Headers:    expectedHeaders,
			NotHeaders: notExpectedHeaders,
		},
	)
}
