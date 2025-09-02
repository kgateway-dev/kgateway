package istio

import (
	"context"
	"net/http"
	"time"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
)

var _ e2e.NewSuiteFunc = NewIstioCustomMtlsSuite

// istioCustomMtlsTestingSuite is the entire Suite of tests for the "Istio" integration cases where auto-mtls is disabled.
// It uses an nginx service as an external nginx.nginx service referenced by the backend.
// The nginx service is configured to have multiple ports:
// - 80: cleartext
// - 443: simple TLS
// - 543: mTLS
type istioCustomMtlsTestingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of kgateway
	testInstallation *e2e.TestInstallation

	// maps test name to a list of manifests to apply before the test
	manifests map[string][]string
}

func NewIstioCustomMtlsSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &istioCustomMtlsTestingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *istioCustomMtlsTestingSuite) BeforeTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for %s, manifest map contents: %v", testName, s.manifests)
	}

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.NoError(err, "can apply "+manifest)
	}

	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment)
	// Check that test resources are running
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyDeployment.ObjectMeta.GetNamespace(),
		metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=gw"}, time.Minute*2)

	// Check that nginx is running
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, nginxDeployment)
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, nginxDeployment.ObjectMeta.GetNamespace(),
		metav1.ListOptions{LabelSelector: "app=nginx"}, time.Minute*2)
}

func (s *istioCustomMtlsTestingSuite) AfterTest(suiteName, testName string) {
	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for " + testName)
	}

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, manifest)
		s.NoError(err, "can delete "+manifest)
	}

	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment)
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, nginxDeployment)
}

func (s *istioCustomMtlsTestingSuite) SetupSuite() {
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, setupNginxMtlsManifest)
	s.NoError(err, "can apply setup manifest")

	s.manifests = map[string][]string{
		"TestCleartextWithIstio":             {nginxBackendRouteManifest, nginxMtlsConfigManifest},
		"TestSimpleTlsWithIstioAndBcp":       {nginxBackendRouteManifest, nginxMtlsConfigManifest, nginxBcpSimpleTlsManifest},
		"TestCustomMtlsWithIstioAndBcp":      {nginxBackendRouteManifest, nginxMtlsConfigManifest, nginxBcpMtlsManifest},
		"TestSimpleTlsWithIstioAndBcpAndBtp": {nginxBackendRouteManifest, nginxMtlsConfigManifest, nginxBtpSimpleTlsManifest},
	}
}

func (s *istioCustomMtlsTestingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, setupNginxMtlsManifest)
	s.NoError(err, "can delete setup manifest")
}

// TestCleartextWithIstio tests that with Istio enabled, a backend that has its auto-mtls disabled annotation
// and references the cleartext port of nginx can be reached.
func (s *istioCustomMtlsTestingSuite) TestCleartextWithIstio() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"), // hostname routed to backend that references the cleartext port of nginx
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK}, time.Minute)
}

// TestSimpleTlsWithIstioAndBcp tests that with Istio enabled, a backend that has its auto-mtls disabled annotation
// and references the simple TLS port of nginx can be reached.
// BackendConfigPolicy is used to validate the TLS certificate.
func (s *istioCustomMtlsTestingSuite) TestSimpleTlsWithIstioAndBcp() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example-simple.com"),
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK}, time.Minute)
}

// TestCustomMtlsWithIstioAndBcp tests that with Istio enabled, a backend that has its auto-mtls disabled annotation
// and references the custom mTLS port of nginx can be reached.
// BackendConfigPolicy is used to validate the TLS certificate.
func (s *istioCustomMtlsTestingSuite) TestCustomMtlsWithIstioAndBcp() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example-mtls.com"),
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK}, time.Minute)
}

// TestSimpleTlsWithIstioAndBcpAndBtp tests that with Istio enabled, a backend that has its auto-mtls disabled annotation
// and references the simple TLS port of nginx can be reached.
// BackendTLSPolicy is used to validate the TLS certificate.
func (s *istioCustomMtlsTestingSuite) TestSimpleTlsWithIstioAndBcpAndBtp() {
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example-simple.com"),
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK}, time.Minute)
}
