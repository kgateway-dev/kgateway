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
	// manifests for verify-certificate-hash tests (TestVerifyCertificateHash)
	gatewayManifest   = filepath.Join(fsutils.MustGetThisDir(), "testdata", "gw.yaml")
	tlsSecretManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "tls-secret.yaml")
	clientCertsSecret = filepath.Join(fsutils.MustGetThisDir(), "testdata", "certs", "ca1", "client-certs-8443-9443-secret.yaml")
	curlPodWithCerts  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "curl-pod-with-certs.yaml")

	// client certificate paths inside the curl pod (for verify-certificate-hash tests)
	clientCertPath8443   = "/etc/client-certs/client-8443.crt"
	clientKeyPath8443    = "/etc/client-certs/client-8443.key"
	clientCertPath9443   = "/etc/client-certs/client-9443.crt"
	clientKeyPath9443    = "/etc/client-certs/client-9443.key"
	commonClientCertPath = "/etc/client-certs-frontend/tls.crt"
	commonClientKeyPath  = "/etc/client-certs-frontend/tls.key"

	// client certificate paths for verify-subject-alt-names tests
	matchingSanCertPath    = "/etc/client-matching-san/tls.crt"
	matchingSanKeyPath     = "/etc/client-matching-san/tls.key"
	nonMatchingSanCertPath = "/etc/client-non-matching-san/tls.crt"
	nonMatchingSanKeyPath  = "/etc/client-non-matching-san/tls.key"

	// manifests for FrontendTLSConfig tests (TestFrontendTLSConfig)
	// Note: gatewayManifest and curlPodWithCerts are shared with verify-certificate-hash tests
	caCertConfigMapManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "certs", "ca1", "ca-cert-configmap.yaml")
	clientCertSecretManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "certs", "ca1", "client-cert-secret.yaml")

	// manifests for multiple CA certificates test (TestMultipleCACertificates)
	caCert2ConfigMapManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "certs", "ca2", "ca-cert-2-configmap.yaml")
	clientCert2SecretManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "certs", "ca2", "client-cert-2-secret.yaml")

	// manifests for verify-subject-alt-names tests (TestVerifySubjectAltNames)
	caAltNamesConfigMapManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "certs", "ca-alt-names", "ca-alt-names-configmap.yaml")
	clientMatchingSanSecret     = filepath.Join(fsutils.MustGetThisDir(), "testdata", "certs", "ca-alt-names", "client-matching-san-secret.yaml")
	clientNonMatchingSanSecret  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "certs", "ca-alt-names", "client-non-matching-san-secret.yaml")

	// objects
	proxyObjectMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
)

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	setup := base.TestCase{
		Manifests: []string{
			curlPodWithCerts,
			testdefaults.HttpbinManifest,
			clientCertsSecret,
			clientCertSecretManifest,    // Include client-cert secret so pod can start
			caCertConfigMapManifest,     // Required for FrontendTLSConfig per-port configs
			caCert2ConfigMapManifest,    // Required for multiple CA certificates test
			clientCert2SecretManifest,   // Required for multiple CA certificates test
			caAltNamesConfigMapManifest, // Required for verify-subject-alt-names test
			clientMatchingSanSecret,     // Required for verify-subject-alt-names test
			clientNonMatchingSanSecret,  // Required for verify-subject-alt-names test
			tlsSecretManifest,
			gatewayManifest,
		},
	}

	testCases := map[string]*base.TestCase{
		"TestALPNProtocol":           {},
		"TestCipherSuites":           {},
		"TestECDHCurves":             {},
		"TestMinTLSVersion":          {},
		"TestMaxTLSVersion":          {},
		"TestVerifyCertificateHash":  {},
		"TestFrontendTLSConfig":      {}, // All required resources are already in setup
		"TestMultipleCACertificates": {}, // All required resources are already in setup
		"TestVerifySubjectAltNames":  {}, // All required resources are already in setup
	}
	return &testingSuite{
		base.NewBaseTestingSuite(ctx, testInst, setup, testCases, base.WithMinGwApiVersion(base.GwApiRequireFrontendTLSConfig)),
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

// commonCurlOptsForMTLS returns the common curl options for the mTLS listener (port 8443)
func commonCurlOptsForMTLS(hostname string, port int) []curl.Option {
	return []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
		curl.WithPort(port),
		curl.WithScheme("https"),
		curl.IgnoreServerCert(),
		curl.WithHeader("Host", hostname),
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

func (s *testingSuite) TestVerifyCertificateHash() {
	s.Run("valid client cert succeeds on first mTLS listener", func() {
		// Client certificate with hash matching the first listener's verify-certificate-hash should succeed
		s.assertEventualCurlResponseForMTLS(
			"mtls.example.com",
			8443,
			curl.WithClientCert(clientCertPath8443, clientKeyPath8443),
		)
	})

	s.Run("invalid client cert fails on first mTLS listener", func() {
		// Client certificate with hash NOT matching the first listener's verify-certificate-hash should fail
		s.assertEventualCurlErrorForMTLS(
			"mtls.example.com",
			8443,
			curl.WithClientCert(clientCertPath9443, clientKeyPath9443),
		)
	})

	s.Run("no client cert fails on first mTLS listener", func() {
		// No client certificate should fail when gateway requires verify-certificate-hash
		s.assertEventualCurlErrorForMTLS("mtls.example.com", 8443)
	})

	s.Run("invalid client cert succeeds on second mTLS listener", func() {
		// The "invalid" cert should work on the second listener (configured with its hash)
		s.assertEventualCurlResponseForMTLS(
			"mtls-alt.example.com",
			9443,
			curl.WithClientCert(clientCertPath9443, clientKeyPath9443),
		)
	})

	s.Run("valid client cert fails on second mTLS listener", func() {
		// The "valid" cert should fail on the second listener (different hash)
		s.assertEventualCurlErrorForMTLS(
			"mtls-alt.example.com",
			9443,
			curl.WithClientCert(clientCertPath8443, clientKeyPath8443),
		)
	})

	s.Run("no client cert fails on second mTLS listener", func() {
		// No client certificate should fail on the second mTLS listener too
		s.assertEventualCurlErrorForMTLS("mtls-alt.example.com", 9443)
	})

	s.Run("regular listener works without client cert", func() {
		// Original listener (port 443) should still work without client certificate
		// This validates that only the mTLS listeners require client certs
		s.assertEventualCurlResponse()
	})
}

// assertEventualCurlResponse is a helper that wraps AssertEventualCurlResponse with common test settings
func (s *testingSuite) assertEventualCurlResponse(opts ...curl.Option) {
	curlOpts := append(commonCurlOpts(), opts...)
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		curlOpts,
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gstruct.Ignore(),
		},
		10*time.Second,
	)
}

