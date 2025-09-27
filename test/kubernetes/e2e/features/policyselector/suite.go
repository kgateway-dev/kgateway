package policyselector

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	commonManifests []string
	testManifests   map[string][]string

	manifestObjects map[string][]client.Object
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &tsuite{
		ctx:             ctx,
		ti:              testInst,
		commonManifests: []string{labelSelectorManifest, defaults.CurlPodManifest, defaults.HttpbinManifest},
		testManifests:   map[string][]string{},
	}
}

func (s *tsuite) SetupSuite() {
	for _, manifest := range s.commonManifests {
		content, err := os.ReadFile(manifest)
		s.Require().NoError(err, manifest)
		yamlStr := strings.ReplaceAll(string(content), "$INSTALL_NAMESPACE", s.ti.Metadata.InstallNamespace)

		out := new(bytes.Buffer)
		err = s.ti.Actions.Kubectl().WithReceiver(out).Apply(s.T().Context(), []byte(yamlStr))
		s.Require().NoErrorf(err, "manifest %s, out: %s", manifest, out.String())
	}

	s.ti.Assertions.EventuallyPodsRunning(s.ctx, defaults.CurlPod.Namespace, metav1.ListOptions{
		LabelSelector: defaults.CurlPodLabelSelector,
	})
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, defaults.HttpbinDeployment.Namespace, metav1.ListOptions{
		LabelSelector: defaults.HttpbinLabelSelector,
	})
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, gateway.Namespace, metav1.ListOptions{
		LabelSelector: defaults.WellKnownAppLabel + "=" + gateway.Name,
	})
}

func (s *tsuite) TearDownSuite() {
	for i := len(s.commonManifests) - 1; i >= 0; i-- {
		manifest := s.commonManifests[i]
		err := s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.NoError(err, manifest)
	}
}

func (s *tsuite) BeforeTest(suiteName, testName string) {
	for _, manifest := range s.testManifests[testName] {
		err := s.ti.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
		s.ti.Assertions.EventuallyObjectsExist(s.ctx, s.manifestObjects[manifest]...)
	}
}

func (s *tsuite) AfterTest(suiteName, testName string) {
	for _, manifest := range s.testManifests[testName] {
		err := s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.NoError(err)
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
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(s.ctx, defaults.CurlPodExecOpt,
		[]curl.Option{curl.WithHostPort(proxyHostPort), curl.WithPath("/get"), curl.WithHeaders(requestHeaders), curl.WithMethod(http.MethodOptions)},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Headers:    wantResponseHeaders,
		},
	)
}
