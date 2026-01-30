//go:build e2e

package basicauth

import (
	"context"
	"encoding/base64"
	"net/http"
	"path/filepath"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

func getTestFile(name string) string {
	return filepath.Join(fsutils.MustGetThisDir(), "testdata", name)
}

var (
	proxyObjectMeta = metav1.ObjectMeta{Name: "gw", Namespace: "default"}

	setup = base.TestCase{Manifests: []string{getTestFile("common.yaml"), getTestFile("service.yaml"), testdefaults.CurlPodManifest}}

	testCases = map[string]*base.TestCase{
		"TestTrafficPolicyBasicAuthForRoute":               {Manifests: []string{getTestFile("httproutes.yaml"), getTestFile("tp-route-basicauth.yaml")}},
		"TestTrafficPolicyBasicAuthGatewayOverrideOnRoute": {Manifests: []string{getTestFile("httproutes-no-ext.yaml"), getTestFile("tp-gateway-basicauth.yaml")}},
	}
)

type testingSuite struct{ *base.BaseTestingSuite }

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases)}
}

func (s *testingSuite) TestTrafficPolicyBasicAuthForRoute() {
	// Ensure routes accepted
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "route-secure", "default", gwv1.RouteConditionAccepted, metav1.ConditionTrue)
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "route-public", "default", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	host := kubeutils.ServiceFQDN(proxyObjectMeta)

	// Valid credentials
	s.assertAuthResponse(host, "/", creds("alice", "password"), http.StatusOK)
	s.assertAuthResponse(host, "/", creds("bob", "password"), http.StatusOK)
	s.assertAuthResponse(host, "/secrets", creds("user", "userpassword"), http.StatusOK)

	// Invalid credentials
	s.assertAuthResponse(host, "/", creds("alice", "wrong"), http.StatusUnauthorized)
	s.assertAuthResponse(host, "/secrets", creds("alice", "password"), http.StatusUnauthorized)
	s.assertNoAuthResponse(host, "secure.example.com", http.StatusUnauthorized)

	// Public route should be accessible without auth
	s.assertNoAuthResponse(host, "public.example.com", http.StatusOK)
}

func (s *testingSuite) TestTrafficPolicyBasicAuthGatewayOverrideOnRoute() {
	// Ensure routes accepted
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "route-secure", "default", gwv1.RouteConditionAccepted, metav1.ConditionTrue)
	s.TestInstallation.AssertionsT(s.T()).EventuallyHTTPRouteCondition(s.Ctx, "route-public", "default", gwv1.RouteConditionAccepted, metav1.ConditionTrue)

	host := kubeutils.ServiceFQDN(proxyObjectMeta)

	// Gateway-level basic auth should protect secure.example.com
	s.assertAuthResponse(host, "/", creds("gateway-user", "password"), http.StatusOK)
	s.assertAuthResponse(host, "/", creds("gateway-user", "wrong"), http.StatusUnauthorized)
	s.assertNoAuthResponse(host, "secure.example.com", http.StatusUnauthorized)

	// Route-level disable should allow public.example.com without auth
	s.assertNoAuthResponse(host, "public.example.com", http.StatusOK)
}

func creds(user, pass string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
}

func (s *testingSuite) assertAuthResponse(host, path, authHeader string, expectedStatus int) {
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{curl.WithHost(host), curl.WithHostHeader("secure.example.com"), curl.WithPath(path), curl.WithHeader("Authorization", "Basic "+authHeader), curl.WithPort(8080)},
		&testmatchers.HttpResponse{StatusCode: expectedStatus},
	)
}

func (s *testingSuite) assertNoAuthResponse(host, hostHeader string, expectedStatus int) {
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{curl.WithHost(host), curl.WithHostHeader(hostHeader), curl.WithPort(8080)},
		&testmatchers.HttpResponse{StatusCode: expectedStatus},
	)
}
