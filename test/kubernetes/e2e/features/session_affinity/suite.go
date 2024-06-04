package session_affinity

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/skv2/codegen/util"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// testingSuite is the entire Suite of tests for the "example" feature
// Typically, we would include a link to the feature code here
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	ti *e2e.TestInstallation

	// maps test name to a list of manifests to apply before the test
	manifests       map[string][]string
	manifestObjects map[string][]client.Object
}

const (
	// ref: test/kubernetes/e2e/features/delegation/testdata/common.yaml
	gatewayPort = 8080
)

var (
	proxyMeta = metav1.ObjectMeta{
		Name:      "gloo-proxy-http-gateway",
		Namespace: "infra",
	}

	proxyDeployment = &appsv1.Deployment{ObjectMeta: proxyMeta}
	proxyService    = &corev1.Service{ObjectMeta: proxyMeta}
	proxyHostPort   = fmt.Sprintf("%s.%s.svc:%d", proxyService.Name, proxyService.Namespace, gatewayPort)
	CurlPodExecOpt  = kubectl.PodExecOptions{
		Name:      "curl",
		Namespace: "curl",
		Container: "curl",
	}
)

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx: ctx,
		ti:  testInst,
	}
}

func (s *testingSuite) SetupSuite() {
	s.manifests = map[string][]string{
		"TestStatefulSession": {sessionAffinityManifest},
	}

}

func (s *testingSuite) TearDownSuite() {
	// This is code that will be executed after an entire suite is run
}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	manifests := s.manifests[testName]
	for _, manifest := range manifests {
		err := s.ti.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
		s.ti.Assertions.EventuallyObjectsExist(s.ctx, s.manifestObjects[manifest]...)
	}
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	manifests := s.manifests[testName]
	for _, manifest := range manifests {
		err := s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
		s.Require().NoError(err)
		s.ti.Assertions.EventuallyObjectsNotExist(s.ctx, s.manifestObjects[manifest]...)
	}
}

func (s *testingSuite) TestExampleAssertion() {
	// Testify assertion
	s.Assert().NotEqual(1, 2, "1 does not equal 2")

	// Testify assertion, using the TestInstallation to provide it
	s.ti.Assertions.Require.NotEqual(1, 2, "1 does not equal 2")

	// Gomega assertion, using the TestInstallation to provide it
	s.ti.Assertions.Gomega.Expect(1).NotTo(Equal(2), "1 does not equal 2")
}

var (
	sessionAffinityManifest = filepath.Join(util.MustGetThisDir(), "testdata", "session_affinity.yaml")
)

func (s *testingSuite) TestStatefulSession() {
	numRequests := 10
	// XProtocol:          "http",
	// XPath:              "/count",
	// XMethod:            "GET",
	// XHost:              "app",
	// XService:           gatewayProxy, // service overrides host?
	// XPort:              gatewayPort,
	// XConnectionTimeout: 1,
	// XWithoutStats:      true,
	// NALogResponses:      true,
	//X Cookie:            "cookie.txt",
	//X CookieJar:         "cookie.txt",
	//X Verbose:           true,

	curlOpts := []curl.Option{
		//curl.VerboseOutput(),
		//curl.WithHostPort(proxyHostPort),
		curl.WithCookie("/tmp/cookie.txt"),
		curl.WithCookieJar("/tmp/cookie.txt"),
		curl.WithPath("/count"),
		curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: s.ti.Metadata.InstallNamespace})),
		//curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
		curl.WithPort(80),
		//curl.WithConnectionTimeout(1),
		//curl.WithMethod("GET"),
		//curl.WithScheme("http"),
		curl.Silent(),
		curl.WithHostHeader("app"),
	}

	// Get the first response - this one we may have to wait for
	s.ti.Assertions.AssertEventualCurlResponse(
		s.ctx,
		CurlPodExecOpt,
		curlOpts,
		&matchers.HttpResponse{StatusCode: http.StatusOK, Body: "1"},
		4*time.Second,
	)

	//
	for i := 2; i <= numRequests; i++ {
		s.ti.Assertions.AssertCurlResponse(
			s.ctx,
			CurlPodExecOpt,
			curlOpts,
			&matchers.HttpResponse{StatusCode: http.StatusOK, Body: strconv.Itoa(i)},
		)
	}

}
