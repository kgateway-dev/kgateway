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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	// During a rollout the old pod can linger, still Running, while it
	// terminates. Wait for exactly one live proxy pod so the logs we read come
	// from the pod that serves the request.
	var pod string
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		pods, err := s.TestInstallation.ClusterContext.Clientset.CoreV1().Pods("kgateway-base").List(s.Ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s", defaults.WellKnownAppLabel, "gateway"),
		})
		if !assert.NoError(c, err) || !assert.Len(c, pods.Items, 1, "expected exactly one gateway proxy pod") {
			return
		}
		if assert.Nil(c, pods.Items[0].DeletionTimestamp, "the gateway proxy pod should not be terminating") {
			pod = pods.Items[0].Name
		}
	}, 60*time.Second, 1*time.Second)

	// The response transformation comes from a TrafficPolicy on the route, while
	// the access log comes from a ListenerPolicy on the listener, and the two
	// reach Envoy independently. A single request sent once the transformation
	// is live can still be served by a listener without the access log, so send
	// inside the poll loop until the access log line shows up.
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		// Verify response transformation with TrafficPolicy
		err := common.BaseGateway.SendOnce(
			s.Ctx,
			&testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Headers:    map[string]any{"x-foo": "bar"},
			},
			curl.WithPath("/get"),
			curl.WithPort(gatewayPort),
		)
		if !assert.NoError(c, err) {
			return
		}

		// Verify access logs with ListenerPolicy
		logs, err := s.TestInstallation.Actions.Kubectl().GetContainerLogs(s.Ctx, "kgateway-base", pod)
		if !assert.NoError(c, err) {
			return
		}
		// Verify the log contains the expected JSON pattern
		assert.Contains(c, logs, `"method":"GET"`)
		assert.Contains(c, logs, `"path":"/get"`)
		assert.Contains(c, logs, `"protocol":"HTTP/1.1"`)
		assert.Contains(c, logs, `"response_code":200`)
		assert.Contains(c, logs, `"backendCluster":"kube_kgateway-base_backend_80"`)
	}, 30*time.Second, 1*time.Second)
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