// assertEventualCurlError is a helper that wraps AssertEventualCurlError with common test settings
func (s *testingSuite) assertEventualCurlError(opts ...curl.Option) {
	curlOpts := append(commonCurlOpts(), opts...)
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlError(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		curlOpts,
		35, // CURLE_HTTP2_STREAM_ERROR
		10*time.Second,
	)
}

// assertEventualCurlResponseForMTLS is a helper for the mTLS listener (port 8443)
func (s *testingSuite) assertEventualCurlResponseForMTLS(hostname string, port int, opts ...curl.Option) {
	curlOpts := append(commonCurlOptsForMTLS(hostname, port), opts...)
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		curlOpts,
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gstruct.Ignore(),
		},
		10*time.Second,
	)
}

// assertEventualCurlErrorForMTLS is a helper for the mTLS listener (port 8443)
func (s *testingSuite) assertEventualCurlErrorForMTLS(hostname string, port int, opts ...curl.Option) {
	curlOpts := append(commonCurlOptsForMTLS(hostname, port), opts...)
	s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlError(
		s.Ctx,
		testdefaults.CurlPodExecOpt,
		curlOpts,
		16, // CURLE_SSL_CACERT_BADFILE
		10*time.Second,
	)
}

func (s *testingSuite) TestFrontendTLSConfig() {
	s.Run("AllowValidOnly requires client cert", func() {
		// Should fail without client cert on port 8445 (per-port config with AllowValidOnly)
		// Use error code 16 (CURLE_SSL_CACERT_BADFILE) which is what we get when client cert is required
		curlOpts := append(commonCurlOpts(), curl.WithPort(8445))
		s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlError(
			s.Ctx,
			testdefaults.CurlPodExecOpt,
			curlOpts,
			16, // CURLE_SSL_CACERT_BADFILE
			10*time.Second,
		)
	})

	s.Run("AllowValidOnly with valid client cert", func() {
		// Should succeed with client cert on port 8445
		s.assertEventualCurlResponse(
			curl.WithPort(8445),
			curl.WithClientCert(commonClientCertPath, commonClientKeyPath),
		)
	})

	s.Run("AllowInsecureFallback without client cert", func() {
		// Should succeed without client cert on port 8444 (per-port config with AllowInsecureFallback)
		s.assertEventualCurlResponse(
			curl.WithPort(8444),
			// No client cert provided
		)
	})

	s.Run("AllowInsecureFallback with client cert", func() {
		// Should succeed with client cert on port 8444
		s.assertEventualCurlResponse(
			curl.WithPort(8444),
			curl.WithClientCert(commonClientCertPath, commonClientKeyPath),
		)
	})
}

