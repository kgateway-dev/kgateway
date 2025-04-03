package acesslog

import (
	"context"
	"net/http"
	"time"

	// . "github.com/onsi/gomega"

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

// TODO(tim): manifest mapping
// TODO(tim): validate the GW pod is ready before running tests

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of tests for external processing functionality
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of kgateway
	testInstallation *e2e.TestInstallation

	// Track active manifests and objects for cleanup
	activeManifests []string
	activeObjects   []client.Object
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

// SetupSuite runs before all tests in the suite
func (s *testingSuite) SetupSuite() {
	// Apply core infrastructure
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.Require().NoError(err)

	// Apply curl pod for testing
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.CurlPodManifest)
	s.Require().NoError(err)

	// Track core objects
	s.activeObjects = []client.Object{
		testdefaults.CurlPod,              // curl
		httpbinDeployment,                 // httpbin
		gatewayService, gatewayDeployment, // gateway service
	}

	// Wait for core infrastructure to be ready
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, s.activeObjects...)
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, httpbinDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=httpbin",
	})
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

	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, s.activeObjects...)
}

// SetupTest runs before each test
func (s *testingSuite) SetupTest() {
	// Reset active manifests tracking
	s.activeManifests = nil
}

// TearDownTest runs after each test
func (s *testingSuite) TearDownTest() {
	// Clean up any test-specific manifests
	for _, manifest := range s.activeManifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err)
	}
}

// // TestAccessLogWithFileSink tests access log with file sink
// func (s *testingSuite) TestAccessLogWithFileSink() {
// 	s.activeManifests = []string{fileSinkManifest}
// 	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, fileSinkManifest)
// 	s.Require().NoError(err)

// 	// check access log
// 	pods, err := s.testInstallation.Actions.Kubectl().GetPodsInNsWithLabel(
// 		s.ctx,
// 		gatewayService.ObjectMeta.GetNamespace(),
// 		"gateway.networking.k8s.io/gateway-name=gw",
// 	)
// 	s.Require().NoError(err)
// 	s.Require().Len(pods, 1)

// 	s.testInstallation.Assertions.AssertEventualCurlResponse(
// 		s.ctx,
// 		testdefaults.CurlPodExecOpt,
// 		[]curl.Option{
// 			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
// 			curl.VerboseOutput(),
// 			curl.WithHostHeader("www.example.com"),
// 			curl.WithPath("/status/200"),
// 			curl.WithPort(8080),
// 		},
// 		&matchers.HttpResponse{
// 			StatusCode: http.StatusOK,
// 		},
// 	)

// 	s.Require().EventuallyWithT(func(c *assert.CollectT) {
// 		// curl httpbin
// 		s.testInstallation.Assertions.AssertEventualCurlResponse(
// 			s.ctx,
// 			testdefaults.CurlPodExecOpt,
// 			[]curl.Option{
// 				curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
// 				curl.VerboseOutput(),
// 				curl.WithHostHeader("www.example.com"),
// 				curl.WithPath("/status/200"),
// 				curl.WithPort(8080),
// 			},
// 			&matchers.HttpResponse{
// 				StatusCode: http.StatusOK,
// 			},
// 		)
// 		logs, err := s.testInstallation.Actions.Kubectl().GetContainerLogs(s.ctx, gatewayService.ObjectMeta.GetNamespace(), pods[0])
// 		s.Require().NoError(err)

// 		// Verify the log contains the expected JSON pattern
// 		assert.Contains(c, logs, `"authority":"www.example.com"`)
// 		assert.Contains(c, logs, `"method":"GET"`)
// 		assert.Contains(c, logs, `"path":"/status/200"`)
// 		assert.Contains(c, logs, `"protocol":"HTTP/1.1"`)
// 		assert.Contains(c, logs, `"response_code":200`)
// 		assert.Contains(c, logs, `"backendCluster":"kube_httpbin_httpbin_8000"`)
// 	}, 5*time.Second, 100*time.Millisecond)
// }

// TestAccessLogWithGrpcSink tests access log with grpc sink
func (s *testingSuite) TestAccessLogWithGrpcSink() {
	s.activeManifests = []string{grpcServiceManifest}
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, grpcServiceManifest)
	s.Require().NoError(err)

	// check access log
	pods, err := s.testInstallation.Actions.Kubectl().GetPodsInNsWithLabel(
		s.ctx,
		accessLoggerDeployment.ObjectMeta.GetNamespace(),
		"kgateway=gateway-proxy-access-logger",
	)
	s.Require().NoError(err)
	s.Require().Len(pods, 1)

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

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		// curl httpbin
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
		logs, err := s.testInstallation.Actions.Kubectl().GetContainerLogs(s.ctx, accessLoggerDeployment.ObjectMeta.GetNamespace(), pods[0])
		s.Require().NoError(err)

		// Verify the log contains the expected JSON pattern
		assert.Contains(c, logs, `"logger_name":"test-accesslog-service"`)
		assert.Contains(c, logs, `"cluster":"kube_httpbin_httpbin_8000"`)
	}, 5*time.Second, 100*time.Millisecond)
}
