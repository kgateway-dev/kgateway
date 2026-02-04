//go:build e2e

package agentgateway

import (
	"context"
	"net/http"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	// This suite applies TrafficPolicy to specific named sections of the HTTPRoute, and requires HTTPRoutes.spec.rules[].name to be present in the Gateway API version.
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, base.TestCase{}, testCases, base.WithMinGwApiVersion(base.GwApiRequireRouteNames)),
	}
}

func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()
}

func (s *testingSuite) TestAgentgatewayTCPRoute() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.Ctx,
		gateway.Name,
		gateway.Namespace,
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
	)
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.Ctx,
		gateway.Name,
		gateway.Namespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionTrue,
	)
	// This assertion verifies that the TCP listener on the gateway has exactly 1 route attached to it.
	// It checks that the "tcp" listener in the "gateway" resource within the "agentgateway-base" namespace
	// has successfully attached the expected number of TCPRoutes (in this case, 1).
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayListenerAttachedRoutes(
		s.Ctx,
		gateway.Name,
		gateway.Namespace,
		"tcp",
		1,
	)

	// Wait for agentgateway pods to be running before sending traffic
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(
		s.Ctx,
		gateway.Namespace,
		metav1.ListOptions{
			LabelSelector: "gateway.networking.k8s.io/gateway-name=" + gateway.Name,
		},
	)

	// Send HTTP request to the TCP gateway using port 8080
	common.BaseGateway.Send(
		s.T(),
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
		},
		curl.WithPort(8080),
	)
}

func (s *testingSuite) TestAgentgatewayHTTPRoute() {
	// Verify the shared gateway has HTTP listener
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.Ctx,
		gateway.Name,
		gateway.Namespace,
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
	)
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayCondition(
		s.Ctx,
		gateway.Name,
		gateway.Namespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionTrue,
	)
	s.TestInstallation.AssertionsT(s.T()).EventuallyGatewayListenerAttachedRoutes(
		s.Ctx,
		gateway.Name,
		gateway.Namespace,
		"http",
		1,
	)

	// Wait for agentgateway pods to be running before sending traffic
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(
		s.Ctx,
		gateway.Namespace,
		metav1.ListOptions{
			LabelSelector: "gateway.networking.k8s.io/gateway-name=" + gateway.Name,
		},
	)

	// Send HTTP request to the shared backend service
	common.BaseGateway.Send(
		s.T(),
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
		},
		curl.WithHostHeader("www.example.com"),
		curl.WithPort(80),
	)
}
