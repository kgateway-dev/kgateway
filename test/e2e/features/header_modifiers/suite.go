//go:build e2e

package header_modifiers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

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
	localGateway common.Gateway
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

func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()

	address := s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayAddress(
		s.Ctx,
		proxyObjectMeta.Name,
		proxyObjectMeta.Namespace,
	)
	s.localGateway = common.Gateway{
		NamespacedName: types.NamespacedName{
			Name:      proxyObjectMeta.Name,
			Namespace: proxyObjectMeta.Namespace,
		},
		Address: address,
	}
}

func (s *testingSuite) checkPodsRunning() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx,
		testdefaults.HttpbinDeployment.GetNamespace(), metav1.ListOptions{
			LabelSelector: testdefaults.HttpbinLabelSelector,
		})
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx,
		proxyObjectMeta.GetNamespace(), metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s", testdefaults.WellKnownAppLabel, proxyObjectMeta.GetName()),
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

func (s *testingSuite) TestHeaderModifiersFromSecret() {
	s.checkPodsRunning()
	// The TrafficPolicy injects X-Api-Key and X-Tenant-Id from the backend-creds Secret.
	s.assertHeaders(8080, map[string][]any{
		"X-Api-Key":   {"my-secret-api-key"},
		"X-Tenant-Id": {"tenant-abc"},
	}, nil)
}

func (s *testingSuite) TestMultiLevelHeaderModifiers() {
	s.checkPodsRunning()
	s.assertHeaders(8080, expectedRequestHeaders("route", "gw"), nil)
}

func (s *testingSuite) TestMultiLevelHeaderModifiersWithListenerSet() {
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

	s.localGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Headers:    responseHeaders,
			NotHeaders: []string{"X-Request-Id", "X-Envoy-Upstream-Service-Time"},
			Body:       testmatchers.JSONContains(requestHeadersJSON),
		},
		curl.WithPath("/headers"),
		curl.WithHostHeader("example.com"),
		curl.WithPort(port),
	)
}
