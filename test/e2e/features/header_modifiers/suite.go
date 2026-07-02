//go:build e2e

package header_modifiers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	// Define versioned setups - the system will select the appropriate one based on Gateway API version and channel
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases,
			base.WithSetupByVersion(map[base.GwApiChannel]map[base.GwApiVersion]*base.TestCase{
				base.GwApiChannelExperimental: {
					base.GwApiV1_3_0: &setupWithListenerSets, // XListenerSet available in experimental >= 1.3
				},
				base.GwApiChannelStandard: {
					base.GwApiV1_5_0: &setupWithListenerSets, // ListenerSet promoted in standard >= 1.5.0
				},
			}),
		),
	}
}

// checkPodsRunning waits for the httpbin backend to be ready. The gateway proxy is the
// shared base gateway (gateway/kgateway-base), already brought up and address-resolved by
// the test runner, so we only need to gate on the backend here.
func (s *testingSuite) checkPodsRunning() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx,
		testdefaults.HttpbinDeployment.GetNamespace(), metav1.ListOptions{
			LabelSelector: testdefaults.HttpbinLabelSelector,
		})
}

// baseGatewayPort is the sentinel passed to assertHeaders for traffic that should hit the
// shared base gateway's default listener (:80). Passing 0 omits curl.WithPort so the address
// (and any GATEWAY_ADDRESS_OVERRIDE) is honored. ListenerSet tests pass the explicit LS port.
const baseGatewayPort = 0

func (s *testingSuite) TestRouteLevelHeaderModifiers() {
	s.checkPodsRunning()
	s.assertHeaders(baseGatewayPort, expectedRequestHeaders("route"), expectedResponseHeaders("route"))
}

func (s *testingSuite) TestGatewayLevelHeaderModifiers() {
	s.checkPodsRunning()
	s.assertHeaders(baseGatewayPort, expectedRequestHeaders("gw"), expectedResponseHeaders("gw"))
}

func (s *testingSuite) TestListenerSetLevelHeaderModifiers() {
	s.checkPodsRunning()
	s.assertHeaders(8081, expectedRequestHeaders("ls"), expectedResponseHeaders("ls"))
}

func (s *testingSuite) TestHeaderModifiersFromSecret() {
	s.checkPodsRunning()
	// The TrafficPolicy injects X-Api-Key and X-Tenant-Id from the backend-creds Secret.
	s.assertHeaders(baseGatewayPort, map[string][]any{
		"X-Api-Key":   {"my-secret-api-key"},
		"X-Tenant-Id": {"tenant-abc"},
	}, nil)
}

func (s *testingSuite) TestMultiLevelHeaderModifiers() {
	s.checkPodsRunning()
	s.assertHeaders(baseGatewayPort, expectedRequestHeaders("route", "gw"), nil)
}

func (s *testingSuite) TestMultiLevelHeaderModifiersWithListenerSet() {
	s.checkPodsRunning()
	s.assertHeaders(baseGatewayPort, expectedRequestHeaders("route", "gw"), nil)
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
			"custom-request-value-" + suffixes[len(suffixes)-1],
		}
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
	requestHeadersJSON, err := json.Marshal(map[string]any{"headers": requestHeaders})
	s.Require().NoError(err, "unable to marshal request headers to JSON")

	opts := []curl.Option{
		curl.WithPath("/headers"),
		curl.WithHostHeader("example.com"),
	}
	if port != baseGatewayPort {
		opts = append(opts, curl.WithPort(port))
	}

	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Headers:    responseHeaders,
			NotHeaders: []string{"X-Request-Id", "X-Envoy-Upstream-Service-Time"},
			Body:       testmatchers.JSONContains(requestHeadersJSON),
		},
		opts...,
	)
}
