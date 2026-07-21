//go:build e2e

package policyselector

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type tsuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	// the global-cors TrafficPolicy lives in the Settings.GlobalPolicyNamespace,
	// which is the install namespace in the e2e setup
	setup := base.TestCase{
		ManifestsWithTransform: map[string]func(string) string{
			labelSelectorManifest: func(content string) string {
				return strings.ReplaceAll(content, "$INSTALL_NAMESPACE", testInst.Metadata.InstallNamespace)
			},
		},
	}
	return &tsuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, nil),
	}
}

func (s *tsuite) TestLabelSelector() {
	// Verify response transformation with TrafficPolicy
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Headers:    map[string]any{"x-foo": "bar"},
		},
		curl.WithPath("/get"),
		curl.WithPort(gatewayPort),
	)

	// Verify access logs with HTTPListenerPolicy
	pods, err := s.TestInstallation.Actions.Kubectl().GetPodsInNsWithLabel(
		s.Ctx, "kgateway-base", fmt.Sprintf("%s=%s", defaults.WellKnownAppLabel, "gateway"),
	)
	s.Require().NoError(err)
	s.Require().Len(pods, 1)
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.TestInstallation.Actions.Kubectl().GetContainerLogs(s.Ctx, "kgateway-base", pods[0])
		s.Require().NoError(err)
		// Verify the log contains the expected JSON pattern
		assert.Contains(c, logs, `"method":"GET"`)
		assert.Contains(c, logs, `"path":"/get"`)
		assert.Contains(c, logs, `"protocol":"HTTP/1.1"`)
		assert.Contains(c, logs, `"response_code":200`)
		assert.Contains(c, logs, `"backendCluster":"kube_kgateway-base_backend_80"`)
	}, 30*time.Second, 100*time.Millisecond)
}

func (s *tsuite) TestGlobalPolicy() {
	requestHeaders := map[string]string{
		"Origin":                        "https://example.com",
		"Access-Control-Request-Method": "GET",
	}
	wantResponseHeaders := map[string]any{
		"Access-Control-Allow-Origin":  "https://example.com",
		"Access-Control-Allow-Methods": "GET, POST, DELETE",
		"Access-Control-Allow-Headers": "x-custom-header",
	}

	// Verify cors policy defined in Settings.GlobalPolicyNamespace (kgateway-system) is applied
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Headers:    wantResponseHeaders,
		},
		curl.WithPath("/get"),
		curl.WithPort(gatewayPort),
		curl.WithHeaders(requestHeaders),
		curl.WithMethod(http.MethodOptions),
	)
}
