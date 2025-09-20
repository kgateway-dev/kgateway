package routereplacement

import (
	"context"
	"net/http"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
	setup = base.TestCase{
		Manifests: []string{
			testdefaults.CurlPodManifest,
			testdefaults.HttpbinManifest,
			setupManifest,
		},
	}

	testCases = map[string]*base.TestCase{
		"TestRouteAttachedInvalidPolicy": {
			Manifests: []string{routeAttachedInvalidPolicyManifest},
		},
		"TestInvalidMatcherDropsRoute": {
			Manifests: []string{invalidMatcherManifest},
		},
		"TestInvalidRouteRuleFilter": {
			Manifests: []string{invalidRouteRuleFilterManifest},
		},
		"TestGatewayWideInvalidPolicy": {
			Manifests: []string{gatewayWideInvalidPolicyManifest},
		},
		"TestListenerSpecificInvalidPolicy": {
			Manifests: []string{listenerSpecificInvalidPolicyManifest},
		},
		"TestListenerSpecificIsolation": {
			Manifests: []string{listenerMergeBlastRadiusManifest},
		},
	}
)

// testingSuite is a suite of route replacement tests that verify the guardrail behavior
// for invalid route configurations
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

func (s *testingSuite) SetupSuite() {
	s.BaseTestingSuite.SetupSuite()
}

func (s *testingSuite) TearDownSuite() {
	s.BaseTestingSuite.TearDownSuite()
}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	s.BaseTestingSuite.BeforeTest(suiteName, testName)
}

// TestRouteAttachedInvalidPolicy tests that routes with valid configuration
// but invalid route-attached policies are replaced with direct responses
func (s *testingSuite) TestRouteAttachedInvalidPolicy() {
	// Verify route status shows Accepted=False with RouteRuleDropped reason (for replacement)
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		invalidPolicyRoute.Name,
		invalidPolicyRoute.Namespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionFalse,
	)

	// Verify that a route with an invalid policy is replaced with a 500 direct response
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("invalid-policy.example.com"),
			curl.WithPort(gatewayPort),
			curl.WithPath("/headers"),
			curl.WithHeader("x-test-header", "some-value-with-policy"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       gomega.ContainSubstring(`invalid route configuration detected and replaced with a direct response.`),
		},
	)
}

// TestInvalidMatcherDropsRoute tests that routes with invalid matchers are dropped entirely
func (s *testingSuite) TestInvalidMatcherDropsRoute() {
	// Verify route status shows Accepted=False with RouteRuleDropped reason
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		invalidMatcherRoute.Name,
		invalidMatcherRoute.Namespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionFalse,
	)

	// Verify that the route was dropped (no route should exist, so we should get 404)
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("invalid-matcher.example.com"),
			curl.WithPort(gatewayPort),
			curl.WithPath("/headers"),
			curl.WithHeader("x-test-header", "some-value"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusNotFound,
		},
	)
}

// TestInvalidRouteRuleFilter tests that routes with invalid built-in route rule filters
// are replaced with direct responses
func (s *testingSuite) TestInvalidRouteRuleFilter() {
	// Verify route status shows Accepted=False with RouteRuleDropped reason (for replacement)
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		invalidConfigRoute.Name,
		invalidConfigRoute.Namespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionFalse,
	)

	// Verify that a route with an invalid built-in policy is replaced with a 500 direct response
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("invalid-config.example.com"),
			curl.WithPort(gatewayPort),
			curl.WithPath("/headers"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       gomega.ContainSubstring(`invalid route configuration detected and replaced with a direct response.`),
		},
	)
}

