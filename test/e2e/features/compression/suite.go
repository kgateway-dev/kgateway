//go:build e2e

package compression

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	adminv3 "github.com/envoyproxy/go-control-plane/envoy/admin/v3"
	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/envoyutils/admincli"
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
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
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

// Sends a gzip-encoded request body and asserts decompressor handles it
func (s *testingSuite) TestRequestDecompression() {
	// first get a gzip file; the easiest way to do this is to GET a compressed response
	// and write it to a file
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithPort(8080),
			curl.WithPath("/html"),
			curl.WithHostHeader("example.com"),
			curl.WithArgs([]string{"--output", "/tmp/gzfile"}),
			curl.WithHeaders(map[string]string{"Accept-Encoding": "gzip"}),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Headers:    map[string]any{"Content-Encoding": "gzip"},
		},
	)

	// Now for the test, post the gzipped body
	// we post to /json because the policy is set there..
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithPort(8080),
			curl.WithPath("/json"),
			curl.WithHostHeader("example.com"),
			curl.WithBody("@/tmp/gzfile"),
			curl.WithHeaders(map[string]string{
				"Content-Encoding": "gzip",
			}),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		},
	)

	// Verify decompressor filter emitted metrics indicating it handled the request via Envoy admin API
	s.TestInstallation.AssertionsT(s.T()).AssertEnvoyAdminApi(
		s.Ctx,
		proxyObjectMeta,
		func(ctx context.Context, adminClient *admincli.Client) {
			s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
				metricSuffix := ".decompressor.gzip.request.decompressed"
				out, err := adminClient.GetStats(ctx, map[string]string{
					"format": "json",
					"filter": ".*" + strings.ReplaceAll(metricSuffix, ".", "\\.") + "$",
				})
				g.Expect(err).NotTo(gomega.HaveOccurred(), "can get envoy stats")

				var resp map[string][]adminv3.SimpleMetric
				g.Expect(json.Unmarshal([]byte(out), &resp)).To(gomega.Succeed(), "can unmarshal envoy stats response")

				stats := resp["stats"]
				g.Expect(stats).To(gomega.HaveLen(1), "expected 1 matching stats result")
				g.Expect(stats[0].GetName()).To(gomega.HaveSuffix(metricSuffix))
				g.Expect(stats[0].GetValue()).To(gomega.BeNumerically(">=", 1.0))
			}).WithTimeout(time.Second * 10).WithPolling(time.Second).
				Should(gomega.Succeed())
		},
	)
}
