//go:build e2e

package a2a

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, map[string]*base.TestCase{}),
	}
}

func (s *testingSuite) TestA2AAgentCard() {
	s.T().Log("Testing A2A agent card discovery")
	s.waitA2AEnvironmentReady()

	headers := a2aHeaders()
	out, err := s.execCurlA2A(8080, "/.well-known/agent-card.json", headers, "", "--max-time", "5")
	s.Require().NoError(err, "agent card curl failed")

	// Without -v, output is just the JSON response
	var card A2AAgentCard
	s.Require().NoError(json.Unmarshal([]byte(strings.TrimSpace(out)), &card), "failed to parse agent card")

	s.Require().Equal("Hello World Agent", card.Name)
	s.Require().Equal(a2aProto, card.ProtocolVersion)
	s.Require().Equal("JSONRPC", card.PreferredTransport)
	s.Require().GreaterOrEqual(len(card.Skills), 1, "expected at least one skill")

	s.T().Logf("Agent card: %s v%s with %d skills", card.Name, card.Version, len(card.Skills))
}

func (s *testingSuite) TestA2AMessageSend() {
	s.T().Log("Testing A2A message/send")
	s.waitA2AEnvironmentReady()

	request := buildMessageSendRequest("hello", "test-123")
	headers := a2aHeaders()

	out, err := s.execCurlA2A(8080, "/", headers, request, "--max-time", "10")
	s.Require().NoError(err, "message/send curl failed")

	var resp A2AMessageResponse
	s.Require().NoError(json.Unmarshal([]byte(strings.TrimSpace(out)), &resp), "failed to parse response")

	s.Require().Nil(resp.Error, "unexpected error in response")
	s.Require().NotNil(resp.Result, "missing result")
	s.Require().Equal("message", resp.Result.Kind)
	s.Require().Equal("agent", resp.Result.Role)
	s.Require().GreaterOrEqual(len(resp.Result.Parts), 1)

	s.T().Logf("Response: %s", resp.Result.Parts[0].Text)
}

func (s *testingSuite) TestA2AHelloWorld() {
	s.T().Log("Testing A2A Hello World skill")
	s.waitA2AEnvironmentReady()

	request := buildMessageSendRequest("hello world", "test-hello")
	headers := a2aHeaders()

	out, err := s.execCurlA2A(8080, "/", headers, request, "--max-time", "10")
	s.Require().NoError(err, "hello world curl failed")

	var resp A2AMessageResponse
	s.Require().NoError(json.Unmarshal([]byte(strings.TrimSpace(out)), &resp), "failed to parse response")

	s.Require().Nil(resp.Error)
	s.Require().NotNil(resp.Result)
	s.Require().Contains(resp.Result.Parts[0].Text, "Hello World", "expected Hello World in response")
}

func (s *testingSuite) waitA2AEnvironmentReady() {
	s.TestInstallation.Assertions.EventuallyPodsRunning(
		s.Ctx, "default",
		metav1.ListOptions{LabelSelector: "app=a2a-helloworld"},
	)
	s.TestInstallation.Assertions.EventuallyPodsRunning(
		s.Ctx, curlPodNamespace,
		metav1.ListOptions{LabelSelector: defaults.WellKnownAppLabel + "=curl"},
	)
	s.TestInstallation.Assertions.EventuallyGatewayCondition(
		s.Ctx, gatewayName, gatewayNamespace,
		gwv1.GatewayConditionProgrammed, metav1.ConditionTrue,
	)
	s.TestInstallation.Assertions.EventuallyPodsRunning(
		s.Ctx, gatewayNamespace,
		metav1.ListOptions{LabelSelector: defaults.WellKnownAppLabel + "=" + gatewayName},
	)
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx, "a2a-route", "default",
		gwv1.RouteConditionAccepted, metav1.ConditionTrue,
	)
}
