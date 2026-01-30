//go:build e2e

package jwt

import (
	"context"
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/features/agentgateway/remotejwtauth"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var (
	// manifests
	setupManifest        = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	jwtManifest          = filepath.Join(fsutils.MustGetThisDir(), "testdata", "jwt.yaml")
	jwtRbacManifest      = filepath.Join(fsutils.MustGetThisDir(), "testdata", "jwt-rbac.yaml")
	jwtHTTPRouteManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "jwt-httproute.yaml")
	jwtDisableManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "jwt-disable.yaml")
	jwtRemoteManifest    = filepath.Join(fsutils.MustGetThisDir(), "testdata", "jwt-remote.yaml")

	gatewayObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}

	// Matches
	expectedJwtMissingFailedResponse = &matchers.HttpResponse{
		StatusCode: http.StatusUnauthorized,
		Body:       gomega.ContainSubstring("Jwt is missing"),
	}
	expectedJwtVerificationFailedResponse = &matchers.HttpResponse{
		StatusCode: http.StatusUnauthorized,
		Body:       gomega.ContainSubstring("Jwt verification fails"),
	}

	expectedJwtIssuerNotConfigured = &matchers.HttpResponse{
		StatusCode: http.StatusUnauthorized,
		Body:       gomega.ContainSubstring("Jwt issuer is not configured"),
	}

	expectStatus200Success = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       nil,
	}

	expectRbacDeniedWithJwt = &matchers.HttpResponse{
		StatusCode: http.StatusForbidden,
		Body:       gomega.ContainSubstring("RBAC: access denied"),
	}

	// invalid jwt (not signed with correct key)
	badJwtToken = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJodHRwczovL2Rldi5leGFtcGxlLmNvbSIsImV4cCI6NDgwNDMyNDczNiwiaWF0IjoxNjQ4NjUxMTM2LCJvcmciOiJpbnRlcm5hbCIsImVtYWlsIjoiZGV2MkBrZ2F0ZXdheS5pbyIsImdyb3VwIjoiZW5naW5lZXJpbmciLCJzY29wZSI6ImlzOmRldmVsb3BlciJ9.pduAl6C0YofLSTUNcQuSd5dvrN-B8eE0pbOJJ9h5Fyh-k1HQQzSpZ47HJngclFmfcWk25qyJfLOnuVuA4PV6PwanPovL5YpdLlAbjHZPfDwsR1v8zUzb97yl-hbQzYCiA8coHO6rQE8hOYD59-DXkH6acuU8nVm3sv6VUA8zR5XpxZfJHJfRu8TZUFowk3FFrdh3nUSeeXLtm0YxN9uVEHKe3v_UEdMBUzri7wC1saKy7CcpikpBwd7itPMpT87BL_f1LvJf7LUEChRC-sp2LYsyjT-rme4YufPp1vVi5dMSCpfmvB1XlgFKzmGBPKvDJPta1DNOmHqEmKmgOQBCmw" //gosec:disable G101

	/*
		Configured with these fields:
			{
			  "iss": "https://dev.example.com",
			  "exp": 4804324736,
			  "iat": 1648651136,
			  "org": "internal",
			  "email": "dev1@kgateway.io",
			  "group": "engineering",
			  "scope": "is:developer"
			}
		Using https://jwt.io/ and the following instructions to generate a public/private key pair:
		1. openssl genrsa 2048 > private-key.pem
		2. openssl rsa -in private-key.pem -pubout
		3. cat private-key.pem | pbcopy
	*/
	// claim has email=dev1@kgateway.io
	dev1JwtToken = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJodHRwczovL2Rldi5leGFtcGxlLmNvbSIsImV4cCI6NDgwNDMyNDczNiwiaWF0IjoxNjQ4NjUxMTM2LCJvcmciOiJpbnRlcm5hbCIsImVtYWlsIjoiZGV2MUBrZ2F0ZXdheS5pbyIsImdyb3VwIjoiZW5naW5lZXJpbmciLCJzY29wZSI6ImlzOmRldmVsb3BlciJ9.pqzk87Gny6mT8Gk7CVfkminm3u9CrNPhRt0oElwmfwZ7Jak1Ss4iOZ7MSZEgZFPxGiaz3DQyvos65dqbM_e4RaLYXb9fFYylaBl8kE8bhqMnXfPBNp9C4XTsSz4mR-eUvnkXXZ31dhMkoZvwIswWXR50wZ0rC6NF60Tye0sHJRdDcwL5778wDzLnualvtIiL-CbhWzXgRmjcrK3sbikLCHBjQiTEyBMPOVqS5NqJBgd7ZW1UASoxuxjCLsN8tBIaAFSACf8FZggAh9vEUJ_uc39kvOKQ0vs0pxvoYtsMPcndBYhws6IUhx_iF__qs_zz9mDNp8aMbXSlEdJG30wiRA" //gosec:disable G101

	// claim has email=dev2@kgateway.io
	dev2JwtToken = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJodHRwczovL2Rldi5leGFtcGxlLmNvbSIsImV4cCI6NDgwNDMyNDczNiwiaWF0IjoxNjQ4NjUxMTM2LCJvcmciOiJpbnRlcm5hbCIsImVtYWlsIjoiZGV2MkBrZ2F0ZXdheS5pbyIsImdyb3VwIjoiZW5naW5lZXJpbmciLCJzY29wZSI6ImlzOmRldmVsb3BlciJ9.S0a_Lu2y0gaXBCnO3ydGJCnXt5R-QMxBvJOjYOTzorcnUOcaOTMOd3fUBY8ojZR-f0xTEy6M6K1V0yKxeq6Mys9Le9SE6oabP6gttktnwL5c9e9rzMcmGz1NVyUBav2N8Yiuw7Va8gyIod02vJrllQteMfZSqoAUmDLmpFs3bvkIgMlWDtVAWPqoGJ4ZI-yf0WfTSmW-kFbaiIz4pQNm03Q9M_ZMiHyOTtCDZuc0pSQ0_uvjnqHrefBgJJkFEv58pVqZVJphEOAfl7CpWlT9dXiPVoMhy4RTezkfrjuCqvW7dDwGZGSUqLYDZsOJ8yeIdeW9LKMaGcPag1AbRCe4HQ" //gosec:disable G101

	setup = base.TestCase{
		Manifests: []string{
			setupManifest,
			testdefaults.HttpbinManifest,
			testdefaults.CurlPodManifest,
		},
	}

	testCases = map[string]*base.TestCase{
		"TestJwtAuthentication": {
			Manifests: []string{jwtManifest},
		},
		"TestJwtAuthenticationHTTPRoute": {
			Manifests: []string{jwtHTTPRouteManifest},
		},
		"TestJwtAuthorization": {
			Manifests: []string{jwtRbacManifest},
		},
		"TestJwtDisable": {
			Manifests: []string{jwtDisableManifest},
		},
		"TestJwtAuthenticationRemote": {
			Manifests: []string{jwtRemoteManifest},
		},
	}
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of tests for jwt functionality
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// TestJwtAuthentication tests the JWT Policy applied at the HTTPRoute rule (extensionRef) level
func (s *testingSuite) TestJwtAuthentication() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx,
		"httpbin-route",
		"default",
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)

	// Send request to route with no JWT config applied, should get 200 OK
	s.T().Log("send request to route with no JWT config applied, should get 200 OK")
	s.assertResponseWithoutAuth("/status/200", expectStatus200Success)

	// The /get route has a JWT config applied, should get 401 Unauthorized
	s.T().Log("The /get route has a JWT config applied, should fail when no JWT is provided")
	s.assertResponseWithoutAuth("/get", expectedJwtMissingFailedResponse)

	s.T().Log("The /get route has a JWT config applied, should fail when incorrect JWT is provided")
	s.assertResponse("/get", badJwtToken, expectedJwtVerificationFailedResponse)

	s.T().Log("The /get route has a JWT config applied, should succeed when correct JWT is provided")
	s.assertResponse("/get", dev1JwtToken, expectStatus200Success)
}

