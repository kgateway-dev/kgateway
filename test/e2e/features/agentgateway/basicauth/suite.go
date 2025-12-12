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

const (
	// test namespace for proxy resources
	namespace = "default"
)

var (
	insecureRouteManifest     = getTestFile("insecure-route.yaml")
	secureGwPolicyManifest    = getTestFile("secured-gateway-policy.yaml")
	secureRoutePolicyManifest = getTestFile("secured-route.yaml")

	proxyObjectMeta    = metav1.ObjectMeta{Name: "super-gateway", Namespace: namespace}
	proxyObjectMetaToo = metav1.ObjectMeta{Name: "super-gateway-too", Namespace: namespace}

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
			Manifests: []string{secureGwPolicyManifest},
		},
	}
)

// testingSuite is a suite of global rate limiting tests
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

func (s *testingSuite) TestRoutePolicy() {
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		"route-example-insecure",
		"default",
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	// test unprotected route works
	s.assertResponseWithoutAuth(kubeutils.ServiceFQDN(proxyObjectMeta), "insecureroute.com", http.StatusOK)

	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		"route-secure",
		"default",
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	// test inline username/password store
	s.assertResponse(kubeutils.ServiceFQDN(proxyObjectMeta), "secureroute.com", base64.StdEncoding.EncodeToString(([]byte)("alice:alicepassword")), http.StatusOK)
	s.assertResponse(kubeutils.ServiceFQDN(proxyObjectMeta), "secureroute.com", base64.StdEncoding.EncodeToString(([]byte)("bob:bobpassword")), http.StatusOK)

	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		"route-secure-too",
		"default",
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	// test secret-based username/password store
	s.assertResponse(kubeutils.ServiceFQDN(proxyObjectMeta), "secureroutetoo.com", base64.StdEncoding.EncodeToString(([]byte)("eve:evepassword")), http.StatusOK)
	s.assertResponse(kubeutils.ServiceFQDN(proxyObjectMeta), "secureroutetoo.com", base64.StdEncoding.EncodeToString(([]byte)("mallory:mallorypassword")), http.StatusOK)
	// test invalid username/password combinations
	s.assertResponse(kubeutils.ServiceFQDN(proxyObjectMeta), "secureroute.com", base64.StdEncoding.EncodeToString(([]byte)("alice:boom")), http.StatusUnauthorized)
	s.assertResponse(kubeutils.ServiceFQDN(proxyObjectMeta), "secureroutetoo.com", base64.StdEncoding.EncodeToString(([]byte)("eve:boom")), http.StatusUnauthorized)
	s.assertResponse(kubeutils.ServiceFQDN(proxyObjectMeta), "secureroute.com", base64.StdEncoding.EncodeToString(([]byte)("trent:boom")), http.StatusUnauthorized)
	s.assertResponseWithoutAuth(kubeutils.ServiceFQDN(proxyObjectMeta), "secureroute.com", http.StatusUnauthorized)
}

func (s *testingSuite) TestGatewayPolicy() {
	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		"route-secure-gw",
		"default",
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	// test inline user/password store
	s.assertResponse(kubeutils.ServiceFQDN(proxyObjectMeta), "securegateways.com", base64.StdEncoding.EncodeToString(([]byte)("alice:alicepassword")), http.StatusOK)
	s.assertResponse(kubeutils.ServiceFQDN(proxyObjectMeta), "securegateways.com", base64.StdEncoding.EncodeToString(([]byte)("bob:bobpassword")), http.StatusOK)

	s.TestInstallation.Assertions.EventuallyHTTPRouteCondition(
		s.Ctx,
		"route-secure-gw-too",
		"default",
		gwv1.RouteConditionAccepted,
		metav1.ConditionTrue,
	)
	// test secret-based user/password store
	s.assertResponse(kubeutils.ServiceFQDN(proxyObjectMetaToo), "securegatewaystoo.com", base64.StdEncoding.EncodeToString(([]byte)("eve:evepassword")), http.StatusOK)
	s.assertResponse(kubeutils.ServiceFQDN(proxyObjectMetaToo), "securegatewaystoo.com", base64.StdEncoding.EncodeToString(([]byte)("mallory:mallorypassword")), http.StatusOK)
	// test invalid username/password combinations
	s.assertResponse(kubeutils.ServiceFQDN(proxyObjectMeta), "securegateways.com", base64.StdEncoding.EncodeToString(([]byte)("alice:boom")), http.StatusUnauthorized)
	s.assertResponse(kubeutils.ServiceFQDN(proxyObjectMeta), "securegateways.com", base64.StdEncoding.EncodeToString(([]byte)("trent:boom")), http.StatusUnauthorized)
	s.assertResponse(kubeutils.ServiceFQDN(proxyObjectMetaToo), "securegatewaystoo.com", base64.StdEncoding.EncodeToString(([]byte)("trent:boom")), http.StatusUnauthorized)
	s.assertResponseWithoutAuth(kubeutils.ServiceFQDN(proxyObjectMeta), "securegateways.com", http.StatusUnauthorized)
	s.assertResponseWithoutAuth(kubeutils.ServiceFQDN(proxyObjectMetaToo), "securegatewaystoo.com", http.StatusUnauthorized)
}

func (s *testingSuite) assertResponse(host, hostHeader, authHeader string, expectedStatus int) {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(host),
			curl.WithHostHeader(hostHeader),
			curl.WithHeader("Authorization", "Basic "+authHeader),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: expectedStatus,
		})
}

func (s *testingSuite) assertResponseWithoutAuth(host, hostHeader string, expectedStatus int) {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(host),
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
