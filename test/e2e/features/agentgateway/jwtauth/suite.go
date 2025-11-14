//go:build e2e

package jwtauth

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
	jwt1      = "eyJhbGciOiJSUzI1NiIsImtpZCI6IjUzMzM3ODA2ODc1NTEwMzg2NTkiLCJ0eXAiOiJKV1QifQ.eyJpc3MiOiJodHRwczovL2tnYXRld2F5LmRldiIsInN1YiI6Imlnbm9yZUBrZ2F0ZXdheS5kZXYiLCJleHAiOjIwNzA3MjY3MjgsIm5iZiI6MTc2MzE0MjcyOCwiaWF0IjoxNzYzMTQyNzI4fQ.q88gLzLe6VzRnI0VC4luX7OebX3LW6OLTOOwscGofccnipqfVAi2onHNZt08St5QZ6sTm7kaIc2jLGcr2mey9TjXS5pWiV6wgIN4vZp96-G_2GXcOdTZwWvBQzhnDRLyEKQV-3tU2LTIN_9f5TgQTgZHzXtdhP4Pa3fOSzlM_Rc0ly0sRxkI0JV6WbvhW4OZT6ZT8jbaU5iTRDIf0p1R7mJS6H9g6JMYBf_7LibhiUIosHJCJFgYMEh51JvvEHSBcJrE_Snt37QPznMuK_krtHDszeJvKNs76bSioK6MBdMn7T2GXqkCxy4I46fP4hv6kehQ5abJhXHE8Lwu5NejKg"
	jwt2      = "eyJhbGciOiJSUzI1NiIsImtpZCI6IjcxMDU3OTM5NTUwODY5Mzk2NjQiLCJ0eXAiOiJKV1QifQ.eyJpc3MiOiJodHRwczovL2tnYXRld2F5LmRldiIsInN1YiI6Imlnbm9yZUBrZ2F0ZXdheS5kZXYiLCJleHAiOjIwNzA3MjY5NTUsIm5iZiI6MTc2MzE0Mjk1NSwiaWF0IjoxNzYzMTQyOTU1fQ.HmBlsqTSC-ZW1L_pnCB_ix7zorIiyg3X_mD8DiPSaZoKHVCJ-sjmUzffxUzINs4_kzglMWYvOeVsHg1YCASn0_9gBVQ5UvZo1lDZSachuqUGReJ4Bneovjdh18T0FjMJFMy-1K8Bp1RMGlSe4EgBj1lJEA-9h-mFXJv9kC_udD8UJtk-BwJbO9OoUFAbuvaWDdMblVFGKuFSVtZthvyMfFsvgjdkuKBYjeyi9ha1cpWxdV3IOdLjOigdqVkHh9s1Agyki1aVMuleqZUlkOgxaaxzHRjxcIMt7MBB0vQZ9pmItiHMBAyc6u9j4WzaKzgZ58zant48T9vqgci6rcnLBQ"
	jwt3      = "eyJhbGciOiJSUzI1NiIsImtpZCI6IjgxNDAyMjc3NTQyNDQ3Mzk0MzEiLCJ0eXAiOiJKV1QifQ.eyJpc3MiOiJodHRwczovL2tnYXRld2F5LmRldiIsInN1YiI6Imlnbm9yZUBrZ2F0ZXdheS5kZXYiLCJleHAiOjIwNzA3MjcwMjYsIm5iZiI6MTc2MzE0MzAyNiwiaWF0IjoxNzYzMTQzMDI2fQ.QNRDPZRFxI_GP1UED3iowjTC8IkuNynvDhALAYb3Rx8kuwaExe6slWNFZwLiBDOEbPJ5-sZfp_aA6l0_KIWBigg0Fsa1eS82Ax9_3YEeFJz6i9vItY4xcXFfL4vTZtmkaNWd1wb2lPsDu6jQsfm6hPTOGk9WHRax2tR8J87sgjQODvCNeZRl3GVH4G-ciDIf-Jo81C_GmoT-UI97ZQ7v7e6GFtsMc1aSyOaiqYGxOvulpTtALy41YQtKO8S07pSGdhuJcJyz-9waZHRe-CSnWsOAsU2Ft7t0X-2rzsGKYn-iASfMNmleUHhqUOLQ1e6JheXBu5VwoPGiZfaHUVXmKA"
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
		"TestGatewayPolicy": {
			Manifests: []string{secureGWPolicyManifest},
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
	insecureRouteManifest     = getTestFile("insecure-route.yaml")
	secureGWPolicyManifest    = getTestFile("secured-gateway-policy.yaml")
	secureRoutePolicyManifest = getTestFile("secured-route.yaml")
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
	s.assertResponse("secureroute.com", jwt1, http.StatusOK)
	// verify a provider with multiple keys in jwks works
	s.assertResponse("secureroute.com", jwt2, http.StatusOK)
	s.assertResponse("secureroute.com", jwt3, http.StatusOK)
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
	// verify a provider with a single key in jwks works
	s.assertResponse("securegateways.com", jwt1, http.StatusOK)
	// verify a provider with multiple keys in jwks works
	s.assertResponse("securegateways.com", jwt2, http.StatusOK)
	s.assertResponse("securegateways.com", jwt3, http.StatusOK)
	s.assertResponse("securegateways.com", "nosuchkey", http.StatusUnauthorized)
	// verify invalid/missing tokens are caught
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
