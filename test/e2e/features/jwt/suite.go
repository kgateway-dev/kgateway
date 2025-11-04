//go:build e2e

package jwt

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var (
	// manifests
	setupManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "setup.yaml")
	jwtManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "jwt.yaml")

	// Core infrastructure objects that we need to track
	gatewayObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	gatewayService    = &corev1.Service{ObjectMeta: gatewayObjectMeta}
	gatewayDeployment = &appsv1.Deployment{ObjectMeta: gatewayObjectMeta}

	httpbinObjectMeta = metav1.ObjectMeta{
		Name:      "httpbin",
		Namespace: "default",
	}
	httpbinDeployment = &appsv1.Deployment{ObjectMeta: httpbinObjectMeta}

	// Matches
	expectedJwtMissingFailedResponse = &matchers.HttpResponse{
		StatusCode: http.StatusUnauthorized,
		Body:       gomega.ContainSubstring("Jwt is missing"),
	}
	expectedJwtVerificationFailedResponse = &matchers.HttpResponse{
		StatusCode: http.StatusUnauthorized,
		Body:       gomega.ContainSubstring("Jwt verification fails"),
	}

	expectStatus200Success = &matchers.HttpResponse{
		StatusCode: http.StatusOK,
		Body:       nil,
	}

	// invalid jwt (not signed with correct key)
	badJwtToken = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJodHRwczovL2Rldi5leGFtcGxlLmNvbSIsImV4cCI6NDgwNDMyNDczNiwiaWF0IjoxNjQ4NjUxMTM2LCJvcmciOiJpbnRlcm5hbCIsImVtYWlsIjoiZGV2MkBrZ2F0ZXdheS5pbyIsImdyb3VwIjoiZW5naW5lZXJpbmciLCJzY29wZSI6ImlzOmRldmVsb3BlciJ9.pduAl6C0YofLSTUNcQuSd5dvrN-B8eE0pbOJJ9h5Fyh-k1HQQzSpZ47HJngclFmfcWk25qyJfLOnuVuA4PV6PwanPovL5YpdLlAbjHZPfDwsR1v8zUzb97yl-hbQzYCiA8coHO6rQE8hOYD59-DXkH6acuU8nVm3sv6VUA8zR5XpxZfJHJfRu8TZUFowk3FFrdh3nUSeeXLtm0YxN9uVEHKe3v_UEdMBUzri7wC1saKy7CcpikpBwd7itPMpT87BL_f1LvJf7LUEChRC-sp2LYsyjT-rme4YufPp1vVi5dMSCpfmvB1XlgFKzmGBPKvDJPta1DNOmHqEmKmgOQBCmw"

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
	dev1JwtToken = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJodHRwczovL2Rldi5leGFtcGxlLmNvbSIsImV4cCI6NDgwNDMyNDczNiwiaWF0IjoxNjQ4NjUxMTM2LCJvcmciOiJpbnRlcm5hbCIsImVtYWlsIjoiZGV2MUBrZ2F0ZXdheS5pbyIsImdyb3VwIjoiZW5naW5lZXJpbmciLCJzY29wZSI6ImlzOmRldmVsb3BlciJ9.pqzk87Gny6mT8Gk7CVfkminm3u9CrNPhRt0oElwmfwZ7Jak1Ss4iOZ7MSZEgZFPxGiaz3DQyvos65dqbM_e4RaLYXb9fFYylaBl8kE8bhqMnXfPBNp9C4XTsSz4mR-eUvnkXXZ31dhMkoZvwIswWXR50wZ0rC6NF60Tye0sHJRdDcwL5778wDzLnualvtIiL-CbhWzXgRmjcrK3sbikLCHBjQiTEyBMPOVqS5NqJBgd7ZW1UASoxuxjCLsN8tBIaAFSACf8FZggAh9vEUJ_uc39kvOKQ0vs0pxvoYtsMPcndBYhws6IUhx_iF__qs_zz9mDNp8aMbXSlEdJG30wiRA"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of tests for jwt functionality
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of kgateway
	testInstallation *e2e.TestInstallation

	// maps test name to a list of manifests to apply before the test
	manifests map[string][]string

	// Track core objects for cleanup
	coreObjects []client.Object
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

