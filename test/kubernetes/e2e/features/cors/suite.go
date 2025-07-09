package cors

import (
	"context"
	"fmt"
	"net/http"
	"testing"

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

// testingSuite is a suite of tests for testing CORS policies
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

// Test cors on specific route in a traffic policy
// The policy has the following allowOrigins:
// - https://notexample.com
// - https://a.b.*
// - https://*.edu
func (s *testingSuite) TestTrafficPolicyCorsForRoute() {
	s.setupTest([]string{httpRoutesManifest, routeCorsTrafficPolicyManifest}, []client.Object{route, route2, routeCorsTrafficPolicy})

	testCases := []struct {
		name   string
		origin string
	}{
		{
			name:   "exact_match_origin",
			origin: "https://notexample.com",
		},
		{
			name:   "prefix_match_origin",
			origin: "https://a.b.c.d",
		},
		{
			name:   "regex_match_origin",
			origin: "https://test.cors.edu",
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			requestHeaders := map[string]string{
				"Origin":                        tc.origin,
				"Access-Control-Request-Method": "GET",
			}

			expectedHeaders := map[string]any{
				"Access-Control-Allow-Origin":  tc.origin,
				"Access-Control-Allow-Methods": "GET, POST, DELETE",
				"Access-Control-Allow-Headers": "x-custom-header",
			}

			// Verify that the route with cors is responding to the OPTIONS request with the expected cors headers
			s.assertResponse("/path1", http.StatusOK, requestHeaders, expectedHeaders, []string{})

			// Verify that the route without cors is not affected by the cors traffic policy (i.e. no cors headers are returned)
			s.assertResponse("/path2", http.StatusOK, requestHeaders, nil, []string{
				"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers"})
		})
	}

	// Negative test cases - origins that should NOT match the patterns
	negativeTestCases := []struct {
		name   string
		origin string
	}{
		{
			name:   "wildcard_subdomain_should_not_match_different_domain",
			origin: "https://notedu.com",
		},
		{
			name:   "wildcard_subdomain_should_not_match_different_tld",
			origin: "https://api.example.org",
		},
		{
			name:   "wildcard_subdomain_should_not_match_without_subdomain",
			origin: "https://edu",
		},
		{
			name:   "prefix_match_should_not_match_different_scheme",
			origin: "http://a.b.c.d",
		},
		{
			name:   "exact_match_should_not_match_similar_domain",
			origin: "https://notexample.org",
		},
		{
			name:   "exact_match_should_not_match_with_subdomain",
			origin: "https://api.notexample.com",
		},
		{
			name:   "prefix_match_should_not_match_invalid_url",
			origin: "https:/a.b",
		},
	}

	for _, tc := range negativeTestCases {
		s.T().Run("negative_"+tc.name, func(t *testing.T) {
			requestHeaders := map[string]string{
				"Origin":                        tc.origin,
				"Access-Control-Request-Method": "GET",
			}

			// For negative cases, we expect no CORS headers to be returned
			// since the origin doesn't match any of the allowed patterns
			s.assertResponse("/path1", http.StatusOK, requestHeaders, nil, []string{
				"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers"})

			// Verify that the route without cors is also not affected
			s.assertResponse("/path2", http.StatusOK, requestHeaders, nil, []string{
				"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers"})
		})
	}
}

