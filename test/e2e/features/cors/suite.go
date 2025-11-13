//go:build e2e

package cors

import (
	"context"
	"net/http"
	"testing"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of tests for testing CORS policies
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases,
			base.WithMinGwApiVersion(base.GwApiRequireCorsFilters),
		),
	}
}

// Test cors on specific route in a traffic policy
// The policy has the following allowOrigins:
// - https://notexample.com
// - https://a.b.*
// - https://*.edu
func (s *testingSuite) TestTrafficPolicyCorsForRoute() {
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
			s.assertResponse("/path1", requestHeaders, expectedHeaders, []string{})

			// Verify that the route without cors is not affected by the cors traffic policy (i.e. no cors headers are returned)
			s.assertResponse("/path2", requestHeaders, nil, []string{
				"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers",
			})
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
			s.assertResponse("/path1", requestHeaders, nil, []string{
				"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers",
			})

			// Verify that the route without cors is also not affected
			s.assertResponse("/path2", requestHeaders, nil, []string{
				"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers",
			})
		})
	}
}

// Test cors at the gateway level which configures cors policy in the virtual host and therefore affects all routes
func (s *testingSuite) TestTrafficPolicyCorsAtGatewayLevel() {
	requestHeaders := map[string]string{
		"Origin":                        "https://notexample.com",
		"Access-Control-Request-Method": "GET",
	}

	expectedHeaders := map[string]any{
		"Access-Control-Allow-Origin":  "https://notexample.com",
		"Access-Control-Allow-Methods": "GET, POST",
		"Access-Control-Allow-Headers": "Content-Type, Authorization",
	}

	s.assertResponse("/path1", requestHeaders, expectedHeaders, []string{})
	s.assertResponse("/path2", requestHeaders, expectedHeaders, []string{})
}

// Test different cors policies at the route level override the gateway level cors policy
func (s *testingSuite) TestTrafficPolicyRouteCorsOverrideGwCors() {
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

	s.assertResponse("/path1", requestHeaders, expectedHeadersPath1, []string{})
	s.assertResponse("/path2", requestHeaders, expectedHeadersPath2, []string{})

	// Assert that the route with CORS disabled does not return CORS headers
	s.assertResponse("/cors-disabled", requestHeaders, nil,
		[]string{"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers"})
}

// Test cors in route rules of a HTTPRoute
// The route has the following allowOrigins:
// - https://notexample.com
// - https://a.b.*
// - https://*.edu
func (s *testingSuite) TestHttpRouteCorsInRouteRules() {
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
			s.assertResponse("/path1", requestHeaders, expectedHeaders, []string{})

			// Verify that the route without cors is not affected by the cors in the HTTPRoute (i.e. no cors headers are returned)
			s.assertResponse("/path2", requestHeaders, nil, []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers"})
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
			s.assertResponse("/path1", requestHeaders, nil, []string{
				"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers",
			})

			// Verify that the route without cors is also not affected
			s.assertResponse("/path2", requestHeaders, nil, []string{
				"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers",
			})
		})
	}
}

// Test a combination of cors in route rules of a HTTPRoute and cors in a traffic policy
// applied at the gateway level.
// We expect the cors in the route rules to override the cors in the traffic policy for /path1 but
// for /path2 the cors in the traffic policy should be applied.
func (s *testingSuite) TestHttpRouteAndTrafficPolicyCors() {
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

	s.assertResponse("/path1", requestHeaders, expectedHeadersPath1, []string{})
	s.assertResponse("/path2", requestHeaders, expectedHeadersPath2, []string{})
}

func (s *testingSuite) assertResponse(path string, requestHeaders map[string]string, expectedHeaders map[string]any, notExpectedHeaders []string) {
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
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
			StatusCode: http.StatusOK,
			Headers:    expectedHeaders,
			NotHeaders: notExpectedHeaders,
		})
}

// TestTrafficPolicyClearStaleStatus verifies that stale status is cleared when targetRef becomes invalid
func (s *testingSuite) TestTrafficPolicyClearStaleStatus() {
	kgatewayControllerName := wellknown.DefaultGatewayControllerName
	otherControllerName := "other-controller.example.com/controller"

	// Add fake ancestor status from another controller
	s.addAncestorStatus("gw-cors-policy", "default", otherControllerName)

	// Verify both kgateway and other controller statuses exist
	s.assertAncestorStatuses("gw", map[string]bool{
		kgatewayControllerName: true,
		otherControllerName:    true,
	})

	// Apply policy with missing gateway target
	err := s.TestInstallation.Actions.Kubectl().ApplyFile(
		s.Ctx,
		gwCorsTrafficPolicyMissingTargetManifest,
	)
	s.Require().NoError(err)

	// Verify kgateway status cleared, other remains
	s.assertAncestorStatuses("gw", map[string]bool{
		kgatewayControllerName: false,
		otherControllerName:    true,
	})
}

func (s *testingSuite) addAncestorStatus(policyName, policyNamespace, controllerName string) {
	currentTimeout, pollingInterval := helpers.GetTimeouts()
	s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
		policy := &v1alpha1.TrafficPolicy{}
		err := s.TestInstallation.ClusterContext.Client.Get(
			s.Ctx,
			types.NamespacedName{Name: policyName, Namespace: policyNamespace},
			policy,
		)
		g.Expect(err).NotTo(gomega.HaveOccurred())

		// Add fake ancestor status
		fakeStatus := gwv1.PolicyAncestorStatus{
			AncestorRef:    gwv1.ParentReference{Name: "gw"},
			ControllerName: gwv1.GatewayController(controllerName),
			Conditions: []metav1.Condition{
				{
					Type:               string(v1alpha1.PolicyConditionAccepted),
					Status:             metav1.ConditionTrue,
					Reason:             string(v1alpha1.PolicyReasonValid),
					Message:            "Accepted by fake controller",
					LastTransitionTime: metav1.Now(),
				},
			},
		}

		policy.Status.Ancestors = append(policy.Status.Ancestors, fakeStatus)
		err = s.TestInstallation.ClusterContext.Client.Status().Update(s.Ctx, policy)
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

func (s *testingSuite) assertAncestorStatuses(ancestorName string, expectedControllers map[string]bool) {
	currentTimeout, pollingInterval := helpers.GetTimeouts()
	s.TestInstallation.Assertions.Gomega.Eventually(func(g gomega.Gomega) {
		policy := &v1alpha1.TrafficPolicy{}
		err := s.TestInstallation.ClusterContext.Client.Get(
			s.Ctx,
			types.NamespacedName{Name: "gw-cors-policy", Namespace: "default"},
			policy,
		)
		g.Expect(err).NotTo(gomega.HaveOccurred())

		foundControllers := make(map[string]bool)
		for _, ancestor := range policy.Status.Ancestors {
			if string(ancestor.AncestorRef.Name) == ancestorName {
				foundControllers[string(ancestor.ControllerName)] = true
			}
		}

		for controller, shouldExist := range expectedControllers {
			exists := foundControllers[controller]
			if shouldExist {
				g.Expect(exists).To(gomega.BeTrue(), "Expected controller %s to exist in status", controller)
			} else {
				g.Expect(exists).To(gomega.BeFalse(), "Expected controller %s to not exist in status", controller)
			}
		}
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}
