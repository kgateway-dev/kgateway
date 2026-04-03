//go:build e2e

package websocket

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/websocket"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

const (
	dialTimeout     = 5 * time.Second
	eventualTimeout = 30 * time.Second
	pollInterval    = 2 * time.Second
)

// dialWebSocket dials a WebSocket connection through the base gateway with the
// given Host header. It retries until success or the eventualTimeout is reached,
// which accounts for the HTTPListenerPolicy needing time to be reconciled and
// pushed to Envoy as xDS config.
func (s *testingSuite) dialWebSocket(g gomega.Gomega, host string) string {
	wsURL := fmt.Sprintf("ws://%s:%d/", common.BaseGateway.Address, 80)

	var msg string
	g.Eventually(func(ig gomega.Gomega) {
		result, err := websocket.Dial(wsURL, host, dialTimeout, nil)
		ig.Expect(err).NotTo(gomega.HaveOccurred(), "WebSocket dial failed for host %s", host)
		msg = result
	}).WithTimeout(eventualTimeout).WithPolling(pollInterval).Should(gomega.Succeed())
	return msg
}

// TestWebSocketHappyPath verifies that a WebSocket upgrade succeeds through a
// route with no transformation policies.
func (s *testingSuite) TestWebSocketHappyPath() {
	g := gomega.NewWithT(s.T())
	msg := s.dialWebSocket(g, "websocket.example.com")
	g.Expect(msg).To(gomega.Equal("websocket-e2e-ping"),
		"echo-server should echo back the test payload")
}

// Test websocket will ignore explicit body transformation
func (s *testingSuite) TestWebSocketWithBodyTransformation() {
	g := gomega.NewWithT(s.T())
	msg := s.dialWebSocket(g, "websocket-body-transform.example.com")
	g.Expect(msg).To(gomega.Equal("websocket-e2e-ping"),
		"echo-server should echo back the test payload; "+
			"if this hangs/times out the Envoy is buffing the body")
}

// Test websocket will work with default transfomration buffering behavior
func (s *testingSuite) TestWebSocketWithDefaultTransformationBuffering() {
	g := gomega.NewWithT(s.T())
	msg := s.dialWebSocket(g, "websocket-default-transform.example.com")
	g.Expect(msg).To(gomega.Equal("websocket-e2e-ping"),
		"echo-server should echo back the test payload; "+
			"if this hangs/times out the Envoy is buffing the body")
}