// TestGatewayWideInvalidPolicy tests that when an invalid policy is attached to the entire gateway,
// all routes across all listeners are replaced with 500 responses
func (s *testingSuite) TestGatewayWideInvalidPolicy() {
	// Verify Gateway shows Accepted=False with GatewayReplaced reason
	s.TestInstallation.Assertions.EventuallyGatewayCondition(
		s.Ctx,
		gatewayWideProxyObjectMeta.Name,
		gatewayWideProxyObjectMeta.Namespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionFalse,
	)

	// Verify both routes still show Accepted=True (routes themselves are valid, gateway policy is invalid)
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		gatewayWideRoute8080.Name,
		gatewayWideRoute8080.Namespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		gatewayWideRoute8081.Name,
		gatewayWideRoute8081.Namespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)

	// Verify that route on port 8080 is replaced with 500 response
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayWideProxyObjectMeta)),
			curl.WithHostHeader("gateway-wide-8080.example.com"),
			curl.WithPort(8080),
			curl.WithPath("/headers"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       gomega.ContainSubstring(`invalid route configuration detected and replaced with a direct response.`),
		},
	)

	// Verify that route on port 8081 is also replaced with 500 response (gateway-wide effect)
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(gatewayWideProxyObjectMeta)),
			curl.WithHostHeader("gateway-wide-8081.example.com"),
			curl.WithPort(8081),
			curl.WithPath("/headers"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       gomega.ContainSubstring(`invalid route configuration detected and replaced with a direct response.`),
		},
	)
}

// TestListenerSpecificInvalidPolicy tests that when an invalid policy is attached to a specific listener,
// only routes on that listener are affected
func (s *testingSuite) TestListenerSpecificInvalidPolicy() {
	// Verify Gateway itself remains Accepted=True (listener-specific policy doesn't affect gateway)
	s.TestInstallation.Assertions.EventuallyGatewayCondition(
		s.Ctx,
		listenerSpecificProxyObjectMeta.Name,
		listenerSpecificProxyObjectMeta.Namespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionTrue,
	)

	// Verify both routes still show Accepted=True (routes themselves are valid, listener policy is invalid)
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		listenerAffectedRoute.Name,
		listenerAffectedRoute.Namespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		listenerUnaffectedRoute.Name,
		listenerUnaffectedRoute.Namespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)

	// Verify that route on affected listener is replaced with 500 response
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(listenerSpecificProxyObjectMeta)),
			curl.WithHostHeader("listener-affected.example.com"),
			curl.WithPort(8080),
			curl.WithPath("/headers"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       gomega.ContainSubstring(`invalid route configuration detected and replaced with a direct response.`),
		},
	)

	// Verify that route on unaffected listener continues to work normally
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(listenerSpecificProxyObjectMeta)),
			curl.WithHostHeader("listener-unaffected.example.com"),
			curl.WithPort(8081),
			curl.WithPath("/headers"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		},
	)
}

// TestListenerSpecificIsolation tests that when listeners share the same port and one has an invalid policy,
// only the specific listener with the invalid policy is affected (i.e. no collateral damage to other listeners on same port)
func (s *testingSuite) TestListenerSpecificIsolation() {
	// Verify Gateway itself remains Accepted=True (listener-specific policy doesn't affect gateway)
	s.TestInstallation.Assertions.EventuallyGatewayCondition(
		s.Ctx,
		listenerIsolationProxyObjectMeta.Name,
		listenerIsolationProxyObjectMeta.Namespace,
		gwv1.GatewayConditionAccepted,
		metav1.ConditionTrue,
	)

	// Verify all routes still show Accepted=True (routes themselves are valid, listener policy is invalid)
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		mergeAffectedRoute.Name,
		mergeAffectedRoute.Namespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		mergeUnaffectedRoute.Name,
		mergeUnaffectedRoute.Namespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		mergeIsolatedRoute.Name,
		mergeIsolatedRoute.Namespace,
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)

	// Verify that route on affected listener (port 8080) is replaced with 500 response
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(listenerIsolationProxyObjectMeta)),
			curl.WithHostHeader("affected.example.com"),
			curl.WithPort(8080),
			curl.WithPath("/headers"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       gomega.ContainSubstring(`invalid route configuration detected and replaced with a direct response.`),
		},
	)

	// Verify that unaffected route on same port (port 8080) continues working normally
	// (policy is attached specifically to listener, not port-wide)
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(listenerIsolationProxyObjectMeta)),
			curl.WithHostHeader("unaffected.example.com"),
			curl.WithPort(8080),
			curl.WithPath("/headers"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		},
	)

	// Verify that isolated route on different port (port 8081) continues to work normally
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(listenerIsolationProxyObjectMeta)),
			curl.WithHostHeader("isolated.example.com"),
			curl.WithPort(8081),
			curl.WithPath("/headers"),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
		},
	)
}
