package acesslog

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of tests for external processing functionality
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of kgateway
	testInstallation *e2e.TestInstallation

	// maps test name to a list of manifests to apply before the test
	manifests map[string][]string

	// maps manifest name to a list of objects to verify
	manifestObjects map[string][]client.Object

	// Track core objects for cleanup
	coreObjects []client.Object
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

// SetupSuite runs before all tests in the suite
func (s *testingSuite) SetupSuite() {
	// Initialize test manifest mappings
	s.manifests = map[string][]string{
		"TestAccessLogWithFileSink": {fileSinkManifest},
		"TestAccessLogWithGrpcSink": {grpcServiceManifest},
	}

	// Initialize manifest to objects mapping
	s.manifestObjects = map[string][]client.Object{
		fileSinkManifest:    {fileSinkConfig},
		grpcServiceManifest: {accessLoggerService, accessLoggerDeployment},
	}

	// Apply core infrastructure
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.Require().NoError(err)

	// Apply curl pod for testing
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.CurlPodManifest)
	s.Require().NoError(err)

	// Track core objects
	s.coreObjects = []client.Object{
		testdefaults.CurlPod,              // curl
		httpbinDeployment,                 // httpbin
		gatewayService, gatewayDeployment, // gateway service
	}

	// Wait for core infrastructure to be ready
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, s.coreObjects...)
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, httpbinDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=httpbin",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(
		s.ctx,
		gatewayDeployment.ObjectMeta.GetNamespace(),
		metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", gatewayObjectMeta.GetName()),
		},
	)
	s.testInstallation.Assertions.EventuallyHTTPRouteCondition(s.ctx, "httpbin", "httpbin", gwv1.RouteConditionAccepted, metav1.ConditionTrue)
}

// TearDownSuite cleans up any remaining resources
func (s *testingSuite) TearDownSuite() {
	// Clean up core infrastructure
	err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, setupManifest)
	s.Require().NoError(err)

	// Clean up curl pod
	err = s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, testdefaults.CurlPodManifest)
	s.Require().NoError(err)

	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, s.coreObjects...)
	s.testInstallation.Assertions.EventuallyPodsNotExist(s.ctx, gatewayObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", gatewayObjectMeta.GetName()),
	})
	s.testInstallation.Assertions.EventuallyPodsNotExist(s.ctx, httpbinObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=httpbin",
	})
}

// BeforeTest runs before each test
func (s *testingSuite) BeforeTest(suiteName, testName string) {
	manifests := s.manifests[testName]
	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
		s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, s.manifestObjects[manifest]...)
	}
}

// AfterTest runs after each test
func (s *testingSuite) AfterTest(suiteName, testName string) {
	manifests := s.manifests[testName]
	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err)
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, s.manifestObjects[manifest]...)
	}
}

// TestAccessLogWithFileSink tests access log with file sink
func (s *testingSuite) TestAccessLogWithFileSink() {
	// check access log
	pods, err := s.testInstallation.Actions.Kubectl().GetPodsInNsWithLabel(
		s.ctx,
		gatewayService.ObjectMeta.GetNamespace(),
		fmt.Sprintf("app.kubernetes.io/name=%s", gatewayObjectMeta.GetName()),
	)
	s.Require().NoError(err)
	s.Require().Len(pods, 1)

	s.sendTestRequest()

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.testInstallation.Actions.Kubectl().GetContainerLogs(s.ctx, gatewayService.ObjectMeta.GetNamespace(), pods[0])
		s.Require().NoError(err)

		// Verify the log contains the expected JSON pattern
		assert.Contains(c, logs, `"authority":"www.example.com"`)
		assert.Contains(c, logs, `"method":"GET"`)
		assert.Contains(c, logs, `"path":"/status/200"`)
		assert.Contains(c, logs, `"protocol":"HTTP/1.1"`)
		assert.Contains(c, logs, `"response_code":200`)
		assert.Contains(c, logs, `"backendCluster":"kube_httpbin_httpbin_8000"`)
	}, 5*time.Second, 100*time.Millisecond)
}

// TestAccessLogWithGrpcSink tests access log with grpc sink
func (s *testingSuite) TestAccessLogWithGrpcSink() {
	s.testInstallation.Assertions.EventuallyPodsRunning(
		s.ctx,
		accessLoggerDeployment.ObjectMeta.GetNamespace(),
		metav1.ListOptions{
			LabelSelector: "kgateway=gateway-proxy-access-logger",
		},
	)
	// check access log
	pods, err := s.testInstallation.Actions.Kubectl().GetPodsInNsWithLabel(
		s.ctx,
		accessLoggerDeployment.ObjectMeta.GetNamespace(),
		"kgateway=gateway-proxy-access-logger",
	)
	s.Require().NoError(err)
	s.Require().Len(pods, 1)

	s.sendTestRequest()

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		logs, err := s.testInstallation.Actions.Kubectl().GetContainerLogs(s.ctx, accessLoggerDeployment.ObjectMeta.GetNamespace(), pods[0])
		s.Require().NoError(err)

		// Verify the log contains the expected JSON pattern
		assert.Contains(c, logs, `"logger_name":"test-accesslog-service"`)
		assert.Contains(c, logs, `"cluster":"kube_httpbin_httpbin_8000"`)
	}, 5*time.Second, 100*time.Millisecond)
}

func (s *testingSuite) sendTestRequest() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/status/200"),
			curl.WithPort(8080),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
		},
	)
}