func (s *testingSuite) TestMultipleCACertificates() {
	// Port 8446 uses wildcard domain *.example.com with multiple CA cert refs
	// This tests the scenario from issue #12938: multiple rootCA certs for the same wildcard domains
	wildcardHostname := "test.example.com" // Matches *.example.com wildcard

	s.Run("client cert signed by first CA succeeds on wildcard domain", func() {
		// Port 8446 has multiple CA cert refs (ca-cert and ca-cert-2) for wildcard domain *.example.com
		// Client cert signed by ca-cert should be accepted
		curlOpts := append(commonCurlOptsForMTLS(wildcardHostname, 8446),
			curl.WithClientCert(commonClientCertPath, commonClientKeyPath))
		s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
			s.Ctx,
			testdefaults.CurlPodExecOpt,
			curlOpts,
			&testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Body:       gstruct.Ignore(),
			},
			10*time.Second,
		)
	})

	s.Run("client cert signed by second CA succeeds on wildcard domain", func() {
		// Port 8446 has multiple CA cert refs (ca-cert and ca-cert-2) for wildcard domain *.example.com
		// Client cert signed by ca-cert-2 should be accepted
		curlOpts := append(commonCurlOptsForMTLS(wildcardHostname, 8446),
			curl.WithClientCert(commonClientCertPath, commonClientKeyPath))
		s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
			s.Ctx,
			testdefaults.CurlPodExecOpt,
			curlOpts,
			&testmatchers.HttpResponse{
				StatusCode: http.StatusOK,
				Body:       gstruct.Ignore(),
			},
			10*time.Second,
		)
	})

	s.Run("no client cert fails on wildcard domain", func() {
		// Port 8446 requires client cert (AllowValidOnly mode) for wildcard domain *.example.com
		// Connection without client cert should fail
		curlOpts := commonCurlOptsForMTLS(wildcardHostname, 8446)
		s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlError(
			s.Ctx,
			testdefaults.CurlPodExecOpt,
			curlOpts,
			16, // CURLE_SSL_CACERT_BADFILE
			10*time.Second,
		)
	})
}

func (s *testingSuite) TestVerifySubjectAltNames() {
	// Custom curl options for port 8447
	curlOpts8447 := []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
		curl.WithPort(8447),
		curl.WithScheme("https"),
		curl.IgnoreServerCert(),
		curl.WithHeader("Host", "example.com"),
		curl.VerboseOutput(),
	}

	s.Run("verify-subject-alt-names with matching SAN should work", func() {
		// Port 8447 requires "mtls.example.com" in client cert SAN
		// client-matching-san.crt has "DNS:mtls.example.com" SAN - should succeed
		s.assertEventualCurlResponse(
			append(curlOpts8447, curl.WithClientCert(matchingSanCertPath, matchingSanKeyPath))...,
		)
	})

	s.Run("verify-subject-alt-names with non-matching SAN should fail", func() {
		// Port 8447 requires "mtls.example.com" in client cert SAN
		// client-non-matching-san.crt has "DNS:mtls-alt.example.com" SAN - should fail
		s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlError(
			s.Ctx,
			testdefaults.CurlPodExecOpt,
			append(curlOpts8447, curl.WithClientCert(nonMatchingSanCertPath, nonMatchingSanKeyPath)),
			16, // CURLE_SSL_CACERT_BADFILE - client cert rejected due to SAN mismatch
			10*time.Second,
		)
	})
}
