//go:build e2e

package apikeyauth

import (
	"context"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of tests for API key authentication functionality
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// TestAPIKeyAuthWithHTTPRouteLevelPolicy tests API key authentication with TrafficPolicy applied at HTTPRoute level
func (s *testingSuite) TestAPIKeyAuthWithHTTPRouteLevelPolicy() {
	// Verify HTTPRoute is accepted before running the test
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "httpbin-route", "default", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	statusReqCurlOpts := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
		curl.WithHostHeader("httpbin"),
		curl.WithPort(8080),
		curl.WithPath("/status/200"),
	}
	// missing API key, should fail
	s.T().Log("The /status route has API key auth applied at HTTPRoute level, should fail when API key is missing")
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusReqCurlOpts,
		expectAPIKeyAuthDenied,
	)
	// has valid API key, should succeed
	s.T().Log("The /status route has API key auth applied at HTTPRoute level, should succeed when valid API key is present")
	statusWithAPIKeyCurlOpts := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-123"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusWithAPIKeyCurlOpts,
		expectStatus200Success,
	)
	// has valid API key with Bearer prefix in Authorization header, should succeed
	s.T().Log("The /status route has API key auth applied at HTTPRoute level, should succeed when valid API key is present with Bearer prefix in Authorization header")
	statusWithBearerAPIKeyCurlOpts := append(statusReqCurlOpts, curl.WithHeader("Authorization", "Bearer k-123"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusWithBearerAPIKeyCurlOpts,
		expectStatus200Success,
	)

	getReqCurlOpts := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
		curl.WithHostHeader("httpbin"),
		curl.WithPort(8080),
		curl.WithPath("/get"),
	}
	// missing API key, should fail
	s.T().Log("The /get route has API key auth applied at HTTPRoute level, should fail when API key is missing")
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getReqCurlOpts,
		expectAPIKeyAuthDenied,
	)
	// has valid API key, should succeed
	s.T().Log("The /get route has API key auth applied at HTTPRoute level, should succeed when valid API key is present")
	getWithAPIKeyCurlOpts := append(getReqCurlOpts, curl.WithHeader("api-key", "k-123"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getWithAPIKeyCurlOpts,
		expectStatus200Success,
	)
	// has valid API key with Bearer prefix in Authorization header, should succeed
	s.T().Log("The /get route has API key auth applied at HTTPRoute level, should succeed when valid API key is present with Bearer prefix in Authorization header")
	getWithBearerAPIKeyCurlOpts := append(getReqCurlOpts, curl.WithHeader("Authorization", "Bearer k-123"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getWithBearerAPIKeyCurlOpts,
		expectStatus200Success,
	)
	// has valid API key with Bearer prefix using different key, should succeed
	s.T().Log("The /get route has API key auth applied at HTTPRoute level, should succeed when valid API key (k-456) is present with Bearer prefix in Authorization header")
	getWithBearerAPIKey2CurlOpts := append(getReqCurlOpts, curl.WithHeader("Authorization", "Bearer k-456"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getWithBearerAPIKey2CurlOpts,
		expectStatus200Success,
	)
	// has invalid API key with Bearer prefix in Authorization header, should fail
	s.T().Log("The /get route has API key auth applied at HTTPRoute level, should fail when invalid API key is present with Bearer prefix in Authorization header")
	getWithInvalidBearerAPIKeyCurlOpts := append(getReqCurlOpts, curl.WithHeader("Authorization", "Bearer invalid-key"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getWithInvalidBearerAPIKeyCurlOpts,
		expectAPIKeyAuthDenied,
	)
}

// TestAPIKeyAuthWithRouteLevelPolicy tests API key authentication with TrafficPolicy applied at route level (sectionName)
func (s *testingSuite) TestAPIKeyAuthWithRouteLevelPolicy() {
	// Verify HTTPRoute is accepted before running the test
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "httpbin-route", "default", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	statusReqCurlOpts := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
		curl.WithHostHeader("httpbin"),
		curl.WithPort(8080),
		curl.WithPath("/status/200"),
	}
	// missing API key, no API key auth on route, should succeed
	s.T().Log("The /status route has no API key auth policy")
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusReqCurlOpts,
		expectStatus200Success,
	)

	getReqCurlOpts := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
		curl.WithHostHeader("httpbin"),
		curl.WithPort(8080),
		curl.WithPath("/get"),
	}
	// missing API key, should fail
	s.T().Log("The /get route has an API key auth policy applied at the route level, should fail when API key is missing")
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getReqCurlOpts,
		expectAPIKeyAuthDenied,
	)
	// has valid API key, should succeed
	s.T().Log("The /get route has an API key auth policy applied at the route level, should succeed when valid API key is present")
	getWithAPIKeyCurlOpts := append(getReqCurlOpts, curl.WithHeader("api-key", "k-123"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getWithAPIKeyCurlOpts,
		expectStatus200Success,
	)
	// has invalid API key, should fail
	s.T().Log("The /get route has an API key auth policy applied at the route level, should fail when invalid API key is present")
	getWithInvalidAPIKeyCurlOpts := append(getReqCurlOpts, curl.WithHeader("api-key", "invalid-key"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getWithInvalidAPIKeyCurlOpts,
		expectAPIKeyAuthDenied,
	)
}

// TestAPIKeyAuthWithQueryParameter tests API key authentication using query parameter as the key source
func (s *testingSuite) TestAPIKeyAuthWithQueryParameter() {
	// Verify HTTPRoute is accepted before running the test
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "httpbin-route-query", "default", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	statusReqCurlOpts := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
		curl.WithHostHeader("httpbin"),
		curl.WithPort(8080),
		curl.WithPath("/status/200"),
	}
	// missing API key, should fail
	s.T().Log("The /status route has API key auth with query parameter, should fail when API key is missing")
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusReqCurlOpts,
		expectAPIKeyAuthDenied,
	)
	// has valid API key in query parameter, should succeed
	s.T().Log("The /status route has API key auth with query parameter, should succeed when valid API key is present in query")
	statusWithAPIKeyCurlOpts := append(statusReqCurlOpts, curl.WithQueryParameters(map[string]string{"api-key": "k-123"}))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusWithAPIKeyCurlOpts,
		expectStatus200Success,
	)

	getReqCurlOpts := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
		curl.WithHostHeader("httpbin"),
		curl.WithPort(8080),
		curl.WithPath("/get"),
	}
	// missing API key, should fail
	s.T().Log("The /get route has API key auth with query parameter, should fail when API key is missing")
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getReqCurlOpts,
		expectAPIKeyAuthDenied,
	)
	// has valid API key in query parameter, should succeed
	s.T().Log("The /get route has API key auth with query parameter, should succeed when valid API key is present in query")
	getWithAPIKeyCurlOpts := append(getReqCurlOpts, curl.WithQueryParameters(map[string]string{"api-key": "k-123"}))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getWithAPIKeyCurlOpts,
		expectStatus200Success,
	)
	// has invalid API key in query parameter, should fail
	s.T().Log("The /get route has API key auth with query parameter, should fail when invalid API key is present in query")
	getWithInvalidAPIKeyCurlOpts := append(getReqCurlOpts, curl.WithQueryParameters(map[string]string{"api-key": "invalid-key"}))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getWithInvalidAPIKeyCurlOpts,
		expectAPIKeyAuthDenied,
	)
}

