package header_modifiers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

func (s *testingSuite) checkPodsRunning() {
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx,
		testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
			LabelSelector: testdefaults.CurlPodLabelSelector,
		})
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx,
		testdefaults.HttpbinDeployment.GetNamespace(), metav1.ListOptions{
			LabelSelector: testdefaults.HttpbinLabelSelector,
		})
	s.TestInstallation.Assertions.EventuallyPodsRunning(s.Ctx,
		proxyObjectMeta.GetNamespace(), metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", proxyObjectMeta.GetName()),
		})
}

func (s *testingSuite) TestRouteLevelHeaderModifiers() {
	s.checkPodsRunning()
	s.assertHeaders(8080, expectedRequestHeaders("route"), expectedResponseHeaders("route"))
}

func (s *testingSuite) TestGatewayLevelHeaderModifiers() {
	s.checkPodsRunning()
	s.assertHeaders(8080, expectedRequestHeaders("gw"), expectedResponseHeaders("gw"))
}

func (s *testingSuite) TestListenerSetLevelHeaderModifiers() {
	s.checkPodsRunning()
	s.assertHeaders(8081, expectedRequestHeaders("ls"), expectedResponseHeaders("ls"))
}

func (s *testingSuite) TestMultiLevelHeaderModifiers() {
	s.checkPodsRunning()
	s.assertHeaders(8080, expectedRequestHeaders("route", "gw"), nil)
	s.assertHeaders(8081, expectedRequestHeaders("route", "ls", "gw"), nil)
}

func expectedRequestHeaders(suffixes ...string) map[string][]any {
	h := map[string][]any{}

	for _, suffix := range suffixes {
		h["X-Custom-Request-Header"] = append(h["X-Custom-Request-Header"],
			"custom-request-value-"+suffix)
	}

	if len(suffixes) > 0 {
		h["X-Custom-Request-Header-Set"] = []any{
			"custom-request-value-" + suffixes[len(suffixes)-1]}
	}

	return h
}

func expectedResponseHeaders(suffix string) map[string]any {
	return map[string]any{
		"X-Custom-Response-Header":     "custom-response-value-" + suffix,
		"X-Custom-Response-Header-Set": "custom-response-value-" + suffix,
	}
}

func (s *testingSuite) assertHeaders(port int,
	requestHeaders map[string][]any,
	responseHeaders map[string]any,
) {
	allOptions := []curl.Option{
		curl.WithPath("/headers"),
		curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
		curl.WithHostHeader("example.com"),
		curl.WithPort(port),
	}

	requestHeadersJSON, err := json.Marshal(map[string]any{"headers": requestHeaders})
	s.Require().NoError(err, "unable to marshal request headers to JSON")

	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		allOptions,
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Headers:    responseHeaders,
			NotHeaders: []string{"X-Request-Id", "X-Envoy-Upstream-Service-Time"},
			Body:       testmatchers.JSONContains(requestHeadersJSON),
		},
	)
}
