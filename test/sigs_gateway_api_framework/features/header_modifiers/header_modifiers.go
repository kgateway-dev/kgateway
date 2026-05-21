//go:build e2e

package header_modifiers

import (
	"embed"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/gateway-api/conformance/utils/http"
	"sigs.k8s.io/gateway-api/conformance/utils/kubernetes"
	confsuite "sigs.k8s.io/gateway-api/conformance/utils/suite"

	"github.com/kgateway-dev/kgateway/v2/test/sigs_gateway_api_framework/common"
)

//go:embed testdata
var ManifestFS embed.FS

const (
	testNamespace = "kgateway-conformance-test"
	routeName     = "header-modifiers-route"
	routeHostname = "header-modifiers.example.com"
	echoBackend   = "echo-server"
)

// Tests exports all header-modifiers conformance scenarios.
var Tests = []confsuite.ConformanceTest{
	RequestHeaderModifier,
}

// RequestHeaderModifier exercises request header modification via HTTPRoute filters.
// The test verifies that the RequestHeaderModifier filter adds headers to requests
// sent to the backend.
var RequestHeaderModifier = confsuite.ConformanceTest{
	ShortName:   "RequestHeaderModifier",
	Description: "An HTTPRoute with request header modifier filters adds headers to backend requests.",
	Manifests:   []string{"testdata/header-modifiers-http-route.yaml"},
	Test: func(t *testing.T, s *confsuite.ConformanceTestSuite) {
		gwNN := types.NamespacedName{Name: common.GatewayName, Namespace: testNamespace}
		routeNN := types.NamespacedName{Name: routeName, Namespace: testNamespace}

		gwAddr := kubernetes.GatewayAndHTTPRoutesMustBeAccepted(
			t, s.Client, s.TimeoutConfig, s.ControllerName,
			kubernetes.NewGatewayRef(gwNN), routeNN,
		)
		kubernetes.HTTPRouteMustHaveResolvedRefsConditionsTrue(
			t, s.Client, s.TimeoutConfig, routeNN, gwNN,
		)

		testCases := []http.ExpectedResponse{
			{
				Request: http.Request{
					Host: routeHostname,
					Path: "/set",
				},
				ExpectedRequest: &http.ExpectedRequest{
					Request: http.Request{
						Path: "/set",
						Headers: map[string]string{
							"X-Test-Header": "test-value",
						},
					},
				},
				Backend:   echoBackend,
				Namespace: testNamespace,
			},
			{
				Request: http.Request{
					Host: routeHostname,
					Path: "/add",
				},
				ExpectedRequest: &http.ExpectedRequest{
					Request: http.Request{
						Path: "/add",
						Headers: map[string]string{
							"X-Added-Header": "added-value",
						},
					},
				},
				Backend:   echoBackend,
				Namespace: testNamespace,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.Request.Path, func(t *testing.T) {
				http.MakeRequestAndExpectEventuallyConsistentResponse(
					t, s.RoundTripper, s.TimeoutConfig,
					gwAddr, tc,
				)
			})
		}
	},
}
