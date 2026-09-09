//go:build e2e

package dfp

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

const connectTimeout = 10 * time.Second

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is a suite of tests for Dynamic Forward Proxy functionality
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	setup := base.TestCase{
		Manifests: []string{gatewayWithRouteManifest},
	}
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, nil),
	}
}

// TestDynamicForwardProxyBackend verifies that requests are dynamically forwarded to the
// upstream resolved from the request's host header
func (s *testingSuite) TestDynamicForwardProxyBackend() {
	testCases := []struct {
		name                         string
		headers                      map[string]string
		hostname                     string
		expectedStatus               int
		expectedUpstreamBodyContents string
	}{
		{
			name: "request forwarded upstream",
			headers: map[string]string{
				"x-header": "header-value",
			},
			// the DFP backend resolves the host header via DNS, so point it at the
			// shared base echo backend which reflects request headers in its response
			hostname:                     "backend.kgateway-base.svc.cluster.local",
			expectedStatus:               http.StatusOK,
			expectedUpstreamBodyContents: "X-Header",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Build curl options
			opts := []curl.Option{
				curl.WithHostHeader(tc.hostname),
				curl.WithPort(80),
			}

			// Add test-specific headers
			for k, v := range tc.headers {
				opts = append(opts, curl.WithHeader(k, v))
			}

			// Test the request
			common.BaseGateway.Send(
				s.T(),
				&testmatchers.HttpResponse{
					StatusCode: tc.expectedStatus,
					Body:       gomega.ContainSubstring(tc.expectedUpstreamBodyContents),
				},
				opts...,
			)
		})
	}
}

// TestDynamicForwardProxyConnectTermination verifies that the gateway terminates CONNECT,
// opens a raw connection to the dynamically resolved destination, and tunnels the client's
// TLS session without adding TLS on the upstream cluster.
func (s *testingSuite) TestDynamicForwardProxyConnectTermination() {
	g := gomega.NewWithT(s.T())
	target := "backend.kgateway-base.svc.cluster.local:443"

	g.Eventually(func(ig gomega.Gomega) {
		conn, err := (&net.Dialer{Timeout: connectTimeout}).DialContext(
			s.Ctx,
			"tcp",
			gatewayAddress(common.BaseGateway.Address, 80),
		)
		ig.Expect(err).NotTo(gomega.HaveOccurred(), "failed to connect to the gateway")
		if err != nil {
			return
		}
		defer conn.Close()
		ig.Expect(conn.SetDeadline(time.Now().Add(connectTimeout))).To(gomega.Succeed())

		_, err = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
		ig.Expect(err).NotTo(gomega.HaveOccurred(), "failed to write CONNECT request")
		if err != nil {
			return
		}

		connectResponse, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodConnect})
		ig.Expect(err).NotTo(gomega.HaveOccurred(), "failed to read CONNECT response")
		if err != nil {
			return
		}
		defer connectResponse.Body.Close()
		ig.Expect(connectResponse.StatusCode).To(gomega.Equal(http.StatusOK))

		tlsConn := tls.Client(conn, &tls.Config{
			ServerName:         "localhost",
			InsecureSkipVerify: true, //nolint:gosec // The shared e2e backend uses a test certificate.
		})
		if err := tlsConn.HandshakeContext(s.Ctx); err != nil {
			ig.Expect(err).NotTo(gomega.HaveOccurred(), "TLS handshake through CONNECT tunnel failed")
			return
		}

		_, err = fmt.Fprintf(tlsConn, "GET / HTTP/1.1\r\nHost: backend.kgateway-base.svc.cluster.local\r\nConnection: close\r\n\r\n")
		ig.Expect(err).NotTo(gomega.HaveOccurred(), "failed to write tunneled HTTPS request")
		if err != nil {
			return
		}

		response, err := http.ReadResponse(bufio.NewReader(tlsConn), &http.Request{Method: http.MethodGet})
		ig.Expect(err).NotTo(gomega.HaveOccurred(), "failed to read tunneled HTTPS response")
		if err != nil {
			return
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		ig.Expect(err).NotTo(gomega.HaveOccurred(), "failed to read tunneled HTTPS response body")
		ig.Expect(response.StatusCode).To(gomega.Equal(http.StatusOK), string(body))
	}).WithTimeout(30 * time.Second).WithPolling(time.Second).Should(gomega.Succeed())
}

func gatewayAddress(address string, defaultPort int) string {
	if _, _, err := net.SplitHostPort(address); err == nil {
		return address
	}
	return net.JoinHostPort(address, strconv.Itoa(defaultPort))
}
