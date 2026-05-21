//go:build e2e

package basicrouting

import (
	"embed"
	"net"
	nethttp "net/http"
	"strconv"
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
	listenerHighPort = 8080
	listenerLowPort  = 80
	routeHostname    = "example.com"
	echoBackendName  = "echo-server"
	testNamespace    = "kgateway-conformance-test"
	routeName        = "basicrouting-route"
)

// Tests exports all basicrouting conformance scenarios.
var Tests = []confsuite.ConformanceTest{
	GatewayWithRoute,
}

// GatewayWithRoute exercises a single HTTPRoute attached to a Gateway with
// two HTTP listeners (ports 80 and 8080), via the upstream Gateway API
// conformance framework. The framework auto-applies test manifests and
// registers teardown via t.Cleanup; the test only asserts behaviour.
var GatewayWithRoute = confsuite.ConformanceTest{
	ShortName:   "GatewayWithRoute",
	Description: "An HTTPRoute attached to a Gateway routes requests to the echo backend on each listener port.",
	Manifests:   []string{"testdata/basicrouting-http-route.yaml"},
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

		for _, port := range []int{listenerHighPort, listenerLowPort} {
			t.Run("listener_port_"+strconv.Itoa(port), func(t *testing.T) {
				http.MakeRequestAndExpectEventuallyConsistentResponse(
					t, s.RoundTripper, s.TimeoutConfig,
					addressOnPort(gwAddr, port),
					http.ExpectedResponse{
						Request:   http.Request{Host: routeHostname, Path: "/"},
						Response:  http.Response{StatusCode: nethttp.StatusOK},
						Backend:   echoBackendName,
						Namespace: testNamespace,
					},
				)
			})
		}
	},
}

// addressOnPort replaces the port in a host:port address with the given port.
// GatewayAndHTTPRoutesMustBeAccepted returns the address using only the first
// listener's port, so we override it to exercise each listener individually.
func addressOnPort(addr string, port int) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}
