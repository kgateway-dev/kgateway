package multiinstall

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type tsuite struct {
	suite.Suite

	ctx context.Context
	// ti contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	ti *e2e.TestInstallation

	namespace string
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &tsuite{
		ctx:       ctx,
		ti:        testInst,
		namespace: testInst.Metadata.InstallNamespace,
	}
}

func (s *tsuite) BeforeTest(suiteName, testName string) {
}

func (s *tsuite) AfterTest(suiteName, testName string) {
}

func (s *tsuite) TestPolicies() {
	// Verify response transformation with TrafficPolicy
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{curl.WithHostPort(ProxyHostPort(s.namespace)), curl.WithPath("/get")},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Headers: map[string]any{"x-foo": "bar"}})

	// Verify access logs with HTTPListenerPolicy
	pods, err := s.ti.Actions.Kubectl().GetPodsInNsWithLabel(
		s.ctx, s.namespace, fmt.Sprintf("app.kubernetes.io/name=%s", Gateway(s.namespace).Name),
	)
	s.Require().NoError(err)
	s.Require().Len(pods, 1)
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.ti.Actions.Kubectl().GetContainerLogs(s.ctx, s.namespace, pods[0])
		s.Require().NoError(err)
		// Verify the log contains the expected JSON pattern
		assert.Contains(c, logs, `"method":"GET"`)
		assert.Contains(c, logs, `"path":"/get"`)
		assert.Contains(c, logs, `"protocol":"HTTP/1.1"`)
		assert.Contains(c, logs, `"response_code":200`)
		assert.Contains(c, logs, fmt.Sprintf(`"backendCluster":"kube_%s_httpbin_8000"`, s.namespace))
	}, 5*time.Second, 100*time.Millisecond)
}
