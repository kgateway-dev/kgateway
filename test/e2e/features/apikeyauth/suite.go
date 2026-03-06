//go:build e2e

package apikeyauth

import (
	"context"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
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
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "httpbin-route", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	// missing API key, should fail
	s.T().Log("The /status route has API key auth applied at HTTPRoute level, should fail when API key is missing")
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
	)

	// has valid API key, should succeed
	s.T().Log("The /status route has API key auth applied at HTTPRoute level, should succeed when valid API key is present")
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("api-key", "k-123"),
	)

	// has valid API key with Bearer prefix in Authorization header, should succeed
	s.T().Log("The /status route has API key auth applied at HTTPRoute level, should succeed when valid API key is present with Bearer prefix in Authorization header")
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithHeader("Authorization", "Bearer k-123"),
	)

	// missing API key, should fail
	s.T().Log("The /get route has API key auth applied at HTTPRoute level, should fail when API key is missing")
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
	)

	// has valid API key, should succeed
	s.T().Log("The /get route has API key auth applied at HTTPRoute level, should succeed when valid API key is present")
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
		curl.WithHeader("api-key", "k-123"),
	)

	// has valid API key with Bearer prefix in Authorization header, should succeed
	s.T().Log("The /get route has API key auth applied at HTTPRoute level, should succeed when valid API key is present with Bearer prefix in Authorization header")
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
		curl.WithHeader("Authorization", "Bearer k-123"),
	)

	// has valid API key with Bearer prefix using different key, should succeed
	s.T().Log("The /get route has API key auth applied at HTTPRoute level, should succeed when valid API key (k-456) is present with Bearer prefix in Authorization header")
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
		curl.WithHeader("Authorization", "Bearer k-456"),
	)

	// has invalid API key with Bearer prefix in Authorization header, should fail
	s.T().Log("The /get route has API key auth applied at HTTPRoute level, should fail when invalid API key is present with Bearer prefix in Authorization header")
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
		curl.WithHeader("Authorization", "Bearer invalid-key"),
	)
}

// TestAPIKeyAuthWithRouteLevelPolicy tests API key authentication with TrafficPolicy applied at route level (sectionName)
func (s *testingSuite) TestAPIKeyAuthWithRouteLevelPolicy() {
	// Verify HTTPRoute is accepted before running the test
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "httpbin-route", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	s.T().Log("The /status route has no API key auth policy")
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
	)

	// missing API key, should fail
	s.T().Log("The /get route has an API key auth policy applied at the route level, should fail when API key is missing")
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
	)

	// has valid API key, should succeed
	s.T().Log("The /get route has an API key auth policy applied at the route level, should succeed when valid API key is present")
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
		curl.WithHeader("api-key", "k-123"),
	)

	// has invalid API key, should fail
	s.T().Log("The /get route has an API key auth policy applied at the route level, should fail when invalid API key is present")
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
		curl.WithHeader("api-key", "invalid-key"),
	)
}

// TestAPIKeyAuthWithQueryParameter tests API key authentication using query parameter as the key source
func (s *testingSuite) TestAPIKeyAuthWithQueryParameter() {
	// Verify HTTPRoute is accepted before running the test
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "httpbin-route-query", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	// missing API key, should fail
	s.T().Log("The /status route has API key auth with query parameter, should fail when API key is missing")
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
	)

	// has valid API key in query parameter, should succeed
	s.T().Log("The /status route has API key auth with query parameter, should succeed when valid API key is present in query")
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithQueryParameters(map[string]string{"api-key": "k-123"}),
	)

	// missing API key, should fail
	s.T().Log("The /get route has API key auth with query parameter, should fail when API key is missing")
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
	)

	// has valid API key in query parameter, should succeed
	s.T().Log("The /get route has API key auth with query parameter, should succeed when valid API key is present in query")
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
		curl.WithQueryParameters(map[string]string{"api-key": "k-123"}),
	)

	// has invalid API key in query parameter, should fail
	s.T().Log("The /get route has API key auth with query parameter, should fail when invalid API key is present in query")
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
		curl.WithQueryParameters(map[string]string{"api-key": "invalid-key"}),
	)
}