// TestJwtAuthenticationHTTPRoute tests the JWT Policy applied at the HTTPRoute level
func (s *testingSuite) TestJwtAuthenticationHTTPRoute() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx,
		"httpbin-route-get",
		"default",
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)

	// Send request to route with no JWT config applied, should get 200 OK
	s.T().Log("send request to route with no JWT config applied, should get 200 OK")
	s.assertResponseWithoutAuth("/status/200", expectStatus200Success)

	// The /get route has a JWT config applied, should get 401 Unauthorized
	s.T().Log("The /get route has a JWT config applied, should fail when no JWT is provided")
	s.assertResponseWithoutAuth("/get", expectedJwtMissingFailedResponse)

	s.T().Log("The /get route has a JWT config applied, should fail when incorrect JWT is provided")
	s.assertResponse("/get", badJwtToken, expectedJwtVerificationFailedResponse)

	s.T().Log("The /get route has a JWT config applied, should succeed when correct JWT is provided")
	s.assertResponse("/get", dev1JwtToken, expectStatus200Success)
}

// TestJwtAuthorization tests the jwt claims have permissions
func (s *testingSuite) TestJwtAuthorization() {
	// correct JWT, but incorrect claims should be denied
	s.T().Log("The /get route has a JWT applies at the route level, should fail when correct JWT is provided but incorrect claims")
	s.assertResponse("/get", dev1JwtToken, expectRbacDeniedWithJwt)
	// correct JWT is used should result in 200 OK
	s.T().Log("The /get route has a JWT applies at the route level, should succeed when correct JWT is provided with correct claims")
	s.assertResponse("/get", dev2JwtToken, expectStatus200Success)
}

