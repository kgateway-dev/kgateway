package testutils

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"time"

	"github.com/onsi/ginkgo/v2"
)

// DefaultHttpClient should be used in tests because it configures a timeout which the http.DefaultClient
// does not have
//
// Please note that when the server response time exceeds the client timeout, you may hit the following error:
//
//	"Client.Timeout exceeded while awaiting headers"
//
// The solution would be to increase the client timeout defined below. We chose 2 seconds as a reasonable
// default which allows tests to pass consistently.
// Since http.Client caches TCP connections, it is advised to create a new client via DefaultClientBuilder().Build()
// each time while testing any feature that operates at L4 to avoid TCP connection caching issues.
var DefaultHttpClient = &http.Client{
	Timeout: time.Second * 2,
}

// HttpClientBuilder simplifies the process of generating an http client in tests
type HttpClientBuilder struct {
	timeout time.Duration

	rootCaCert         string
	serverName         string
	proxyProtocolBytes []byte
}

// DefaultClientBuilder returns an HttpClientBuilder with some default values
// Since http.Client caches TCP connections, it is advised to create a new client each time
// while testing any feature that operates at L4.
func DefaultClientBuilder() *HttpClientBuilder {
	return &HttpClientBuilder{
		timeout:    DefaultHttpClient.Timeout,
		serverName: "gateway-proxy",
	}
}

func (c *HttpClientBuilder) WithTimeout(timeout time.Duration) *HttpClientBuilder {
	c.timeout = timeout
	return c
}

func (c *HttpClientBuilder) WithProxyProtocolBytes(bytes []byte) *HttpClientBuilder {
	c.proxyProtocolBytes = bytes
	return c
}

func (c *HttpClientBuilder) WithTLSRootCa(rootCaCert string) *HttpClientBuilder {
	c.rootCaCert = rootCaCert
	return c
}

func (c *HttpClientBuilder) WithTLSServerName(serverName string) *HttpClientBuilder {
	c.serverName = serverName
	return c
}

func (c *HttpClientBuilder) Clone() *HttpClientBuilder {
	if c == nil {
		return nil
	}
	clone := new(HttpClientBuilder)

	clone.timeout = c.timeout
	clone.rootCaCert = c.rootCaCert
	clone.serverName = c.serverName
	clone.proxyProtocolBytes = nil
	clone.proxyProtocolBytes = append(clone.proxyProtocolBytes, c.proxyProtocolBytes...)
	return clone
}

func (c *HttpClientBuilder) Build() *http.Client {
	var (
		client          http.Client
		tlsClientConfig *tls.Config
		dialContext     func(ctx context.Context, network, addr string) (net.Conn, error)
	)

	if c.timeout.Seconds() == 0 {
		ginkgo.Fail("No timeout set on client")
	}

	// If the rootCACert is provided, configure the client to use TLS
	if c.rootCaCert != "" {
		caCertPool := x509.NewCertPool()
		ok := caCertPool.AppendCertsFromPEM([]byte(c.rootCaCert))
		if !ok {
			ginkgo.Fail("CA Cert is not ok")
		}

		tlsClientConfig = &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         c.serverName,
			RootCAs:            caCertPool,
		}
	}

	// If the proxyProtocolBytes are provided, configure the dialContext to prepend
	// the bytes at the beginning of the connection
	// https://www.haproxy.org/download/1.9/doc/proxy-protocol.txt
	if len(c.proxyProtocolBytes) > 0 {
		dialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			var zeroDialer net.Dialer
			connection, err := zeroDialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}

			// inject proxy protocol bytes
			// example: []byte("PROXY TCP4 1.2.3.4 1.2.3.5 443 443\r\n")
			_, err = connection.Write(c.proxyProtocolBytes)
			if err != nil {
				_ = connection.Close()
				return nil, err
			}

			return connection, nil
		}
	}

	client.Transport = &http.Transport{
		TLSClientConfig: tlsClientConfig,
		DialContext:     dialContext,
	}
	client.Timeout = c.timeout

	return &client
}
