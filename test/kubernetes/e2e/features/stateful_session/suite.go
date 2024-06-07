package stateful_session

import (
	"context"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	. "github.com/onsi/gomega"

	"github.com/solo-io/gloo/test/gomega/transforms"
	"github.com/stretchr/testify/suite"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/skv2/codegen/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// testingSuite is the entire Suite of tests for the "example" feature
// Typically, we would include a link to the feature code here
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	ti *e2e.TestInstallation

	// maps test name to a list of manifests to apply before the test
	manifests       map[string][]string
	manifestObjects map[string][]client.Object
}

const (
	cookieName = "sessionaffinitycookie"
	headerName = "sessionaffinityheader"
)

var (
	CurlPodExecOpt = kubectl.PodExecOptions{
		Name:      "curl",
		Namespace: "curl",
		Container: "curl",
	}

	curlOptsCookies = []curl.Option{
		curl.WithCookie("/tmp/cookie.txt"),
		curl.WithCookieJar("/tmp/cookie.txt"),
	}

	// Need the testing suite to set the host, so define this in SetupSuite
	curlOptsCommon []curl.Option
)

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx: ctx,
		ti:  testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	s.manifests = map[string][]string{
		"TestStatefulSessionCookieBased":     {sessionAffinityManifest, statefulSessionCookieGatewayManifest},
		"TestStatefulSessionCookieNotInPath": {sessionAffinityManifest, statefulSessionCookieGatewayManifest},
		"TestStatefulSessionCookieStrict":    {sessionAffinityManifest, statefulSessionCookieGatewayStrictManifest},
		"TestStatefulSessionHeaderBased":     {sessionAffinityManifest, statefulSessionHeaderGatewayManifest},
		"TestStatefulSessionHeaderStrict":    {sessionAffinityManifest, statefulSessionHeaderGatewayStrictManifest},
	}

	curlOptsCommon = []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.ti.Metadata.InstallNamespace})),
		curl.WithPort(80),
		curl.Silent(),
		curl.WithHostHeader("app"),
	}

}

func (s *testingSuite) TearDownSuite() {
	// This is code that will be executed after an entire suite is run
}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	manifests := s.manifests[testName]
	for _, manifest := range manifests {
		err := s.ti.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
		s.ti.Assertions.EventuallyObjectsExist(s.ctx, s.manifestObjects[manifest]...)
	}
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	manifests := s.manifests[testName]
	for _, manifest := range manifests {
		err := s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err)
		s.ti.Assertions.EventuallyObjectsNotExist(s.ctx, s.manifestObjects[manifest]...)
	}
}

var (
	sessionAffinityManifest                    = filepath.Join(util.MustGetThisDir(), "testdata", "session_affinity.yaml")
	statefulSessionCookieGatewayManifest       = filepath.Join(util.MustGetThisDir(), "testdata", "cookie_gateway.yaml")
	statefulSessionCookieGatewayStrictManifest = filepath.Join(util.MustGetThisDir(), "testdata", "cookie_gateway_strict.yaml")
	statefulSessionHeaderGatewayManifest       = filepath.Join(util.MustGetThisDir(), "testdata", "header_gateway.yaml")
	statefulSessionHeaderGatewayStrictManifest = filepath.Join(util.MustGetThisDir(), "testdata", "header_gateway_strict.yaml")
)

// These tests all use the "counter" application described here: https://docs.solo.io/gloo-edge/latest/installation/advanced_configuration/session_affinity/#overview-of-the-counter-application
// It basically returns a count of the number of requests it has received and can help us determine if requests are being routed to the same pod
func (s *testingSuite) TestStatefulSessionCookieBased() {
	numRequests := 20

	curlOpts := append(curlOptsCommon, curlOptsCookies...)
	curlOpts = append(curlOpts, curl.WithPath("/session_path/count"))

	// Get the first response - this one we may have to wait for
	// This is also the only response with a cookie and TTL
	// TTL is handled client side, so we only test that the header is returned
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		CurlPodExecOpt,
		curlOpts,
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       "1",
			Headers: map[string]interface{}{
				"Set-Cookie": And(ContainSubstring("; Max-Age=10;"), ContainSubstring(cookieName)),
			},
		},
		10*time.Second,
	)

	// Once responses are coming, they should keep incrementing
	for i := 2; i <= numRequests; i++ {
		s.ti.Assertions.AssertCurlResponse(
			s.ctx,
			CurlPodExecOpt,
			curlOpts,
			&matchers.HttpResponse{StatusCode: http.StatusOK, Body: strconv.Itoa(i)},
		)
	}

}

func (s *testingSuite) TestStatefulSessionCookieNotInPath() {
	numRequests := 99

	curlOpts := append(curlOptsCommon, curlOptsCookies...)
	curlOpts = append(curlOpts, curl.WithPath("/non_session_path/count"))

	// Envoy round robin load balancing may not appear to be even, and we can not rely on a predictable sequence of distribution of requests
	// https://www.envoyproxy.io/docs/envoy/latest/faq/load_balancing/concurrency_lb
	// We are not attempting to test Envoy's round robin load balancer, only establishing a baseline negative test case that there is no session affinity on this path,
	// so we will test for a roughly even distribution of requests by running 100 requests across 4 replicas. These requests return a count of requests to that pod.
	// after these 100 requests, we will make 4 more requests and valdiate that the count returned is <35. This is broad, but is good enough to validate non-stickiness
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		CurlPodExecOpt,
		curlOpts,
		&matchers.HttpResponse{StatusCode: http.StatusOK, Body: "1"},
		10*time.Second,
	)

	// Once responses are coming, they should keep succeeding
	for i := 0; i <= numRequests; i++ {
		s.ti.Assertions.AssertCurlResponse(
			s.ctx,
			CurlPodExecOpt,
			curlOpts,
			&matchers.HttpResponse{StatusCode: http.StatusOK},
		)
	}

	// After 100 requests, ensure that responses are < 35
	for i := 0; i <= 4; i++ {
		s.ti.Assertions.AssertCurlResponse(
			s.ctx,
			CurlPodExecOpt,
			curlOpts,
			&matchers.HttpResponse{
				StatusCode: http.StatusOK,
				Body:       WithTransform(transforms.BytesToInt, BeNumerically("<=", 35)),
			},
		)
	}
}

