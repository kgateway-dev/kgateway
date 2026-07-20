//go:build e2e

package requestmirror

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, nil),
	}
}

func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()

	// Wait for the sink access-log policy and the mirror routes to be programmed before sending.
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPListenerPolicyCondition(
		s.Ctx, "mirror-sink-logs", "kgateway-base", gwv1.GatewayConditionAccepted, metav1.ConditionTrue)
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx, "mirror-keep-host", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue)
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx, "mirror-default-host", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue)
}

// TestMirrorPreservesHostWhenDisabled verifies that with disableShadowHostSuffixAppend=true the
// mirror receives the original :authority, with no "-shadow" suffix.
func (s *testingSuite) TestMirrorPreservesHostWhenDisabled() {
	s.assertMirroredAuthority("/anything/keep-host", `"authority":"mirror.example.com"`)
}

// TestMirrorAppendsShadowByDefault verifies the default (no policy) still appends "-shadow".
func (s *testingSuite) TestMirrorAppendsShadowByDefault() {
	s.assertMirroredAuthority("/anything/default-host", `"authority":"mirror.example.com-shadow"`)
}

// assertMirroredAuthority sends a request to path and asserts the mirror-sink log records
// wantAuthority. Mirroring is fire-and-forget, so the request is re-sent each poll until the log
// records it.
func (s *testingSuite) assertMirroredAuthority(path, wantAuthority string) {
	pods := s.mirrorSinkPods()

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		common.BaseGateway.Send(
			s.T(),
			&matchers.HttpResponse{StatusCode: http.StatusOK},
			curl.WithHostHeader("mirror.example.com"),
			curl.WithPath(path),
			curl.WithPort(80),
		)

		logs, err := s.TestInstallation.Actions.Kubectl().GetContainerLogs(
			s.Ctx, mirrorSinkObjectMeta.GetNamespace(), pods[0])
		assert.NoError(c, err)
		assert.Contains(c, logs, fmt.Sprintf(`"path":"%s"`, path))
		assert.Contains(c, logs, wantAuthority)
	}, 30*time.Second, 1*time.Second)
}

func (s *testingSuite) mirrorSinkPods() []string {
	label := fmt.Sprintf("%s=%s", defaults.WellKnownAppLabel, mirrorSinkObjectMeta.GetName())
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(
		s.Ctx, mirrorSinkObjectMeta.GetNamespace(), metav1.ListOptions{LabelSelector: label})
	pods, err := s.TestInstallation.Actions.Kubectl().GetPodsInNsWithLabel(
		s.Ctx, mirrorSinkObjectMeta.GetNamespace(), label)
	s.Require().NoError(err)
	s.Require().NotEmpty(pods)
	return pods
}
