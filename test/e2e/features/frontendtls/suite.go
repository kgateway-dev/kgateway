//go:build e2e

package frontendtls

import (
	"context"
	"net/http"
	"path/filepath"
	"time"

	"github.com/onsi/gomega/gstruct"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	testdefaults "github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
	// manifests
	gatewayManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gw.yaml")
	tlsSecretManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tls-secret.yaml")

	// objects
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
)

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	setup := base.TestCase{
		Manifests: []string{
			testdefaults.CurlPodManifest,
			testdefaults.HttpbinManifest,
			tlsSecretManifest,
			gatewayManifest,
		},
	}

	testCases := map[string]*base.TestCase{
		"TestALPNProtocol":  {},
		"TestCipherSuites":  {},
		"TestECDHCurves":    {},
		"TestMinTLSVersion": {},
		"TestMaxTLSVersion": {},
	}
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// commonCurlOpts returns the common curl options used across all TLS tests for the default gateway
func commonCurlOpts() []curl.Option {
	return []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
		curl.WithPort(443),
		curl.WithScheme("https"),
		curl.IgnoreServerCert(),
		curl.WithHeader("Host", "example.com"),
		curl.VerboseOutput(),
	}
}

type testingSuite struct {
	*base.BaseTestingSuite
}

func (s *testingSuite) TestALPNProtocol() {
	s.Run("HTTP2 negotiation", func() {
		// HTTP/2 should work with the gateway (configured with h2 ALPN)
		// Server should accept h2 protocol
		s.assertEventualCurlResponse(curl.WithHTTP2())
	})

	// the negative test doesn't behave as expected because Curl will fallback to a supported protocol if the one it specified is not supported by the server
	// s.Run("HTTP1.1 fallback", func() {
	// 	// Should fail with HTTP1.1
	// 	s.assertEventualCurlError(curl.WithHTTP11())
	// })
}

func (s *testingSuite) TestCipherSuites() {
	s.Run("allowed cipher succeeds", func() {
		// Allowed cipher (ECDHE-RSA-AES128-GCM-SHA256) should work with TLS 1.2
		s.assertEventualCurlResponse(
			curl.WithTLSVersion(curl.TLSVersion12),
			curl.WithTLSMaxVersion(curl.TLSVersion12),
			curl.WithCiphers(curl.CipherECDHERSAAES128GCMSHA256),
		)
	})

	s.Run("disallowed cipher fails", func() {
		// Force TLS 1.2 to ensure cipher restrictions apply (TLS 1.3 has different cipher suites)
		// The gateway only allows ECDHE-RSA-AES128-GCM-SHA256
		// Try to force a different cipher (ECDHE-RSA-AES256-GCM-SHA384)
		s.assertEventualCurlError(
			curl.WithTLSVersion(curl.TLSVersion12),
			curl.WithTLSMaxVersion(curl.TLSVersion12),
			curl.WithCiphers(curl.CipherECDHERSAAES256GCMSHA384),
		)
	})
}

func (s *testingSuite) TestECDHCurves() {
	s.Run("X25519 curve succeeds", func() {
		// X25519 curve should work with TLS 1.2
		s.assertEventualCurlResponse(
			curl.WithTLSVersion(curl.TLSVersion12),
			curl.WithTLSMaxVersion(curl.TLSVersion12),
			curl.WithCurves(curl.CurveX25519),
		)
	})

	s.Run("P-256 curve succeeds", func() {
		// P-256 (prime256v1) curve should work with TLS 1.2
		s.assertEventualCurlResponse(
			curl.WithTLSVersion(curl.TLSVersion12),
			curl.WithTLSMaxVersion(curl.TLSVersion12),
			curl.WithCurves(curl.CurvePrime256v1),
		)
	})

	s.Run("disallowed curve fails", func() {
		// Force TLS 1.2 to ensure curve restrictions apply
		// Gateway only allows X25519 and P-256, so secp384r1 should fail
		s.assertEventualCurlError(
			curl.WithTLSVersion(curl.TLSVersion12),
			curl.WithTLSMaxVersion(curl.TLSVersion12),
			curl.WithCurves("secp384r1"),
		)
	})
}

func (s *testingSuite) TestMinTLSVersion() {
	s.Run("TLS 1.2 succeeds", func() {
		// TLS 1.2 should work (gateway min is 1.2)
		s.assertEventualCurlResponse(curl.WithTLSVersion(curl.TLSVersion12))
	})

	s.Run("TLS 1.1 fails", func() {
		// TLS 1.1 should fail (gateway min is 1.2)
		// Force both min and max to TLS 1.1 so curl only attempts TLS 1.1
		s.assertEventualCurlError(
			curl.WithTLSVersion(curl.TLSVersion11),
			curl.WithTLSMaxVersion(curl.TLSVersion11),
		)
	})
}

func (s *testingSuite) TestMaxTLSVersion() {
	s.Run("TLS 1.2 succeeds", func() {
		// TLS 1.2 should work (gateway max is 1.2)
		s.assertEventualCurlResponse(curl.WithTLSVersion(curl.TLSVersion12))
	})

	s.Run("TLS 1.3 fails", func() {
		// TLS 1.3 should fail (gateway max is 1.2)
		// Force both min and max to TLS 1.3 so curl only attempts TLS 1.3
		s.assertEventualCurlError(
			curl.WithTLSVersion(curl.TLSVersion13),
			curl.WithTLSMaxVersion(curl.TLSVersion13),
		)
	})
}

// assertEventualCurlResponse is a helper that wraps AssertEventualCurlResponse with common test settings
func (s *testingSuite) assertEventualCurlResponse(opts ...curl.Option) {
	curlOpts := append(commonCurlOpts(), opts...)
	s.TestInstallation.Assertions.AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		curlOpts,
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gstruct.Ignore(),
		},
		20*time.Second,
	)
}

// assertEventualCurlError is a helper that wraps AssertEventualCurlError with common test settings
func (s *testingSuite) assertEventualCurlError(opts ...curl.Option) {
	curlOpts := append(commonCurlOpts(), opts...)
	s.TestInstallation.Assertions.AssertEventualCurlError(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		curlOpts,
		35, // CURLE_HTTP2_STREAM_ERROR
		10*time.Second,
	)
}