// TestAPIKeyAuthWithCookie tests API key authentication using cookie as the key source
func (s *testingSuite) TestAPIKeyAuthWithCookie() {
	// Verify HTTPRoute is accepted before running the test
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "httpbin-route-cookie", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	// missing API key, should fail
	s.T().Log("The /status route has API key auth with cookie, should fail when API key is missing")
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
	)

	// has valid API key in cookie, should succeed
	s.T().Log("The /status route has API key auth with cookie, should succeed when valid API key is present in cookie")
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
		curl.WithCookie("api-key=k-123"),
	)

	// missing API key, should fail
	s.T().Log("The /get route has API key auth with cookie, should fail when API key is missing")
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
	)

	// has valid API key in cookie, should succeed
	s.T().Log("The /get route has API key auth with cookie, should succeed when valid API key is present in cookie")
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
		curl.WithCookie("api-key=k-123"),
	)

	// has invalid API key in cookie, should fail
	s.T().Log("The /get route has API key auth with cookie, should fail when invalid API key is present in cookie")
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
		curl.WithCookie("api-key=invalid-key"),
	)
}

// TestAPIKeyAuthWithSecretUpdate tests that API key authentication correctly handles secret updates
func (s *testingSuite) TestAPIKeyAuthWithSecretUpdate() {
	// Verify HTTPRoute is accepted before running the test
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "httpbin-route-secret-update", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	statusReqCurlOpts := []curl.Option{
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
	}

	// Step 1: Verify initial API keys work (k-123, k-456)
	s.T().Log("Step 1: Verifying initial API keys (k-123, k-456) work")
	statusWithK123 := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-123"))
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		statusWithK123...,
	)
	statusWithK456 := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-456"))
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		statusWithK456...,
	)

	// Step 2: Update the secret with new keys (k-789, k-999) and remove old ones
	// Note: kubectl apply with stringData merges with existing data, so we need to delete and recreate
	// to properly replace the secret content
	s.T().Log("Step 2: Updating secret with new API keys (k-789, k-999)")
	err := s.TestInstallation.Actions.Kubectl().Delete(s.Ctx, []byte(`apiVersion: v1
kind: Secret
metadata:
  name: api-keys-secret-update
  namespace: kgateway-base
`))
	s.Require().NoError(err, "failed to delete old secret")

	updatedSecretYAML := `apiVersion: v1
kind: Secret
metadata:
  name: api-keys-secret-update
  namespace: kgateway-base
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
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		append(statusReqCurlOpts, curl.WithHeader("api-key", "k-789"))...,
	)

	// Step 3: Verify new keys work
	s.T().Log("Step 3: Verifying new API keys (k-789, k-999) work")
	statusWithK789 := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-789"))
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		statusWithK789...,
	)
	statusWithK999 := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-999"))
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		statusWithK999...,
	)

	// Step 4: Verify old keys no longer work
	s.T().Log("Step 4: Verifying old API keys (k-123, k-456) no longer work")
	statusWithK123Old := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-123"))
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		statusWithK123Old...,
	)
	statusWithK456Old := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-456"))
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		statusWithK456Old...,
	)

	// Step 5: Update secret again to add back an old key and add a new one
	// Note: kubectl apply with stringData merges with existing data
	// The secret will now have: client1 (k-123), client3 (k-789), client4 (k-999), client5 (k-111)
	s.T().Log("Step 5: Updating secret again to add back k-123 and add k-111")
	finalSecretYAML := `apiVersion: v1
kind: Secret
metadata:
  name: api-keys-secret-update
  namespace: kgateway-base
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
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		statusWithK123Final...,
	)
	statusWithK789Final := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-789"))
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		statusWithK789Final...,
	)
	statusWithK999Final := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-999"))
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		statusWithK999Final...,
	)
	statusWithK111Final := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-111"))
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		statusWithK111Final...,
	)

	// Step 7: Verify removed keys no longer work
	// k-456 was removed in Step 2, so it should not work
	s.T().Log("Step 7: Verifying removed API key (k-456) no longer work")
	statusWithK456Removed := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-456"))
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		statusWithK456Removed...,
	)
}

