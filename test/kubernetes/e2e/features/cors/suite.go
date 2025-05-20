package cors

import (
	"context"
	"fmt"
	"net/http"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of basic routing / "happy path" tests
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of kgateway
	testInstallation *e2e.TestInstallation

	// manifests shared by all tests
	commonManifests []string
	// resources from manifests shared by all tests
	commonResources []client.Object
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	s.commonManifests = []string{
		testdefaults.CurlPodManifest,
		simpleServiceManifest,
		commonManifest,
	}
	s.commonResources = []client.Object{
		// resources from curl manifest
		testdefaults.CurlPod,
		// resources from service manifest
		simpleSvc, simpleDeployment,
		// resources from gateway manifest
		gateway,
		// deployer-generated resources
		proxyDeployment, proxyService, proxyServiceAccount,
	}

	// set up common resources once
	for _, manifest := range s.commonManifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err, "can apply "+manifest)
	}
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, s.commonResources...)

	// make sure pods are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, simpleDeployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=backend-0,version=v1",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", proxyObjectMeta.GetName()),
	})
}

func (s *testingSuite) TearDownSuite() {
	// clean up common resources
	for _, manifest := range s.commonManifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err, "can delete "+manifest)
	}
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, s.commonResources...)

	// make sure pods are gone
	s.testInstallation.Assertions.EventuallyPodsNotExist(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})
	s.testInstallation.Assertions.EventuallyPodsNotExist(s.ctx, simpleDeployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=backend-0,version=v1",
	})
	s.testInstallation.Assertions.EventuallyPodsNotExist(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", proxyObjectMeta.GetName()),
	})
}

// Test cors on specific route
func (s *testingSuite) TestCorsForRoute() {
	s.setupTest([]string{httpRoutesManifest, routeCorsManifest}, []client.Object{route, route2, routeCorsTrafficPolicy})
	requestHeaders := map[string]string{
		"Origin":                         "https://notexample.com",
		"Access-Control-Request-Method":  "POST",
		"Access-Control-Request-Headers": "x-custom-header",
	}

	expectedHeaders := map[string]any{
		"Access-Control-Allow-Origin":  "https://notexample.com",
		"Access-Control-Allow-Methods": "GET, POST, DELETE",
		"Access-Control-Allow-Headers": "x-custom-header",
	}

	// Verify that the route with cors is responding to the OPTIONS request with the expected cors headers
	s.assertResponse("/path1", http.StatusOK, requestHeaders, expectedHeaders, []string{})

	// Verify that the route without cors is not affected by the cors traffic policy
	s.assertResponse("/path2", http.StatusOK, requestHeaders, nil, []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers"})
}

// Test cors at the gateway level
func (s *testingSuite) TestCorsAtGatewayLevel() {
	s.setupTest([]string{httpRoutesManifest, gwCorsManifest}, []client.Object{route, route2, gwCorsTrafficPolicy})

	requestHeaders := map[string]string{
		"Origin":                         "https://notexample.com",
		"Access-Control-Request-Method":  "GET",
		"Access-Control-Request-Headers": "content-type",
	}

	expectedHeaders := map[string]any{
		"Access-Control-Allow-Origin":  "https://notexample.com",
		"Access-Control-Allow-Methods": "GET, POST",
		"Access-Control-Allow-Headers": "Content-Type, Authorization",
	}

	s.assertResponse("/path1", http.StatusOK, requestHeaders, expectedHeaders, []string{})
	s.assertResponse("/path2", http.StatusOK, requestHeaders, expectedHeaders, []string{})
}

// Test different cors policies at the route level override the gateway level cors policy
func (s *testingSuite) TestRouteCorsOverrideGwCors() {
	s.setupTest([]string{httpRoutesManifest, gwCorsManifest, routeCorsManifest}, []client.Object{route, route2, gwCorsTrafficPolicy, routeCorsTrafficPolicy})

	requestHeaders := map[string]string{
		"Origin":                         "https://notexample.com",
		"Access-Control-Request-Method":  "DELETE",
		"Access-Control-Request-Headers": "x-custom-header",
	}

	expectedHeadersPath1 := map[string]any{
		"Access-Control-Allow-Origin":  "https://notexample.com",
		"Access-Control-Allow-Methods": "GET, POST, DELETE",
		"Access-Control-Allow-Headers": "x-custom-header",
	}

	expectedHeadersPath2 := map[string]any{
		"Access-Control-Allow-Origin":  "https://notexample.com",
		"Access-Control-Allow-Methods": "GET, POST",
		"Access-Control-Allow-Headers": "Content-Type, Authorization",
	}

	s.assertResponse("/path1", http.StatusOK, requestHeaders, expectedHeadersPath1, []string{})
	s.assertResponse("/path2", http.StatusOK, requestHeaders, expectedHeadersPath2, []string{})
}

func (s *testingSuite) setupTest(manifests []string, resources []client.Object) {
	s.T().Cleanup(func() {
		for _, manifest := range manifests {
			err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
			s.Require().NoError(err)
		}
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, resources...)
	})

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err, "can apply "+manifest)
	}
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, resources...)
}

func (s *testingSuite) assertResponse(path string, expectedStatus int, requestHeaders map[string]string, expectedHeaders map[string]any, notExpectedHeaders []string) {
	resp := s.testInstallation.Assertions.AssertCurlReturnResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithMethod(http.MethodOptions),
			curl.WithPath(path),
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPort(8080),
			curl.WithHeaders(requestHeaders),
		},
		&testmatchers.HttpResponse{
			StatusCode: expectedStatus,
			Headers:    expectedHeaders,
			NotHeaders: notExpectedHeaders,
		})
	defer resp.Body.Close()
}