func (s *testingSuite) TestStatefulSessionCookieStrict() {
	curlOpts := append(curlOptsCommon, curlOptsCookies...)
	curlOpts = append(curlOpts, curl.WithPath("/session_path/count"))

	curlOptsWithoutCookies := append(curlOptsCommon, curl.WithPath("/session_path/count"))

	// Get the first response - this one we may have to wait for
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		CurlPodExecOpt,
		curlOpts,
		&matchers.HttpResponse{StatusCode: http.StatusOK, Body: "1"},
		10*time.Second,
	)

	// Scale down the deployment to 0
	s.ti.Actions.Kubectl().ScaleDeploymentTo(s.ctx, "session-affinity", 0)

	// Wait until we get a 503 - don't use the cookies to avoid any side effects
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		CurlPodExecOpt,
		curlOptsWithoutCookies,
		&matchers.HttpResponse{StatusCode: http.StatusServiceUnavailable},
		10*time.Second,
	)

	// Scale back up to 4
	s.ti.Actions.Kubectl().ScaleDeploymentTo(s.ctx, "session-affinity", 4)

	// Should get a 200 when not using the cookie
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		CurlPodExecOpt,
		curlOptsWithoutCookies,
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Headers: map[string]interface{}{
				"Set-Cookie": ContainSubstring(cookieName),
			},
		},
		10*time.Second,
	)

	// Should get a 503 when using the cookie
	s.ti.Assertions.AssertCurlResponse(
		s.ctx,
		CurlPodExecOpt,
		curlOpts,
		&matchers.HttpResponse{StatusCode: http.StatusServiceUnavailable},
	)

}

func (s *testingSuite) TestStatefulSessionHeaderBased() {
	numRequests := 20

	curlOpts := append(curlOptsCommon, curlOptsCookies...)
	curlOpts = append(curlOpts, curl.WithPath("/session_path/count"))

	// Get the first response - this one we may have to wait for
	response := s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		CurlPodExecOpt,
		curlOpts,
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       "1",
			Headers: map[string]interface{}{
				headerName: Not(BeEmpty()),
			},
		},
		10*time.Second,
	)

	sessionHeaderVal := response.Header.Get(headerName)
	curlOpts = append(curlOpts, curl.WithHeader(headerName, sessionHeaderVal))

	// Once responses are coming, they should keep incrementing
	for i := 2; i <= numRequests; i++ {
		s.ti.Assertions.AssertCurlResponse(
			s.ctx,
			CurlPodExecOpt,
			curlOpts,
			&matchers.HttpResponse{StatusCode: http.StatusOK, Body: strconv.Itoa(i)},
		)
	}

}

// func (s *testingSuite) TestStatefulSessionHeaderStrict() {
// 	curlOpts := append(curlOptsCommon, curlOptsCookies...)
// 	curlOpts = append(curlOpts, curl.WithPath("/session_path/count"))

// 	// Get the first response - this one we may have to wait for
// 	response := s.ti.Assertions.AssertEventualCurlResponse(
// 		s.ctx,
// 		CurlPodExecOpt,
// 		curlOpts,
// 		&matchers.HttpResponse{
// 			StatusCode: http.StatusOK,
// 			Body:       "1",
// 			Headers: map[string]interface{}{
// 				headerName: Not(BeEmpty()),
// 			},
// 		},
// 		10*time.Second,
// 	)

// 	sessionHeaderVal := response.Header.Get(headerName)
// 	curlOptsWithHeader := append(curlOpts, curl.WithHeader(headerName, sessionHeaderVal))

// 	// Scale down the deployment to 0
// 	s.ti.Actions.Kubectl().ScaleDeploymentTo(s.ctx, "session-affinity", 0)

// 	// Wait until we get a 503 - don't use the header to avoid any side effects
// 	s.ti.Assertions.AssertEventualCurlResponse(
// 		s.ctx,
// 		CurlPodExecOpt,
// 		curlOpts,
// 		&matchers.HttpResponse{StatusCode: http.StatusServiceUnavailable},
// 		10*time.Second,
// 	)

// 	// Scale back up to 4
// 	s.ti.Actions.Kubectl().ScaleDeploymentTo(s.ctx, "session-affinity", 4)

// 	// Should get a 200 when not using the header
// 	s.ti.Assertions.AssertEventualCurlResponse(
// 		s.ctx,
// 		CurlPodExecOpt,
// 		curlOpts,
// 		&matchers.HttpResponse{
// 			StatusCode: http.StatusOK,
// 			Headers: map[string]interface{}{
// 				headerName: Not(BeEmpty()),
// 			},
// 		},
// 		10*time.Second,
// 	)

// 	// Should get a 503 when using the header
// 	s.ti.Assertions.AssertCurlResponse(
// 		s.ctx,
// 		CurlPodExecOpt,
// 		curlOptsWithHeader,
// 		&matchers.HttpResponse{StatusCode: http.StatusServiceUnavailable},
// 	)

// }
