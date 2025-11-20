//go:build e2e

package remotejwtauth

import (
	"context"
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

//
// Use `go run hack/utils/jwt/jwt-generator.go`
// to generate jwks and a jwt signed by the key in it
//

var _ e2e.NewSuiteFunc = NewTestingSuite

const (
	namespace = "default"
	// jwt subject is "ignore@kgateway.dev"
	// could also retrieve these jwts from  https://dummy-idp.default:8443/org-one/jwt, https://dummy-idp.default:8443/org-two/jwt, and https://dummy-idp.default:8443/org-three/jwt  respectively
	jwtOrgOne = "eyJhbGciOiJSUzI1NiIsImtpZCI6IjUzNTAyMzEyMTkzMDYwMzg2OTIiLCJ0eXAiOiJKV1QifQ.eyJpc3MiOiJodHRwczovL2tnYXRld2F5LmRldiIsInN1YiI6Imlnbm9yZUBrZ2F0ZXdheS5kZXYiLCJleHAiOjIwNzExNjM0MDcsIm5iZiI6MTc2MzU3OTQwNywiaWF0IjoxNzYzNTc5NDA3fQ.TsHCCdd0_629wibU4EviEi1-_UXaFUX1NuLgXCrC-tr7kqlcnUJIJC0WSab1EgXKtF8gTfwTUeQcAQNrunwngQU-K9DFcH5-2vnGeiXV3_X3SokkPq74ceRrCFEL2d7YNaGfhq_UNyvKRJsRz-pwdKK7QIPXALmWaUHn7EV7zU-CcPCKNwmt62P88qNp5HYSbgqz_WfnzIIH8LANpCC8fUqVedgTJMJ86E06pfDNUuuXe_fhjgMQXlfyDeUxIuzJunvS2qIqt4IYMzjcQbl2QI1QK3xz37tridSP_WVuuMUe2Lqo0oDjWVpxqPb5fb90W6a6khRP59Pf6qKMbQ9SQg"
	jwtOrgTwo = "eyJhbGciOiJSUzI1NiIsImtpZCI6IjI4OTk1NjQyMzcyMTQ2ODQ5NDciLCJ0eXAiOiJKV1QifQ.eyJpc3MiOiJodHRwczovL2tnYXRld2F5LmRldiIsInN1YiI6Imlnbm9yZUBrZ2F0ZXdheS5kZXYiLCJleHAiOjIwNzExNjM1MzIsIm5iZiI6MTc2MzU3OTUzMiwiaWF0IjoxNzYzNTc5NTMyfQ.kLazcb2o_zcVfJ7WECsQJdOaluxAJ-GdOkeuXUOJSeN8PvahjxfpftgeJjcGsp2sl-VIKXIuTLH6csHT_CBq7kI8bVKGDkk8qw3w8gem7MtiXKPMSYiYEHAoCCzsl8O-pGPF6G_PU-CfiWla8CIAjOewLzRmLeAYmwEiUYf8LQ7y6BbVDzvtxIQW3pTurHXFy0TZ6nUGqu_Xwh7uXe42WC0T-9LAI4zsGo5x_FKhlE_6N9_a7R0UIYFeRrbph_b1z47xTZ3YhZBmQmue2j1xR6hwRCnL7mOaCrxdte8SqXNUVA6vPSaiMTSkdmKyeRSzeTliDKiqAmP8eiIaqAoN5A"
)

var (
	proxyObjectMeta = metav1.ObjectMeta{Name: "super-gateway", Namespace: namespace}

	setup = base.TestCase{
		Manifests: []string{
			getTestFile("common.yaml"),
			getTestFile("service.yaml"),
			testdefaults.CurlPodManifest,
		},
	}

	testCases = map[string]*base.TestCase{
		"TestRoutePolicy": {
			Manifests: []string{insecureRouteManifest, secureRoutePolicyManifest},
		},
		"TestRoutePolicyWithRbac": {
			Manifests: []string{secureRoutePolicyWithRbacManifest},
		},
		"TestGatewayPolicy": {
			Manifests: []string{secureGWPolicyManifest},
		},
		"TestGatewayPolicyWithRbac": {
			Manifests: []string{secureGWPolicyWithRbacManifest},
		},
	}
)

type testingSuite struct {
	*base.BaseTestingSuite

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of kgateway
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
		testInstallation: testInst,
	}
}

var (
	insecureRouteManifest             = getTestFile("insecure-route.yaml")
	secureGWPolicyManifest            = getTestFile("secured-gateway-policy.yaml")
	secureGWPolicyWithRbacManifest    = getTestFile("secured-gateway-policy-with-rbac.yaml")
	secureRoutePolicyManifest         = getTestFile("secured-route.yaml")
	secureRoutePolicyWithRbacManifest = getTestFile("secured-route-with-rbac.yaml")
)

func (s *testingSuite) TestRoutePolicy() {
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		"route-example-insecure",
		"default",
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	// verify unprotected route works
	s.assertResponseWithoutAuth("insecureroute.com", http.StatusOK)

	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		"route-secure",
		"default",
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	// verify a provider with a single key in jwks works
	s.assertResponse("secureroute.com", jwtOrgOne, http.StatusOK)
	s.assertResponse("secureroute.com", jwtOrgTwo, http.StatusOK)
	// verify invalid/missing tokens are caught
	s.assertResponse("secureroute.com", "nosuchkey", http.StatusUnauthorized)
	s.assertResponseWithoutAuth("secureroute.com", http.StatusUnauthorized)
}

func (s *testingSuite) TestGatewayPolicy() {
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		"route-secure-gw",
		"default",
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	s.assertResponse("securegateways.com", jwtOrgOne, http.StatusOK)
	s.assertResponse("securegateways.com", jwtOrgTwo, http.StatusOK)
	// verify invalid/missing tokens are caught
	s.assertResponse("securegateways.com", "nosuchkey", http.StatusUnauthorized)
	s.assertResponseWithoutAuth("securegateways.com", http.StatusUnauthorized)
}

func (s *testingSuite) assertResponse(hostHeader, authHeader string, expectedStatus int) {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader(hostHeader),
			curl.WithHeader("Authorization", "Bearer "+authHeader),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: expectedStatus,
		})
}

func (s *testingSuite) assertResponseWithoutAuth(hostHeader string, expectedStatus int) {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader(hostHeader),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: expectedStatus,
		})
}

func getTestFile(filename string) string {
	return filepath.Join(fsutils.MustGetThisDir(), "testdata", filename)
}
