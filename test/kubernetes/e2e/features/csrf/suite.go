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
	s.setupTest([]string{csrfRouteTrafficPolicyManifest}, []client.Object{routeTrafficPolicy})

	// Request without origin header should be rejected
	s.assertPreflightResponse("/path1", http.StatusForbidden, []curl.Option{})

	// Request without origin header to route that doesn't have CSRF protection
	// should be allowed
	s.assertPreflightResponse("/path2", http.StatusOK, []curl.Option{})

	// Request with valid origin header should be allowed
	s.assertPreflightResponse("/path1", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "example.com"),
	})

	// Request with invalid origin header should be rejected
	s.assertPreflightResponse("/path1", http.StatusForbidden, []curl.Option{
		curl.WithHeader("Origin", "notexample.com"),
	})

	// Test suffix matching
	s.assertPreflightResponse("/path1", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "a.routetest.io"),
	})
}

func (s *testingSuite) TestGatewayLevelCSRF() {
	s.setupTest([]string{csrfGwTrafficPolicyManifest}, []client.Object{gwtrafficPolicy})

	// Request without origin header should be rejected
	s.assertPreflightResponse("/path1", http.StatusForbidden, []curl.Option{})

	// Request without origin header should be rejected
	s.assertPreflightResponse("/path2", http.StatusForbidden, []curl.Option{})

	// Request with valid origin header should be allowed
	s.assertPreflightResponse("/path1", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "example.com"),
	})

	// Test suffix matching
	s.assertPreflightResponse("/path1", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "a.gwtest.io"),
	})

	// Request with valid origin header should be allowed
	s.assertPreflightResponse("/path2", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "example.com"),
	})

	// Test prefix matching
	s.assertPreflightResponse("/path2", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "sample.com"),
	})
}

func (s *testingSuite) TestMultiLevelsCSRF() {
	s.setupTest([]string{csrfGwTrafficPolicyManifest, csrfRouteTrafficPolicyManifest}, []client.Object{gwtrafficPolicy, routeTrafficPolicy})

	// Request without origin header should be rejected
	s.assertPreflightResponse("/path1", http.StatusForbidden, []curl.Option{})

	// Request without origin header should be rejected
	s.assertPreflightResponse("/path2", http.StatusForbidden, []curl.Option{})

	// Test suffix matching from route level policy (overrides the gateway additional origins)
	s.assertPreflightResponse("/path1", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "a.routetest.io"),
	})

	// Test suffix matching from gateway level policy as no route level policy is set
	s.assertPreflightResponse("/path2", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "a.gwtest.io"),
	})
}

func (s *testingSuite) TestShadowedRouteLevelCSRF() {
	s.setupTest([]string{csrfShadowedRouteTrafficPolicyManifest}, []client.Object{routeTrafficPolicy})

	// CSRF policies are being evaluated (not tested) but not enforced

	s.assertPreflightResponse("/path1", http.StatusOK, []curl.Option{})
	s.assertPreflightResponse("/path2", http.StatusOK, []curl.Option{})
	s.assertPreflightResponse("/path1", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "example.com"),
	})
	s.assertPreflightResponse("/path1", http.StatusOK, []curl.Option{
		curl.WithHeader("Origin", "notexample.com"),
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

// A safe http method is one that doesn't alter the state of the server (ie read only).
// A CSRF attack targets state changing requests, so the filter only acts on unsafe methods (ones that change state).
// We use POST as the unsafe method to test the filter.
func (s *testingSuite) assertPreflightResponse(path string, expectedStatus int, options []curl.Option) {
	allOptions := append([]curl.Option{
		curl.WithMethod("POST"),
		curl.WithPath(path),
		curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
		curl.WithHostHeader("example.com"),
		curl.WithPort(8080),
	}, options...)

	s.testInstallation.Assertions.AssertEventuallyConsistentCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		allOptions,
		&testmatchers.HttpResponse{StatusCode: expectedStatus},
	)
}
