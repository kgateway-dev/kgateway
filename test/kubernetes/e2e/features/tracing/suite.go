package tracing

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
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

func (s *testingSuite) TestOTelTracing() {
	s.testOTelTracing()
}

func (s *testingSuite) TestOTelTracingSecure() {
	s.testOTelTracing()
}

// testOTelTracing makes a request to the httpbin service
// and checks if the collector pod logs contain the expected lines
func (s *testingSuite) testOTelTracing() {
	s.TestInstallation.Assertions.EventuallyHTTPListenerPolicyCondition(s.Ctx, "tracing-policy", "default", gwv1.GatewayConditionAccepted, metav1.ConditionTrue)

	// The headerValue passed is used to differentiate between multiple calls by identifying a unique trace per call
	headerValue := fmt.Sprintf("%v", rand.Intn(10000))
	s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
		// make curl request to httpbin service with the custom header
		s.TestInstallation.Assertions.AssertEventualCurlResponse(
			s.Ctx,
			defaults.CurlPodExecOpt,
			[]curl.Option{
				curl.WithHostHeader("www.example.com"),
				curl.WithHeader("x-header-tag", headerValue),
				curl.WithPath("/status/200"),
				curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			},
			&matchers.HttpResponse{
				StatusCode: 200,
			},
			20*time.Second,
			2*time.Second,
		)

		// Example trace found in the otel-collector logs
		// {"level":"info","ts":"2025-06-20T17:58:58.513Z","msg":"ResourceSpans #0\nResource SchemaURL: \nResource attributes:\n     -> service.name: Str(my:service)\n     -> telemetry.sdk.language: Str(cpp)\n     -> telemetry.sdk.name: Str(envoy)\n     -> telemetry.sdk.version: Str(a152096e910205ccf09863f93fc66150dc5438aa/1.34.1/Distribution/RELEASE/BoringSSL)\nScopeSpans #0\nScopeSpans SchemaURL: \nInstrumentationScope envoy a152096e910205ccf09863f93fc66150dc5438aa/1.34.1/Distribution/RELEASE/BoringSSL\nSpan #0\n    Trace ID       : 3771968e2d941a83f41517dd052fbfdb\n    Parent ID      : \n    ID             : 8d10646806636467\n    Name           : ingress\n    Kind           : Server\n    Start time     : 2025-06-20 17:58:55.73679 +0000 UTC\n    End time       : 2025-06-20 17:58:55.759439 +0000 UTC\n    Status code    : Unset\n    Status message : \nAttributes:\n     -> node_id: Str(gw-7fc7dbd6fc-fb24g.default)\n     -> zone: Str()\n     -> guid:x-request-id: Str(b5fa5226-ad2f-90a1-bd3a-e02d834301cd)\n     -> http.url: Str(http://www.example.com/status/200)\n     -> http.method: Str(GET)\n     -> downstream_cluster: Str(-)\n     -> user_agent: Str(curl/7.83.1-DEV)\n     -> http.protocol: Str(HTTP/1.1)\n     -> peer.address: Str(10.244.0.21)\n     -> request_size: Str(0)\n     -> response_size: Str(0)\n     -> component: Str(proxy)\n     -> upstream_cluster: Str(kube_httpbin_httpbin_8000)\n     -> upstream_cluster.name: Str(kube_httpbin_httpbin_8000)\n     -> http.status_code: Str(200)\n     -> response_flags: Str(-)\n     -> custom: Str(literal)\n     -> request: Str(value)\n","kind":"exporter","data_type":"traces","name":"debug"}
		expectedLines := []string{
			`-> http.url: Str(http://www.example.com/status/200)`,
			`-> http.method: Str(GET)`,
			`-> http.status_code: Str(200)`,
			`-> upstream_cluster: Str(kube_httpbin_httpbin_8000)`,
			// Resource attributes specified via the environmentResourceDetector
			`-> environment: Str(detector)`,
			`-> resource: Str(attribute)`,
			// Custom tag passed in the config
			`-> custom: Str(literal)`,
			// Custom tag fetched from the request header
			fmt.Sprintf("-> request: Str(%s)", headerValue),
		}

		// fetch the collector pod logs
		logs, err := s.TestInstallation.Actions.Kubectl().GetContainerLogs(s.Ctx, "default", "otel-collector")
		g.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to get pod logs")

		// check if the logs match the patterns
		allMatched := true
		for _, line := range expectedLines {
			if !strings.Contains(logs, line) {
				allMatched = false
			}
		}
		g.Expect(allMatched).To(gomega.BeTrue(), "lines not found in logs")
	}, time.Second*60, time.Second*15, "should find traces in collector pod logs").Should(gomega.Succeed())
}
