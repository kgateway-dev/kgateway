package policyselector

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

	// maps test name to a list of manifests to apply before the test
	manifests map[string][]string

	manifestObjects map[string][]client.Object
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &tsuite{
		ctx: ctx,
		ti:  testInst,
	}
}

func (s *tsuite) SetupSuite() {
	s.manifests = map[string][]string{
		"TestLabelSelector": {labelSelectorManifest, defaults.CurlPodManifest},
	}
	// Not every resource that is applied needs to be verified. We are not testing `kubectl apply`,
	// but the below code demonstrates how it can be done if necessary
	s.manifestObjects = map[string][]client.Object{
		labelSelectorManifest:    {gateway, httpbinRoute, httpbinDeployment},
		defaults.CurlPodManifest: {defaults.CurlPod},
	}
}

func (s *tsuite) TearDownSuite() {
}

func (s *tsuite) BeforeTest(suiteName, testName string) {
	manifests := s.manifests[testName]
	for _, manifest := range manifests {
		err := s.ti.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
		s.ti.Assertions.EventuallyObjectsExist(s.ctx, s.manifestObjects[manifest]...)
	}
}

func (s *tsuite) AfterTest(suiteName, testName string) {
	manifests := s.manifests[testName]
	for _, manifest := range manifests {
		err := s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err)
		s.ti.Assertions.EventuallyObjectsNotExist(s.ctx, s.manifestObjects[manifest]...)
	}
}

func (s *tsuite) TestLabelSelector() {
	// Verify response transformation with TrafficPolicy
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{curl.WithHostPort(proxyHostPort), curl.WithPath("/get")},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK, Headers: map[string]any{"x-foo": "bar"}})

	// Verify access logs with HTTPListenerPolicy
	pods, err := s.ti.Actions.Kubectl().GetPodsInNsWithLabel(
		s.ctx, gateway.Namespace, fmt.Sprintf("app.kubernetes.io/name=%s", gateway.Name),
	)
	s.Require().NoError(err)
	s.Require().Len(pods, 1)
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.ti.Actions.Kubectl().GetContainerLogs(s.ctx, gateway.Namespace, pods[0])
		s.Require().NoError(err)
		// Verify the log contains the expected JSON pattern
		assert.Contains(c, logs, `"method":"GET"`)
		assert.Contains(c, logs, `"path":"/get"`)
		assert.Contains(c, logs, `"protocol":"HTTP/1.1"`)
		assert.Contains(c, logs, `"response_code":200`)
		assert.Contains(c, logs, `"backendCluster":"kube_default_httpbin_8000"`)
	}, 5*time.Second, 100*time.Millisecond)
}