// Test cors at the gateway level which configures cors policy in the virtual host and therefore affects all routes
func (s *testingSuite) TestTrafficPolicyCorsAtGatewayLevel() {
	s.setupTest([]string{httpRoutesManifest, gwCorsTrafficPolicyManifest}, []client.Object{route, route2, gwCorsTrafficPolicy})

	requestHeaders := map[string]string{
		"Origin":                        "https://notexample.com",
		"Access-Control-Request-Method": "GET",
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
func (s *testingSuite) TestTrafficPolicyRouteCorsOverrideGwCors() {
	s.setupTest([]string{httpRoutesManifest, gwCorsTrafficPolicyManifest, routeCorsTrafficPolicyManifest},
		[]client.Object{route, route2, gwCorsTrafficPolicy, routeCorsTrafficPolicy})

	requestHeaders := map[string]string{
		"Origin":                        "https://notexample.com",
		"Access-Control-Request-Method": "GET",
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

// Test cors in route rules of a HTTPRoute
// The route has the following allowOrigins:
// - https://notexample.com
// - https://a.b.*
// - https://*.edu
func (s *testingSuite) TestHttpRouteCorsInRouteRules() {
	s.setupTest([]string{httpRoutesManifest, corsHttpRoutesManifest}, []client.Object{route, route2})

	testCases := []struct {
		name   string
		origin string
	}{
		{
			name:   "exact_match_origin",
			origin: "https://notexample.com",
		},
		{
			name:   "prefix_match_origin",
			origin: "https://a.b.c.d",
		},
		{
			name:   "regex_match_origin",
			origin: "https://test.cors.edu",
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			requestHeaders := map[string]string{
				"Origin":                        tc.origin,
				"Access-Control-Request-Method": "GET",
			}

			expectedHeaders := map[string]any{
				"Access-Control-Allow-Origin":  tc.origin,
				"Access-Control-Allow-Methods": "GET",
				"Access-Control-Allow-Headers": "x-custom-header",
			}

			// Verify that the route with cors is responding to the OPTIONS request with the expected cors headers
			s.assertResponse("/path1", http.StatusOK, requestHeaders, expectedHeaders, []string{})

			// Verify that the route without cors is not affected by the cors in the HTTPRoute (i.e. no cors headers are returned)
			s.assertResponse("/path2", http.StatusOK, requestHeaders, nil, []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers"})
		})
	}

	// Negative test cases - origins that should NOT match the patterns
	negativeTestCases := []struct {
		name   string
		origin string
	}{
		{
			name:   "wildcard_subdomain_should_not_match_different_domain",
			origin: "https://notedu.com",
		},
		{
			name:   "wildcard_subdomain_should_not_match_different_tld",
			origin: "https://api.example.org",
		},
		{
			name:   "wildcard_subdomain_should_not_match_without_subdomain",
			origin: "https://edu",
		},
		{
			name:   "prefix_match_should_not_match_different_scheme",
			origin: "http://a.b.c.d",
		},
		{
			name:   "exact_match_should_not_match_similar_domain",
			origin: "https://notexample.org",
		},
		{
			name:   "exact_match_should_not_match_with_subdomain",
			origin: "https://api.notexample.com",
		},
		{
			name:   "prefix_match_should_not_match_invalid_url",
			origin: "https:/a.b",
		},
	}

	for _, tc := range negativeTestCases {
		s.T().Run("negative_"+tc.name, func(t *testing.T) {
			requestHeaders := map[string]string{
				"Origin":                        tc.origin,
				"Access-Control-Request-Method": "GET",
			}

			// For negative cases, we expect no CORS headers to be returned
			// since the origin doesn't match any of the allowed patterns
			s.assertResponse("/path1", http.StatusOK, requestHeaders, nil, []string{
				"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers"})

			// Verify that the route without cors is also not affected
			s.assertResponse("/path2", http.StatusOK, requestHeaders, nil, []string{
				"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers"})
		})
	}
}

// Test a combination of cors in route rules of a HTTPRoute and cors in a traffic policy
// applied at the gateway level.
// We expect the cors in the route rules to override the cors in the traffic policy for /path1 but
// for /path2 the cors in the traffic policy should be applied.
func (s *testingSuite) TestHttpRouteAndTrafficPolicyCors() {
	s.setupTest([]string{httpRoutesManifest, corsHttpRoutesManifest, gwCorsTrafficPolicyManifest},
		[]client.Object{route, route2, gwCorsTrafficPolicy})

	requestHeaders := map[string]string{
		"Origin":                        "https://notexample.com",
		"Access-Control-Request-Method": "GET",
	}

	// HTTPRoute for /path1 should have this cors response headers
	expectedHeadersPath1 := map[string]any{
		"Access-Control-Allow-Origin":  "https://notexample.com",
		"Access-Control-Allow-Methods": "GET",
		"Access-Control-Allow-Headers": "x-custom-header",
	}

	// CORS at the vhost level translated from the TrafficPolicy should have
	// this cors response headers for all other routes
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
