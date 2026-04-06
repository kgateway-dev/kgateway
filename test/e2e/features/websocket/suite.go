//go:build e2e

package websocket

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/websocket"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	envoyadmincli "github.com/kgateway-dev/kgateway/v2/test/envoyutils/admincli"
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

func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()

	s.assertPodsRunning()
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
		result, err := websocket.Dial(wsURL, host, dialTimeout, nil, true)
		ig.Expect(err).NotTo(gomega.HaveOccurred(), "WebSocket dial failed for host %s", host)
		msg = result
	}).WithTimeout(eventualTimeout).WithPolling(pollInterval).Should(gomega.Succeed())
	return msg
}

// TestWebSocketHappyPath verifies that a WebSocket upgrade succeeds through a
// route with no transformation policies.
func (s *testingSuite) TestWebSocketHappyPath() {
	g := gomega.NewWithT(s.T())
	s.assertWebsocketUpgradeEnabled()
	msg := s.dialWebSocket(g, "websocket.example.com")
	g.Expect(msg).To(gomega.Equal("websocket-e2e-ping"),
		"echo-server should echo back the test payload")
}

// Test websocket will ignore explicit body transformation
func (s *testingSuite) TestWebSocketWithBodyTransformation() {
	g := gomega.NewWithT(s.T())
	s.assertWebsocketUpgradeEnabled()
	msg := s.dialWebSocket(g, "websocket-body-transform.example.com")
	g.Expect(msg).To(gomega.Equal("websocket-e2e-ping"),
		"echo-server should echo back the test payload; "+
			"if this hangs/times out the Envoy is buffering the body")
}

// Test websocket will work with default transformation buffering behavior
func (s *testingSuite) TestWebSocketWithDefaultTransformationBuffering() {
	g := gomega.NewWithT(s.T())
	s.assertWebsocketUpgradeEnabled()
	msg := s.dialWebSocket(g, "websocket-default-transform.example.com")
	g.Expect(msg).To(gomega.Equal("websocket-e2e-ping"),
		"echo-server should echo back the test payload; "+
			"if this hangs/times out the Envoy is buffering the body")
}

func (s *testingSuite) assertWebsocketUpgradeEnabled() {
	proxyObjectMeta := metav1.ObjectMeta{
		Name:      common.BaseGateway.Name,
		Namespace: common.BaseGateway.Namespace,
	}

	s.TestInstallation.AssertionsT(s.T()).AssertEnvoyAdminApi(
		s.Ctx,
		proxyObjectMeta,
		func(ctx context.Context, adminClient *envoyadmincli.Client) {
			s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
				listener, err := adminClient.GetSingleListenerFromDynamicListeners(ctx, "listener~80")
				g.Expect(err).ToNot(gomega.HaveOccurred(), "failed to get listener")
				if err != nil {
					// when we get an error, the g.Expect() doesn't stop execution of this function
					// but listener is nil and will cause a crash, Gomega will catch it and swallow it but
					// will not retry even with Eventually()
					return
				}

				websocketEnabled := strings.Contains(listener.String(), "upgrade_configs:{upgrade_type:\"websocket\"}")
				g.Expect(websocketEnabled).To(gomega.BeTrue(), fmt.Sprintf("%v", listener.String()))
			}).
				WithContext(ctx).
				WithTimeout(30*time.Second).
				WithPolling(2*time.Second).
				Should(gomega.Succeed(), "failed to get expected websocket enabled")
		},
	)
}

func (s *testingSuite) assertPodsRunning() {
	// control-plane
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx, s.TestInstallation.Metadata.InstallNamespace, metav1.ListOptions{
		LabelSelector: defaults.ControllerLabelSelector,
	})

	// envoy proxy
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx, common.BaseGateway.Namespace, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", defaults.WellKnownAppLabel, common.BaseGateway.Name),
	})

	// websocket server backend
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx, "kgateway-base", metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=websocket-backend", defaults.WellKnownAppLabel),
	})
}