// TestAPIKeyAuthWithCookie tests API key authentication using cookie as the key source
func (s *testingSuite) TestAPIKeyAuthWithCookie() {
	// Verify HTTPRoute is accepted before running the test
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "httpbin-route-cookie", "default", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	statusReqCurlOpts := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
		curl.WithHostHeader("httpbin"),
		curl.WithPort(8080),
		curl.WithPath("/status/200"),
	}
	// missing API key, should fail
	s.T().Log("The /status route has API key auth with cookie, should fail when API key is missing")
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusReqCurlOpts,
		expectAPIKeyAuthDenied,
	)
	// has valid API key in cookie, should succeed
	s.T().Log("The /status route has API key auth with cookie, should succeed when valid API key is present in cookie")
	statusWithAPIKeyCurlOpts := append(statusReqCurlOpts, curl.WithCookie("api-key=k-123"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusWithAPIKeyCurlOpts,
		expectStatus200Success,
	)

	getReqCurlOpts := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
		curl.WithHostHeader("httpbin"),
		curl.WithPort(8080),
		curl.WithPath("/get"),
	}
	// missing API key, should fail
	s.T().Log("The /get route has API key auth with cookie, should fail when API key is missing")
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getReqCurlOpts,
		expectAPIKeyAuthDenied,
	)
	// has valid API key in cookie, should succeed
	s.T().Log("The /get route has API key auth with cookie, should succeed when valid API key is present in cookie")
	getWithAPIKeyCurlOpts := append(getReqCurlOpts, curl.WithCookie("api-key=k-123"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getWithAPIKeyCurlOpts,
		expectStatus200Success,
	)
	// has invalid API key in cookie, should fail
	s.T().Log("The /get route has API key auth with cookie, should fail when invalid API key is present in cookie")
	getWithInvalidAPIKeyCurlOpts := append(getReqCurlOpts, curl.WithCookie("api-key=invalid-key"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getWithInvalidAPIKeyCurlOpts,
		expectAPIKeyAuthDenied,
	)
}

// TestAPIKeyAuthWithSecretUpdate tests that API key authentication correctly handles secret updates
func (s *testingSuite) TestAPIKeyAuthWithSecretUpdate() {
	// Verify HTTPRoute is accepted before running the test
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "httpbin-route-secret-update", "default", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	statusReqCurlOpts := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
		curl.WithHostHeader("httpbin"),
		curl.WithPort(8080),
		curl.WithPath("/status/200"),
	}

	// Step 1: Verify initial API keys work (k-123, k-456)
	s.T().Log("Step 1: Verifying initial API keys (k-123, k-456) work")
	statusWithK123 := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-123"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusWithK123,
		expectStatus200Success,
	)
	statusWithK456 := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-456"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusWithK456,
		expectStatus200Success,
	)

	// Step 2: Update the secret with new keys (k-789, k-999) and remove old ones
	// Note: kubectl apply with stringData merges with existing data, so we need to delete and recreate
	// to properly replace the secret content
	s.T().Log("Step 2: Updating secret with new API keys (k-789, k-999)")
	err := s.TestInstallation.Actions.Kubectl().Delete(s.Ctx, []byte(`apiVersion: v1
kind: Secret
metadata:
  name: api-keys-secret-update
  namespace: default
`))
	s.Require().NoError(err, "failed to delete old secret")

	updatedSecretYAML := `apiVersion: v1
kind: Secret
metadata:
  name: api-keys-secret-update
  namespace: default
type: Opaque
stringData:
  client3: k-789
  client4: k-999
` //gosec:disable G101
	err = s.TestInstallation.Actions.Kubectl().Apply(s.Ctx, []byte(updatedSecretYAML))
	s.Require().NoError(err, "failed to apply updated secret")

	// Wait for secret update to propagate through KRT and Envoy config regeneration
	// The KRT framework should detect the secret change and trigger filter config regeneration
	// Using Eventually to wait for the new keys to work, which confirms the update propagated
	s.T().Log("Waiting for secret update to propagate...")
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		append(statusReqCurlOpts, curl.WithHeader("api-key", "k-789")),
		expectStatus200Success,
	)

	// Step 3: Verify new keys work
	s.T().Log("Step 3: Verifying new API keys (k-789, k-999) work")
	statusWithK789 := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-789"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusWithK789,
		expectStatus200Success,
	)
	statusWithK999 := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-999"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusWithK999,
		expectStatus200Success,
	)

	// Step 4: Verify old keys no longer work
	s.T().Log("Step 4: Verifying old API keys (k-123, k-456) no longer work")
	statusWithK123Old := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-123"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusWithK123Old,
		expectAPIKeyAuthDenied,
	)
	statusWithK456Old := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-456"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusWithK456Old,
		expectAPIKeyAuthDenied,
	)

	// Step 5: Update secret again to add back an old key and add a new one
	// Note: kubectl apply with stringData merges with existing data
	// The secret will now have: client1 (k-123), client3 (k-789), client4 (k-999), client5 (k-111)
	s.T().Log("Step 5: Updating secret again to add back k-123 and add k-111")
	finalSecretYAML := `apiVersion: v1
kind: Secret
metadata:
  name: api-keys-secret-update
  namespace: default
type: Opaque
stringData:
  client1: k-123
  client5: k-111
` //gosec:disable G101
	err = s.TestInstallation.Actions.Kubectl().Apply(s.Ctx, []byte(finalSecretYAML))
	s.Require().NoError(err, "failed to apply final secret update")

	// Step 6: Verify the final set of keys work
	// After Step 5 merge update, the secret has: client1 (k-123), client3 (k-789), client4 (k-999), client5 (k-111)
	s.T().Log("Step 6: Verifying final API keys (k-123, k-789, k-999, k-111) works")
	statusWithK123Final := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-123"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusWithK123Final,
		expectStatus200Success,
	)
	statusWithK789Final := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-789"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusWithK789Final,
		expectStatus200Success,
	)
	statusWithK999Final := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-999"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusWithK999Final,
		expectStatus200Success,
	)
	statusWithK111Final := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-111"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusWithK111Final,
		expectStatus200Success,
	)

	// Step 7: Verify removed keys no longer work
	// k-456 was removed in Step 2, so it should not work
	s.T().Log("Step 7: Verifying removed API key (k-456) no longer work")
	statusWithK456Removed := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-456"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusWithK456Removed,
		expectAPIKeyAuthDenied,
	)
}