// SetupSuite runs before all tests in the suite
func (s *testingSuite) SetupSuite() {
	// Initialize test manifest mappings
	s.manifests = map[string][]string{
		"TestJwtAuthentication": {jwtManifest},
	}

	// Apply core infrastructure
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.Require().NoError(err)

	// Apply curl pod for testing
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.HttpbinManifest)
	s.Require().NoError(err)

	// Apply curl pod for testing
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.CurlPodManifest)
	s.Require().NoError(err)

	// Track core objects
	s.coreObjects = []client.Object{
		testdefaults.CurlPod,              // curl
		httpbinDeployment,                 // httpbin
		gatewayService, gatewayDeployment, // gateway service
	}

	// Wait for core infrastructure to be ready
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, s.coreObjects...)
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, testdefaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: testdefaults.CurlPodLabelSelector,
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, httpbinDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", httpbinDeployment.GetName()),
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(
		s.ctx,
		gatewayDeployment.ObjectMeta.GetNamespace(),
		metav1.ListOptions{
			LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", gatewayObjectMeta.GetName()),
		},
	)
	s.testInstallation.Assertions.EventuallyHTTPRouteCondition(s.ctx, "httpbin-route", "default", gwv1.RouteConditionAccepted, metav1.ConditionTrue)
}

// TearDownSuite cleans up any remaining resources
func (s *testingSuite) TearDownSuite() {
	// Clean up core infrastructure
	err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, setupManifest)
	s.Require().NoError(err)

	// Clean up curl pod
	err = s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, testdefaults.CurlPodManifest)
	s.Require().NoError(err)

	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, s.coreObjects...)
	s.testInstallation.Assertions.EventuallyPodsNotExist(s.ctx, gatewayObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", gatewayObjectMeta.GetName()),
	})
	s.testInstallation.Assertions.EventuallyPodsNotExist(s.ctx, httpbinObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/name=%s", httpbinObjectMeta.GetName()),
	})
}

// BeforeTest runs before each test
func (s *testingSuite) BeforeTest(suiteName, testName string) {
	manifests := s.manifests[testName]
	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
	}
}

// AfterTest runs after each test
func (s *testingSuite) AfterTest(suiteName, testName string) {
	manifests := s.manifests[testName]
	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err)
	}
}

// TestJwtAuthentication tests the jwt is valid
func (s *testingSuite) TestJwtAuthentication() {
	// Send request to route with no JWT config applied, should get 200 OK
	s.T().Log("send request to route with no JWT config applied, should get 200 OK")
	statusReqCurlOpts := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
		curl.WithHostHeader("httpbin"),
		curl.WithPort(8080),
		curl.WithPath("/status/200"),
	}
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		statusReqCurlOpts,
		expectStatus200Success)

	// The /get route does have a JWT config applied, should get 401 Unauthorized
	s.T().Log("The /get route does have a JWT config applied, should fail when no JWT is provided")
	getReqCurlOpts := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(gatewayService.ObjectMeta)),
		curl.WithHostHeader("httpbin"),
		curl.WithPort(8080),
		curl.WithPath("/get"),
	}
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		getReqCurlOpts,
		expectedJwtMissingFailedResponse)

	s.T().Log("The /get route does have a JWT config applied, should fail when incorrect JWT is provided")
	getReqBadJwtCurlOpts := append(getReqCurlOpts, curl.WithHeader("Authorization", "Bearer "+badJwtToken))
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		getReqBadJwtCurlOpts,
		expectedJwtVerificationFailedResponse,
	)

	s.T().Log("The /get route does have a JWT config applied, should succeed when correct JWT is provided")
	getReqJwtCurlOpts := append(getReqCurlOpts, curl.WithHeader("Authorization", "Bearer "+dev1JwtToken))
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		getReqJwtCurlOpts,
		expectStatus200Success,
	)
}
