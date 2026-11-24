//go:build e2e

package grpcweb

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	"istio.io/istio/pkg/test/util/retry"
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
)

// TestGRPCWebOnRoute enables gRPC-Web on one route and checks that a route without it is
// unaffected.
func (s *testingSuite) TestGRPCWebOnRoute() {
	s.waitForRoutes()

	s.T().Log("gRPC-Web request on the enabled route is converted to gRPC and back")
	s.assertGRPCWebResponse(s.T(), "grpc-web")

	s.T().Log("gRPC-Web request on a route without the policy is not converted")
	s.assertNotConverted(s.T(), "grpc-web-disabled")
}

// TestGRPCWebOnGatewayWithRouteDisable enables gRPC-Web on the Gateway and disables it on one
// route with a more specific policy.
func (s *testingSuite) TestGRPCWebOnGatewayWithRouteDisable() {
	s.waitForRoutes()

	s.T().Log("gRPC-Web request on a route covered by the Gateway policy is converted")
	s.assertGRPCWebResponse(s.T(), "grpc-web")

	s.T().Log("gRPC-Web request on the route that disables it is not converted")
	s.assertNotConverted(s.T(), "grpc-web-disabled")
}

func (s *testingSuite) waitForRoutes() {
	for _, route := range []string{"grpc-web", "grpc-web-disabled"} {
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

// assertGRPCWebResponse expects a gRPC-Web response: the "pong" message in a data frame and
// grpc-status 0 in the trailer frame that the gRPC-Web filter appends to the body.
func (s *testingSuite) assertGRPCWebResponse(t *testing.T, host string) {
	common.BaseGateway.Send(
		t,
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Headers: map[string]any{
				"Content-Type": gomega.HavePrefix("application/grpc-web"),
			},
			Body: gomega.And(
				gomega.ContainSubstring("pong"),
				gomega.ContainSubstring("grpc-status:0"),
			),
		},
		grpcWebRequest(host)...,
	)
}

// assertNotConverted expects the request to reach the backend as gRPC-Web, which a plain gRPC
// server rejects, so the response is not a gRPC-Web response. Only the absence of conversion is
// asserted, since how the server rejects the request is up to its gRPC library.
func (s *testingSuite) assertNotConverted(t *testing.T, host string) {
	retry.UntilSuccessOrFail(t, func() error {
		resp, err := common.BaseGateway.Do(t.Context(), grpcWebRequest(host)...)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "application/grpc-web") {
			return fmt.Errorf("expected no gRPC-Web conversion for host %s, got status %d with content-type %q", host, resp.StatusCode, ct)
		}
		return nil
	})
}
