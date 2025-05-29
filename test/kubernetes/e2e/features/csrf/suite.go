package csrf

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
		commonManifest,
	}
	s.commonResources = []client.Object{
		// resources from curl manifest
		testdefaults.CurlPod,
		// resources from service manifest
		simpleSvc, simpleDeployment,
		// resources from gateway manifest
		gateway,
		// routes
		route, route2,
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

func (s *testingSuite) TestRouteLevelCSRF() {
	s.setupTest([]string{csrfRouteTrafficPolicyManifest}, []client.Object{trafficPolicy})

	// Request without origin header should be rejected
	s.assertResponse("/path1", http.StatusForbidden, []curl.Option{
		curl.WithMethod("POST"),
	})

	// Request without origin header to route that doesn't have CSRF protection
	// should be allowed
	s.assertResponse("/path2", http.StatusOK, []curl.Option{
		curl.WithMethod("POST"),
	})

	// Request with valid origin header should be allowed
	s.assertResponse("/path1", http.StatusOK, []curl.Option{
		curl.WithMethod("POST"),
		curl.WithHeader("Origin", "example.com"),
	})

	// Request with invalid origin header should be rejected
	s.assertResponse("/path1", http.StatusForbidden, []curl.Option{
		curl.WithMethod("POST"),
		curl.WithHeader("Origin", "notexample.com"),
	})

	// Request with additional allowed origin header should be allowed
	s.assertResponse("/path1", http.StatusOK, []curl.Option{
		curl.WithMethod("POST"),
		curl.WithHeader("Origin", "example.org"),
	})
}

func (s *testingSuite) TestGatewayLevelCSRF() {
	s.setupTest([]string{csrfGwTrafficPolicyManifest}, []client.Object{trafficPolicy})

	// Request without origin header should be rejected
	s.assertResponse("/path1", http.StatusForbidden, []curl.Option{
		curl.WithMethod("POST"),
	})

	// Request without origin header should be rejected
	s.assertResponse("/path2", http.StatusForbidden, []curl.Option{
		curl.WithMethod("POST"),
	})

	// Request with valid origin header should be allowed
	s.assertResponse("/path1", http.StatusOK, []curl.Option{
		curl.WithMethod("POST"),
		curl.WithHeader("Origin", "example.com"),
	})

	// Request with valid origin header should be allowed
	s.assertResponse("/path2", http.StatusOK, []curl.Option{
		curl.WithMethod("POST"),
		curl.WithHeader("Origin", "example.com"),
	})
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

func (s *testingSuite) assertResponse(path string, expectedStatus int, options []curl.Option) {
	allOptions := append([]curl.Option{
		curl.WithPath(path),
		curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
		curl.WithHostHeader("example.com"),
		curl.WithPort(8080),
	}, options...)

	resp := s.testInstallation.Assertions.AssertCurlReturnResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		allOptions,
		&testmatchers.HttpResponse{StatusCode: expectedStatus},
	)
	s.Equal(expectedStatus, resp.StatusCode)
	resp.Body.Close()
}