// TestJwtAuthenticationRemote tests the JWT Policy applied at the gateway using a remote JWKS server
func (s *testingSuite) TestJwtAuthenticationRemote() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx,
		"httpbin-route-get",
		"default",
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)

	s.T().Log("status route should fail when no JWT is provided")
	s.assertResponseWithoutAuth("/status/200", expectedJwtMissingFailedResponse)

	s.T().Log("status route should succeed when correct JWT is provided")
	s.assertResponse("/status/200", remotejwtauth.JwtOrgOne, expectStatus200Success)

	s.T().Log("The /get route has a JWT config applied, should fail when no JWT is provided")
	s.assertResponseWithoutAuth("/get", expectedJwtMissingFailedResponse)

	s.T().Log("The /get route has a JWT config applied, should fail when incorrect JWT is provided")
	s.assertResponse("/get", badJwtToken, expectedJwtIssuerNotConfigured)

	s.T().Log("The /get route has a JWT config applied, should succeed when correct JWT is provided")
	s.assertResponse("/get", remotejwtauth.JwtOrgOne, expectStatus200Success)
}

// TestJwtDisable tests that JWT can be disabled at the route level
func (s *testingSuite) TestJwtDisable() {
	// Wait for both routes to be accepted
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx,
		"httpbin-route-jwt",
		"default",
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)

	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(
		s.Ctx,
		"httpbin-route-no-jwt",
		"default",
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)

	// The /get route should require JWT (inherits from gateway policy)
	s.T().Log("The /get route inherits JWT from gateway policy, should fail when no JWT is provided")
	s.assertResponseWithoutAuth("/get", expectedJwtMissingFailedResponse)

	s.T().Log("The /get route does have a JWT config applied, should succeed when correct JWT is provided")
	s.assertResponse("/get", dev1JwtToken, expectStatus200Success)

	// The /status/200 route has JWT disabled, should work without JWT
	s.T().Log("The /status/200 route has JWT disabled, should work without JWT")
	s.assertResponseWithoutAuth("/status/200", expectStatus200Success)
}

func (s *testingSuite) assertResponse(path, authHeader string, expected *matchers.HttpResponse) {
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithPath(path),
			curl.WithHost(kubeutils.ServiceFQDN(gatewayObjectMeta)),
			curl.WithHostHeader("httpbin"),
			curl.WithHeader("Authorization", "Bearer "+authHeader),
			curl.WithPort(8080),
		},
		expected,
	)
}

func (s *testingSuite) assertResponseWithoutAuth(path string, expected *matchers.HttpResponse) {
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithPath(path),
			curl.WithHost(kubeutils.ServiceFQDN(gatewayObjectMeta)),
			curl.WithHostHeader("httpbin"),
			curl.WithPort(8080),
		},
		expected,
	)
}