// TestAPIKeyAuthRouteOverrideGateway tests that route-level API key auth policy overrides gateway-level policy.
// Gateway-level policy uses one secret (k-123, k-456), while route-level policy uses a different secret (k-789, k-999).
// This verifies that the route-level policy takes precedence and uses its own secret.
func (s *testingSuite) TestAPIKeyAuthRouteOverrideGateway() {
	// Verify HTTPRoute is accepted before running the test
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "httpbin-route-override", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	// Test route with route-level policy override - should use route-level secret (k-789, k-999)
	getReqCurlOpts := []curl.Option{
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
	}

	// missing API key, should fail
	s.T().Log("The /get route has route-level API key auth policy, should fail when API key is missing")
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		getReqCurlOpts...,
	)

	// has valid API key from route-level secret, should succeed
	s.T().Log("The /get route should succeed with valid API key from route-level secret (k-789)")
	getWithRouteAPIKeyCurlOpts := append(getReqCurlOpts, curl.WithHeader("api-key", "k-789"))
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		getWithRouteAPIKeyCurlOpts...,
	)

	// has another valid API key from route-level secret, should succeed
	s.T().Log("The /get route should succeed with another valid API key from route-level secret (k-999)")
	getWithRouteAPIKey2CurlOpts := append(getReqCurlOpts, curl.WithHeader("api-key", "k-999"))
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		getWithRouteAPIKey2CurlOpts...,
	)

	// has API key from gateway-level secret, should fail (route-level policy overrides)
	s.T().Log("The /get route should fail with API key from gateway-level secret (k-123) - route-level policy overrides")
	getWithGatewayAPIKeyCurlOpts := append(getReqCurlOpts, curl.WithHeader("api-key", "k-123"))
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		getWithGatewayAPIKeyCurlOpts...,
	)

	// has another API key from gateway-level secret, should fail
	s.T().Log("The /get route should fail with another API key from gateway-level secret (k-456) - route-level policy overrides")
	getWithGatewayAPIKey2CurlOpts := append(getReqCurlOpts, curl.WithHeader("api-key", "k-456"))
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		getWithGatewayAPIKey2CurlOpts...,
	)

	// Test route without route-level policy - should use gateway-level secret (k-123, k-456)
	statusReqCurlOpts := []curl.Option{
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
	}

	// missing API key, should fail (gateway-level policy applies)
	s.T().Log("The /status/200 route has no route-level policy, should require API key from gateway-level policy")
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		statusReqCurlOpts...,
	)

	// has valid API key from gateway-level secret, should succeed
	s.T().Log("The /status/200 route should succeed with valid API key from gateway-level secret (k-123)")
	statusWithGatewayAPIKeyCurlOpts := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-123"))
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		statusWithGatewayAPIKeyCurlOpts...,
	)

	// has another valid API key from gateway-level secret, should succeed
	s.T().Log("The /status/200 route should succeed with another valid API key from gateway-level secret (k-456)")
	statusWithGatewayAPIKey2CurlOpts := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-456"))
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		statusWithGatewayAPIKey2CurlOpts...,
	)

	// has API key from route-level secret, should fail (only applies to /get route)
	s.T().Log("The /status/200 route should fail with API key from route-level secret (k-789) - only gateway-level policy applies")
	statusWithRouteAPIKeyCurlOpts := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-789"))
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		statusWithRouteAPIKeyCurlOpts...,
	)
}

// TestAPIKeyAuthDisableAtRouteLevel tests the NEW disable field feature
// This test verifies that API key auth can be disabled at route level to override gateway-level policy
func (s *testingSuite) TestAPIKeyAuthDisableAtRouteLevel() {
	// Verify HTTPRoute is accepted before running the test
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "httpbin-route-disable", "kgateway-base", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	// Test /status/200 route - has gateway-level policy, no route-level override
	statusReqCurlOpts := []curl.Option{
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/status/200"),
	}
	// missing API key, should fail (gateway-level policy applies)
	s.T().Log("The /status/200 route has gateway-level API key auth, should fail without API key")
	common.BaseGateway.Send(
		s.T(),
		expectAPIKeyAuthDenied,
		statusReqCurlOpts...,
	)

	// has valid API key, should succeed
	s.T().Log("The /status/200 route should succeed with valid API key from gateway-level policy")
	statusWithAPIKeyCurlOpts := append(statusReqCurlOpts, curl.WithHeader("api-key", "k-123"))
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		statusWithAPIKeyCurlOpts...,
	)

	// Test /get route - has disable: {} at route level, should NOT require API key
	getReqCurlOpts := []curl.Option{
		curl.WithHostHeader("httpbin"),
		curl.WithPort(80),
		curl.WithPath("/get"),
	}
	// missing API key, should SUCCEED (disable field overrides gateway-level policy)
	s.T().Log("The /get route has disable: {} at route level, should succeed without API key (NEW FEATURE)")
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		getReqCurlOpts...,
	)

	// has API key, should still succeed (API key is ignored when disabled)
	s.T().Log("The /get route with disable should succeed even with API key present")
	getWithAPIKeyCurlOpts := append(getReqCurlOpts, curl.WithHeader("api-key", "k-123"))
	common.BaseGateway.Send(
		s.T(),
		expectStatus200Success,
		getWithAPIKeyCurlOpts...,
	)
}