// TestAPIKeyAuthRouteOverrideGateway tests that route-level API key auth policy overrides gateway-level policy.
// Gateway-level policy uses one secret (k-123, k-456), while route-level policy uses a different secret (k-789, k-999).
// This verifies that the route-level policy takes precedence and uses its own secret.
func (s *testingSuite) TestAPIKeyAuthRouteOverrideGateway() {
	// Verify HTTPRoute is accepted before running the test
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "httpbin-route-override", "default", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	// Test route with route-level policy override - should use route-level secret (k-789, k-999)
	getReqCurlOpts := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
		curl.WithHostHeader("httpbin"),
		curl.WithPort(8080),
		curl.WithPath("/get"),
	}
	// missing API key, should fail
	s.T().Log("The /get route has route-level API key auth policy, should fail when API key is missing")
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getReqCurlOpts,
		expectAPIKeyAuthDenied,
	)
	// has valid API key from route-level secret, should succeed
	s.T().Log("The /get route should succeed with valid API key from route-level secret (k-789)")
	getWithRouteAPIKeyCurlOpts := append(getReqCurlOpts, curl.WithHeader("api-key", "k-789"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getWithRouteAPIKeyCurlOpts,
		expectStatus200Success,
	)
	// has another valid API key from route-level secret, should succeed
	s.T().Log("The /get route should succeed with another valid API key from route-level secret (k-999)")
	getWithRouteAPIKey2CurlOpts := append(getReqCurlOpts, curl.WithHeader("api-key", "k-999"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getWithRouteAPIKey2CurlOpts,
		expectStatus200Success,
	)
	// has API key from gateway-level secret, should fail (route-level policy overrides)
	s.T().Log("The /get route should fail with API key from gateway-level secret (k-123) - route-level policy overrides")
	getWithGatewayAPIKeyCurlOpts := append(getReqCurlOpts, curl.WithHeader("api-key", "k-123"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getWithGatewayAPIKeyCurlOpts,
		expectAPIKeyAuthDenied,
	)
	// has another API key from gateway-level secret, should fail
	s.T().Log("The /get route should fail with another API key from gateway-level secret (k-456) - route-level policy overrides")
	getWithGatewayAPIKey2CurlOpts := append(getReqCurlOpts, curl.WithHeader("api-key", "k-456"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getWithGatewayAPIKey2CurlOpts,
		expectAPIKeyAuthDenied,
	)

	// Test route without route-level policy - should use gateway-level secret (k-123, k-456)
	statusReqCurlOpts := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
		curl.WithHostHeader("httpbin"),
		curl.WithPort(8080),
		curl.WithPath("/status/200"),
	}
	// missing API key, should fail (gateway-level policy applies)
	s.T().Log("The /status/200 route has no route-level policy, should require API key from gateway-level policy")
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusReqCurlOpts,
		expectAPIKeyAuthDenied,
	)
	// has valid API key from gateway-level secret, should succeed
	s.T().Log("The /status/200 route should succeed with valid API key from gateway-level secret (k-123)")
	statusWithGatewayAPIKeyCurlOpts := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-123"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusWithGatewayAPIKeyCurlOpts,
		expectStatus200Success,
	)
	// has another valid API key from gateway-level secret, should succeed
	s.T().Log("The /status/200 route should succeed with another valid API key from gateway-level secret (k-456)")
	statusWithGatewayAPIKey2CurlOpts := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-456"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusWithGatewayAPIKey2CurlOpts,
		expectStatus200Success,
	)
	// has API key from route-level secret, should fail (only applies to /get route)
	s.T().Log("The /status/200 route should fail with API key from route-level secret (k-789) - only gateway-level policy applies")
	statusWithRouteAPIKeyCurlOpts := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-789"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusWithRouteAPIKeyCurlOpts,
		expectAPIKeyAuthDenied,
	)
}

