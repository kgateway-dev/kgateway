package directresponse

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/gloo/test/kubernetes/e2e/defaults"
	testdefaults "github.com/solo-io/gloo/test/kubernetes/e2e/defaults"
)

type testingSuite struct {
	suite.Suite
	ctx              context.Context
	testInstallation *e2e.TestInstallation
	// maps test name to a list of manifests to apply before the test
	manifests map[string][]string
}

func NewTestingSuite(
	ctx context.Context,
	testInst *e2e.TestInstallation,
) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	// Check that the common setup manifest is applied
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupManifest)
	s.NoError(err, "can apply "+setupManifest)
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.CurlPodManifest)
	s.NoError(err, "can apply curl pod manifest")

	// Check that istio injection is successful and httpbin is running
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, httpbinDeployment)
	// httpbin can take a while to start up with Istio sidecar
	s.testInstallation.Assertions.EventuallyPodsRunning(
		s.ctx,
		httpbinDeployment.ObjectMeta.GetNamespace(),
		metav1.ListOptions{LabelSelector: "app=httpbin"},
		time.Minute*2,
	)

	// include gateway manifests for the tests, so we recreate it for each test run
	s.manifests = map[string][]string{
		"TestBasicDirectResponse":        {gatewayManifest, basicDirectResposeManifests},
		"TestDelegation":                 {gatewayManifest, basicDelegationManifests},
		"TestInvalidDirectResponseRoute": {gatewayManifest, invalidDirectResponseManifests},
	}
}

func (s *testingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, setupManifest)
	s.NoError(err, "can delete setup manifest")
	err = s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, testdefaults.CurlPodManifest)
	s.NoError(err, "can delete curl pod manifest")
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, httpbinDeployment)
}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for %s, manifest map contents: %v", testName, s.manifests)
	}
	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Assert().NoError(err, "can apply manifest "+manifest)
	}

	// we recreate the `Gateway` resource (and thus dynamically provision the proxy pod) for each test run
	// so let's assert the proxy svc and pod is ready before moving on
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gloo-proxy-gw",
	})
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for " + testName)
	}

	for _, manifest := range manifests {
		output, err := s.testInstallation.Actions.Kubectl().DeleteFileWithOutput(s.ctx, manifest)
		s.testInstallation.Assertions.ExpectObjectDeleted(manifest, err, output)
	}
}

func (s *testingSuite) TestBasicDirectResponse() {
	// verify that a direct response route works as expected
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(glooProxyObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/robots.txt"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       ContainSubstring("Disallow: /custom"),
		},
		time.Minute,
	)
}

func (s *testingSuite) TestDelegation() {
	// verify that a direct response route works as expected
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(glooProxyObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/ip"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusNotFound,
			Body:       ContainSubstring(`/ip is not supported`),
		},
		time.Minute,
	)
}

func (s *testingSuite) TestInvalidDirectResponseRoute() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(glooProxyObjectMeta)),
			curl.WithHostHeader("www.example.com"),
			curl.WithPath("/non-existent"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusInternalServerError,
		},
		time.Minute,
	)
}
