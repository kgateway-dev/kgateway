package istio

import (
	"context"
	"net/http"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/tests/base"
)

var _ e2e.NewSuiteFunc = NewIstioCustomMtlsSuite

var (
	// Setup configuration for the entire suite
	setup = base.TestCase{
		Manifests: []string{setupNginxMtlsManifest, nginxBackendRouteManifest, nginxMtlsConfigManifest, defaults.CurlPodManifest},
	}

	// Test cases configuration
	testCases = map[string]*base.TestCase{
		"TestCleartextWithIstio": {
			Manifests: []string{},
		},
		"TestSimpleTlsWithIstioAndBcp": {
			Manifests: []string{nginxBcpSimpleTlsManifest},
		},
		"TestCustomMtlsWithIstioAndBcp": {
			Manifests: []string{nginxBcpMtlsManifest},
		},
		"TestSimpleTlsWithIstioAndBtp": {
			Manifests: []string{nginxBtpSimpleTlsManifest},
		},
	}
)

// istioCustomMtlsTestingSuite is the entire Suite of tests for the "Istio" integration cases where auto-mtls is disabled.
// It uses an nginx service as an external nginx.nginx service referenced by the backend.
// The nginx service is configured to have multiple ports:
// - 80: cleartext
// - 443: simple TLS
// - 543: mTLS
type istioCustomMtlsTestingSuite struct {
	*base.BaseTestingSuite
}

func NewIstioCustomMtlsSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &istioCustomMtlsTestingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// TestCleartextWithIstio tests that with Istio enabled, a backend that has its auto-mtls disabled annotation
// and references the cleartext port of nginx can be reached.
func (s *istioCustomMtlsTestingSuite) TestCleartextWithIstio() {
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
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
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
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
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example-mtls.com"),
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK}, time.Minute)
}

// TestSimpleTlsWithIstioAndBtp tests that with Istio enabled, a backend that has its auto-mtls disabled annotation
// and references the simple TLS port of nginx can be reached.
// BackendTLSPolicy is used to validate the TLS certificate.
func (s *istioCustomMtlsTestingSuite) TestSimpleTlsWithIstioAndBtp() {
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		curlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example-simple.com"),
		},
		&testmatchers.HttpResponse{StatusCode: http.StatusOK}, time.Minute)
}
