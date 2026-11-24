//go:build e2e

package grpcweb

import (
	"context"
	"net/http"
	"testing"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite tests the TrafficPolicy grpcWeb field: browser gRPC-Web requests converted to
// gRPC for a backend that only speaks gRPC.
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

const (
	grpcWebContentType = "application/grpc-web+proto"
	// pingPath calls yages.Echo/Ping, which takes google.protobuf.Empty and returns "pong".
	pingPath = "/yages.Echo/Ping"
	// emptyMessageFrame is a gRPC-Web data frame (flag byte 0, 4-byte big-endian length 0)
	// carrying an empty protobuf message.
	emptyMessageFrame = "\x00\x00\x00\x00\x00"

	// routeHost has a route-level policy that enables gRPC-Web in TestGRPCWebOnRoute.
	routeHost = "grpc-web"
	// disabledHost has no policy in TestGRPCWebOnRoute, and a route-level policy that disables
	// gRPC-Web in TestGRPCWebOnGatewayWithRouteDisable.
	disabledHost = "grpc-web-disabled"
	// gatewayHost never has a route-level policy, so it is converted only when the Gateway-level
	// policy is in effect.
	gatewayHost = "grpc-web-gateway"
)

// TestGRPCWebOnRoute enables gRPC-Web on one route and checks that routes without it are
// unaffected.
func (s *testingSuite) TestGRPCWebOnRoute() {
	s.waitForRoutes()

	// Checked first: gatewayHost is converted only by a Gateway-level policy, so once it is
	// consistently not converted, no Gateway policy left over from another test is in effect and
	// the conversion checked below can only come from this test's route-level policy.
	s.T().Log("gRPC-Web request on a route without the policy is not converted")
	s.assertNotConverted(s.T(), gatewayHost)
	s.assertNotConverted(s.T(), disabledHost)

	s.T().Log("gRPC-Web request on the enabled route is converted to gRPC and back")
	s.assertConverted(s.T(), routeHost)
}

// TestGRPCWebOnGatewayWithRouteDisable enables gRPC-Web on the Gateway and disables it on one
// route with a more specific policy.
func (s *testingSuite) TestGRPCWebOnGatewayWithRouteDisable() {
	s.waitForRoutes()

	// Checked first: no route-level policy ever targets gatewayHost, so its conversion shows the
	// Gateway-level policy is in effect before the disable is checked against it.
	s.T().Log("gRPC-Web request on a route covered only by the Gateway policy is converted")
	s.assertConverted(s.T(), gatewayHost)

	s.T().Log("gRPC-Web request on the route that disables it is not converted")
	s.assertNotConverted(s.T(), disabledHost)
}

func (s *testingSuite) waitForRoutes() {
	for _, route := range []string{routeHost, disabledHost, gatewayHost} {
		s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
			s.Ctx, route, "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue,
		)
	}
}

func grpcWebRequest(host string) []curl.Option {
	return []curl.Option{
		curl.WithHostHeader(host),
		curl.WithPort(80),
		curl.WithMethod(http.MethodPost),
		curl.WithPath(pingPath),
		curl.WithContentType(grpcWebContentType),
		curl.WithHeader("X-Grpc-Web", "1"),
		curl.WithBody(emptyMessageFrame),
	}
}

// sendEventuallyConsistent waits for the response to match while config propagates, then
// requires it to keep matching, so a response seen once during a config change cannot pass.
func (s *testingSuite) sendEventuallyConsistent(t *testing.T, match func() *testmatchers.HttpResponse, host string) {
	t.Helper()
	common.BaseGateway.Send(t, match(), grpcWebRequest(host)...)
	common.BaseGateway.SendEventuallyConsistent(s.Ctx, t, match(), grpcWebRequest(host)...)
}

// assertConverted expects a gRPC-Web response: the "pong" message in a data frame and
// grpc-status 0 in the trailer frame that the gRPC-Web filter appends to the body.
func (s *testingSuite) assertConverted(t *testing.T, host string) {
	t.Helper()
	s.sendEventuallyConsistent(t, func() *testmatchers.HttpResponse {
		return &testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Headers: map[string]any{
				"Content-Type": gomega.HavePrefix("application/grpc-web"),
			},
			Body: gomega.And(
				gomega.ContainSubstring("pong"),
				gomega.ContainSubstring("grpc-status:0"),
			),
		}
	}, host)
}

// assertNotConverted expects the request to reach the backend unconverted. The backend is a
// plain gRPC server, whose gRPC library resets a stream with the gRPC-Web content type with an
// HTTP/2 PROTOCOL_ERROR, which Envoy reports as a 502 naming that reset reason. Asserting that
// exact response, rather than only "not a gRPC-Web response", keeps a 404 or 503 during a
// config change from passing.
func (s *testingSuite) assertNotConverted(t *testing.T, host string) {
	t.Helper()
	s.sendEventuallyConsistent(t, func() *testmatchers.HttpResponse {
		return &testmatchers.HttpResponse{
			StatusCode: http.StatusBadGateway,
			Body:       gomega.ContainSubstring("reset reason: protocol error"),
		}
	}, host)
}
