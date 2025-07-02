package extproc

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/transforms"
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
		extProcService, extProcDeployment, // ext-proc service
		backendService, backendDeployment, // backend service
		gatewayService, gatewayDeployment, // gateway service
	}

	// Wait for core infrastructure to be ready
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, s.activeObjects...)
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, extProcDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=ext-proc-grpc",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, backendDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=backend-0",
	})
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

// TestExtProcWithGatewayTargetRef tests ExtProc with targetRef to Gateway
func (s *testingSuite) TestExtProcWithGatewayTargetRef() {
	s.activeManifests = []string{gatewayTargetRefManifest}
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, gatewayTargetRefManifest)
	s.Require().NoError(err)

	// Test that ExtProc is applied to all routes through the Gateway
	// First route - should have ExtProc applied
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPort(8080),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproctest": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("Extproctest")),
				),
			),
		})

	// Second route - should also have ExtProc applied
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/myapp"),
			curl.WithPort(8080),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproctest": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("Extproctest")),
				),
			),
		})
}

// TestExtProcWithHTTPRouteTargetRef tests ExtProc with targetRef to HTTPRoute
func (s *testingSuite) TestExtProcWithHTTPRouteTargetRef() {
	s.activeManifests = []string{httpRouteTargetRefManifest}
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, httpRouteTargetRefManifest)
	s.Require().NoError(err)

	// Test route with ExtProc - should have header modified
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/myapp"),
			curl.WithPort(8080),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproctest": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("Extproctest")),
				),
			),
		})

	// Test route without ExtProc - should not have header modified
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPort(8080),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproctest": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.Not(gomega.HaveKey("Extproctest"))),
				),
			),
		})
}

// TestExtProcWithSingleRoute tests ExtProc applied to a specific rule within a route
func (s *testingSuite) TestExtProcWithSingleRoute() {
	s.activeManifests = []string{singleRouteManifest}
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, singleRouteManifest)
	s.Require().NoError(err)

	// TODO: Should header-based routing work?

	// Test route with ExtProc and matching header - should have header modified
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/myapp"),
			// curl.WithHeader("x-test", "true"),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproctest": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("Extproctest")),
				),
			),
		})

	// Test second rule - should not have header modified
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPort(8080),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproctest": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.Not(gomega.HaveKey("Extproctest"))),
				),
			),
		})
}

// TestExtProcWithBackendFilter tests backend-level ExtProc filtering
func (s *testingSuite) TestExtProcWithBackendFilter() {
	// Apply the backend filter test manifests
	s.activeManifests = []string{backendFilterManifest}
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, backendFilterManifest)
	s.Require().NoError(err)

	// Test path with ExtProc enabled
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/with-extproc"),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproctest": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.HaveKey("Extproctest")),
				),
			),
		})

	// Test path without ExtProc
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
			curl.VerboseOutput(),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/without-extproc"),
			curl.WithHeader("instructions", getInstructionsJson(instructions{
				AddHeaders: map[string]string{"extproctest": "true"},
			})),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body: gomega.WithTransform(transforms.WithJsonBody(),
				gomega.And(
					gomega.HaveKeyWithValue("headers", gomega.Not(gomega.HaveKey("Extproctest"))),
				),
			),
		})
}

// The instructions format that the example extproc service understands.
// See the `basic-sink` example in https://github.com/solo-io/ext-proc-examples
type instructions struct {
	// Header key/value pairs to add to the request or response.
	AddHeaders map[string]string `json:"addHeaders"`
	// Header keys to remove from the request or response.
	RemoveHeaders []string `json:"removeHeaders"`
}

func getInstructionsJson(instr instructions) string {
	bytes, _ := json.Marshal(instr)
	return string(bytes)
}