// TestAPIKeyAuthDisableAtRouteLevel tests the NEW disable field feature
// This test verifies that API key auth can be disabled at route level to override gateway-level policy
func (s *testingSuite) TestAPIKeyAuthDisableAtRouteLevel() {
	// Verify HTTPRoute is accepted before running the test
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "httpbin-route-disable", "default", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	// Test /status/200 route - has gateway-level policy, no route-level override
	statusReqCurlOpts := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
		curl.WithHostHeader("httpbin"),
		curl.WithPort(8080),
		curl.WithPath("/status/200"),
	}
	// missing API key, should fail (gateway-level policy applies)
	s.T().Log("The /status/200 route has gateway-level API key auth, should fail without API key")
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusReqCurlOpts,
		expectAPIKeyAuthDenied,
	)
	// has valid API key, should succeed
	s.T().Log("The /status/200 route should succeed with valid API key from gateway-level policy")
	statusWithAPIKeyCurlOpts := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-123"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		statusWithAPIKeyCurlOpts,
		expectStatus200Success,
	)

	// Test /get route - has disable: {} at route level, should NOT require API key
	getReqCurlOpts := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
		curl.WithHostHeader("httpbin"),
		curl.WithPort(8080),
		curl.WithPath("/get"),
	}
	// missing API key, should SUCCEED (disable field overrides gateway-level policy)
	s.T().Log("The /get route has disable: {} at route level, should succeed without API key (NEW FEATURE)")
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getReqCurlOpts,
		expectStatus200Success,
	)
	// has API key, should still succeed (API key is ignored when disabled)
	s.T().Log("The /get route with disable should succeed even with API key present")
	getWithAPIKeyCurlOpts := append(getReqCurlOpts, curl.WithHeader("api-key", "k-123"))
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		getWithAPIKeyCurlOpts,
		expectStatus200Success,
	)
}
