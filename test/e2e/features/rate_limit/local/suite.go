//go:build e2e

package local_rate_limit

import (
	"context"
	"fmt"
	"net/http"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
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
	s.testInstallation.AssertionsT(s.T()).EventuallyObjectsExist(s.ctx, s.commonResources...)

	// make sure pods are running
	s.testInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})
	s.testInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx, simpleDeployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=backend-0,version=v1",
	})
	s.testInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", testdefaults.WellKnownAppLabel, proxyObjectMeta.GetName()),
	})
}

func (s *testingSuite) TearDownSuite() {
	if testutils.ShouldSkipCleanup(s.T()) {
		return
	}
	// clean up common resources
	for _, manifest := range s.commonManifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err, "can delete "+manifest)
	}
	s.testInstallation.AssertionsT(s.T()).EventuallyObjectsNotExist(s.ctx, s.commonResources...)

	// make sure pods are gone
	s.testInstallation.AssertionsT(s.T()).EventuallyPodsNotExist(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})
	s.testInstallation.AssertionsT(s.T()).EventuallyPodsNotExist(s.ctx, simpleDeployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app=backend-0,version=v1",
	})
	s.testInstallation.AssertionsT(s.T()).EventuallyPodsNotExist(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", testdefaults.WellKnownAppLabel, proxyObjectMeta.GetName()),
	})
}

// Test cases for local rate limit on a route (/path1)
func (s *testingSuite) TestLocalRateLimitForRoute() {
	s.setupTest([]string{httpRoutesManifest, routeLocalRateLimitManifest}, []client.Object{route, route2, routeRateLimitTrafficPolicy})

	// First request should be successful
	s.assertResponse("/path1")

	// Consecutive requests should be rate limited
	s.assertConsistentResponse("/path1", http.StatusTooManyRequests)

	// Second route shouldn't be rate limited
	s.assertConsistentResponse("/path2", http.StatusOK)
}

// Test cases for local rate limit on a gateway
func (s *testingSuite) TestLocalRateLimitForGateway() {
	s.setupTest([]string{httpRoutesManifest, gwLocalRateLimitManifest}, []client.Object{route, route2, gwRateLimitTrafficPolicy})

	// First request should be successful (to any route)
	s.assertResponse("/path1")

	// Consecutive requests should be rate limited
	s.assertConsistentResponse("/path1", http.StatusTooManyRequests)

	// Also verify that the second route is rate limited
	s.assertConsistentResponse("/path2", http.StatusTooManyRequests)
}

// Test cases for local rate limit on a gateway and route (/path1)
func (s *testingSuite) TestLocalRateLimitForGatewayAndRoute() {
	s.setupTest([]string{httpRoutesManifest, gwLocalRateLimitManifest, routeLocalRateLimitManifest},
		[]client.Object{route, route2, gwRateLimitTrafficPolicy, routeRateLimitTrafficPolicy})

	// First request should be successful (to any route)
	s.assertResponse("/path1")

	// Consecutive requests should be rate limited
	s.assertConsistentResponse("/path1", http.StatusTooManyRequests)

	// Also verify that the second route is rate limited
	s.assertConsistentResponse("/path2", http.StatusTooManyRequests)

	// Verify that the rate limit is removed after a token has been added to the bucket (10s)
	// while GW rate limit is configured to add token every 300s. Therefore, the route
	// rate limit configuration takes precedence.
	s.assertEventualResponse("/path1", http.StatusOK)
}

// Test cases for local rate limit on a gateway and route (/path1) with disabled
// local rate limit
func (s *testingSuite) TestLocalRateLimitDisabledForRoute() {
	s.setupTest([]string{httpRoutesManifest, gwLocalRateLimitManifest, disabledRouteLocalRateLimitManifest},
		[]client.Object{route, route2, gwRateLimitTrafficPolicy, routeRateLimitTrafficPolicy})

	// First request should be successful (to any route)
	s.assertResponse("/path1")

	// Consecutive requests should not be rate limited (disaled for this path)
	s.assertConsistentResponse("/path1", http.StatusOK)

	// Also verify that the second route is rate limited
	s.assertConsistentResponse("/path2", http.StatusTooManyRequests)
}

// Test cases for local rate limit on a route (/path2) using extensionref in the HTTPRoute
func (s *testingSuite) TestLocalRateLimitForRouteUsingExtensionRef() {
	s.setupTest([]string{extensionRefManifest}, []client.Object{route, routeRateLimitTrafficPolicy})

	// First request should be successful
	s.assertResponse("/path2")

	// Consecutive requests should be rate limited
	s.assertConsistentResponse("/path2", http.StatusTooManyRequests)

	// Second route shouldn't be rate limited
	s.assertConsistentResponse("/path1", http.StatusOK)
}

func (s *testingSuite) setupTest(manifests []string, resources []client.Object) {
	testutils.Cleanup(s.T(), func() {
		for _, manifest := range manifests {
			err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
			s.Require().NoError(err)
		}
		s.testInstallation.AssertionsT(s.T()).EventuallyObjectsNotExist(s.ctx, resources...)
	})

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err, "can apply "+manifest)
	}
	s.testInstallation.AssertionsT(s.T()).EventuallyObjectsExist(s.ctx, resources...)
}

func (s *testingSuite) assertResponse(path string) {
	s.testInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithPath(path),
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		})
}

func (s *testingSuite) assertConsistentResponse(path string, expectedStatus int) {
	s.testInstallation.AssertionsT(s.T()).AssertEventuallyConsistentCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithPath(path),
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: expectedStatus,
		})
}

func (s *testingSuite) assertEventualResponse(path string, expectedStatus int) {
	resp := s.testInstallation.AssertionsT(s.T()).AssertEventualCurlReturnResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithPath(path),
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: expectedStatus,
		})
	defer resp.Body.Close()
}
