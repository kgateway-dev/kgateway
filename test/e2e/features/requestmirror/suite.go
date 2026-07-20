//go:build e2e

package requestmirror

import (
	"context"
	"fmt"
	"net/http"
	"strings"
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

	a := s.TestInstallation.AssertionsT(s.T())
	// Gate on the mirror-sink being fully ready before sending. A freshly deployed sink that only
	// reports Accepted may not be serving or logging yet, so wait for its Envoy Programmed condition,
	// its route accepted, and its pods running.
	a.EventuallyGatewayCondition(s.Ctx, "mirror-sink", "kgateway-base", gwv1.GatewayConditionProgrammed, metav1.ConditionTrue)
	a.EventuallyHTTPRouteCondition(s.Ctx, "mirror-sink-route", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue)
	a.EventuallyHTTPListenerPolicyCondition(s.Ctx, "mirror-sink-logs", "kgateway-base", gwv1.GatewayConditionAccepted, metav1.ConditionTrue)
	a.EventuallyPodsRunning(s.Ctx, mirrorSinkObjectMeta.GetNamespace(), metav1.ListOptions{LabelSelector: s.mirrorSinkLabel()})
	a.EventuallyHTTPRouteCondition(s.Ctx, "mirror-keep-host", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue)
	a.EventuallyHTTPRouteCondition(s.Ctx, "mirror-default-host", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue)
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

// assertMirroredAuthority requires one mirror-sink access-log record with both the mirrored path and
// wantAuthority, so unrelated records can't satisfy the two matches separately. Mirroring is
// fire-and-forget, so each poll re-sends the request and re-lists the sink pods (which can be
// replaced).
func (s *testingSuite) assertMirroredAuthority(path, wantAuthority string) {
	wantPath := fmt.Sprintf(`"path":"%s"`, path)

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		common.BaseGateway.Send(
			s.T(),
			&matchers.HttpResponse{StatusCode: http.StatusOK},
			curl.WithHostHeader("mirror.example.com"),
			curl.WithPath(path),
			curl.WithPort(80),
		)

		pods, err := s.TestInstallation.Actions.Kubectl().GetPodsInNsWithLabel(
			s.Ctx, mirrorSinkObjectMeta.GetNamespace(), s.mirrorSinkLabel())
		assert.NoError(c, err)

		var found bool
		for _, pod := range pods {
			logs, err := s.TestInstallation.Actions.Kubectl().GetContainerLogs(
				s.Ctx, mirrorSinkObjectMeta.GetNamespace(), pod)
			assert.NoError(c, err)
			for line := range strings.SplitSeq(logs, "\n") {
				if strings.Contains(line, wantPath) && strings.Contains(line, wantAuthority) {
					found = true
				}
			}
		}
		assert.True(c, found, "want an access-log record with %s and %s", wantPath, wantAuthority)
	}, 90*time.Second, 2*time.Second)
}

func (s *testingSuite) mirrorSinkLabel() string {
	return fmt.Sprintf("%s=%s", defaults.WellKnownAppLabel, mirrorSinkObjectMeta.GetName())
}
