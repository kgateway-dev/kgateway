package e2e_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	gloohelpers "github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/gloo/test/v1helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Proxy Protocol", func() {

	var (
		err           error
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *services.EnvoyInstance

		gateway        *gatewayv1.Gateway
		virtualService *gatewayv1.VirtualService
		testUpstream   *v1helpers.TestUpstream
		secret         *gloov1.Secret

		requestScheme      string
		rootCACert         string
		proxyProtocolBytes []byte
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()
		defaults.HttpsPort = services.NextBindPort()

		// run gloo
		ro := &services.RunOptions{
			NsToWrite: defaults.GlooSystem,
			NsToWatch: []string{
				"default",
				defaults.GlooSystem,
			},
			WhatToRun: services.What{
				DisableFds: true,
				DisableUds: true,
			},
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, ro)

		// run envoy
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())
		err = envoyInstance.RunWithRole(defaults.GlooSystem+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort)
		Expect(err).NotTo(HaveOccurred())

		// prepare default resources
		secret = &gloov1.Secret{
			Metadata: &core.Metadata{
				Name:      "secret",
				Namespace: "default",
			},
			Kind: &gloov1.Secret_Tls{
				Tls: &gloov1.TlsSecret{
					CertChain:  gloohelpers.Certificate(),
					PrivateKey: gloohelpers.PrivateKey(),
				},
			},
		}

		testUpstream = v1helpers.NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())

		// https://www.haproxy.org/download/1.9/doc/proxy-protocol.txt
		proxyProtocolBytes = []byte("PROXY TCP4 1.2.3.4 1.2.3.4 123 123\r\n")
	})

	JustBeforeEach(func() {
		// Write Secret
		_, err = testClients.SecretClient.Write(secret, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		// Write Upstream
		_, err = testClients.UpstreamClient.Write(testUpstream.Upstream, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		// Write VirtualService
		_, err = testClients.VirtualServiceClient.Write(virtualService, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		// Write Gateway
		_, err = testClients.GatewayClient.Write(gateway, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		// Wait for a proxy to be generated
		gloohelpers.EventuallyResourceAccepted(func() (resources.InputResource, error) {
			return testClients.ProxyClient.Read(defaults.GlooSystem, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
		})
	})

	AfterEach(func() {
		envoyInstance.Clean()
		cancel()
	})

	EventuallyGatewayReturnsOk := func(client *http.Client) {
		EventuallyWithOffset(1, func() (int, error) {
			var buf bytes.Buffer
			res, err := client.Post(fmt.Sprintf("%s://%s:%d/1", requestScheme, "localhost", gateway.BindPort), "application/octet-stream", &buf)
			if err != nil {
				return 0, err
			}
			return res.StatusCode, nil

		}, "15s", "1s").Should(Equal(http.StatusOK))
	}

	Context("HttpGateway", func() {

		Context("without TLS", func() {

			BeforeEach(func() {
				requestScheme = "http"
				rootCACert = ""
				// http gateway
				gateway = gatewaydefaults.DefaultGateway(defaults.GlooSystem)
				// vs without sslConfig
				virtualService = getVirtualServiceToUpstream(testUpstream.Upstream.Metadata.Ref(), nil)
			})

			Context("without PROXY protocol", func() {

				BeforeEach(func() {
					gateway.UseProxyProto = &wrappers.BoolValue{Value: false}
				})

				It("works", func() {
					client := getHttpClientWithoutProxyProtocol(rootCACert)
					EventuallyGatewayReturnsOk(client)
				})

			})

			Context("with PROXY protocol", func() {

				BeforeEach(func() {
					gateway.UseProxyProto = &wrappers.BoolValue{Value: true}
				})

				It("works", func() {
					client := getHttpClientWithProxyProtocol(rootCACert, proxyProtocolBytes)
					EventuallyGatewayReturnsOk(client)
				})

			})

		})

		Context("with TLS", func() {

			BeforeEach(func() {
				requestScheme = "https"
				rootCACert = gloohelpers.Certificate()
				// https gateway
				gateway = gatewaydefaults.DefaultSslGateway(defaults.GlooSystem)
				// vs with sslConfig
				sslConfig := &gloov1.SslConfig{
					SslSecrets: &gloov1.SslConfig_SecretRef{
						SecretRef: secret.Metadata.Ref(),
					},
				}
				virtualService = getVirtualServiceToUpstream(testUpstream.Upstream.Metadata.Ref(), sslConfig)
			})

			Context("without PROXY protocol", func() {

				BeforeEach(func() {
					gateway.UseProxyProto = &wrappers.BoolValue{Value: false}
				})

				It("works", func() {
					client := getHttpClientWithoutProxyProtocol(rootCACert)
					EventuallyGatewayReturnsOk(client)
				})

			})

			Context("with PROXY protocol", func() {

				BeforeEach(func() {
					gateway.UseProxyProto = &wrappers.BoolValue{Value: true}
				})

				It("works", func() {
					client := getHttpClientWithProxyProtocol(rootCACert, proxyProtocolBytes)
					EventuallyGatewayReturnsOk(client)
				})

			})

			Context("with PROXY protocol and SNI", func() {

				BeforeEach(func() {
					gateway.UseProxyProto = &wrappers.BoolValue{Value: true}

					sslConfig := &gloov1.SslConfig{
						SslSecrets: &gloov1.SslConfig_SecretRef{
							SecretRef: secret.Metadata.Ref(),
						},
						SniDomains: []string{"gateway-proxy"},
					}
					virtualService = getVirtualServiceToUpstream(testUpstream.Upstream.Metadata.Ref(), sslConfig)
				})

				It("works", func() {
					client := getHttpClientWithProxyProtocol(rootCACert, proxyProtocolBytes)
					EventuallyGatewayReturnsOk(client)
				})

			})

		})

	})

})

func getHttpClientWithoutProxyProtocol(rootCACert string) *http.Client {
	client, err := getHttpClient(rootCACert, nil)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return client
}

func getHttpClientWithProxyProtocol(rootCACert string, proxyProtocolBytes []byte) *http.Client {
	client, err := getHttpClient(rootCACert, proxyProtocolBytes)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return client
}

func getHttpClient(rootCACert string, proxyProtocolBytes []byte) (*http.Client, error) {

	var (
		client          http.Client
		tlsClientConfig *tls.Config
		dialContext     func(ctx context.Context, network, addr string) (net.Conn, error)
	)

	// If the rootCACert is provided, configure the client to use TLS
	if rootCACert != "" {
		caCertPool := x509.NewCertPool()
		ok := caCertPool.AppendCertsFromPEM([]byte(rootCACert))
		if !ok {
			return nil, fmt.Errorf("ca cert is not OK")
		}

		tlsClientConfig = &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         "gateway-proxy",
			RootCAs:            caCertPool,
		}
	}

	// If the proxyProtocolBytes are provided, configure the dialContext to prepend
	//	the bytes at the beginning of the connection
	if len(proxyProtocolBytes) > 0 {
		dialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			var zeroDialer net.Dialer
			c, err := zeroDialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}

			// inject proxy protocol bytes
			// example: []byte("PROXY TCP4 1.2.3.4 1.2.3.5 443 443\r\n")
			_, err = c.Write(proxyProtocolBytes)
			if err != nil {
				_ = c.Close()
				return nil, err
			}

			return c, nil
		}

	}

	client.Transport = &http.Transport{
		TLSClientConfig: tlsClientConfig,
		DialContext:     dialContext,
	}

	return &client, nil

}

func getVirtualServiceToUpstream(upstreamRef *core.ResourceRef, sslConfig *gloov1.SslConfig) *gatewayv1.VirtualService {
	vs := &gatewayv1.VirtualService{
		Metadata: &core.Metadata{
			Name:      "vs",
			Namespace: defaults.GlooSystem,
		},
		VirtualHost: &gatewayv1.VirtualHost{
			Domains: []string{"*"},
			Routes: []*gatewayv1.Route{{
				Action: &gatewayv1.Route_RouteAction{
					RouteAction: &gloov1.RouteAction{
						Destination: &gloov1.RouteAction_Single{
							Single: &gloov1.Destination{
								DestinationType: &gloov1.Destination_Upstream{
									Upstream: upstreamRef,
								},
							},
						},
					},
				},
				Matchers: []*matchers.Matcher{
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/",
						},
					},
				},
			}},
		},
	}
	if sslConfig != nil {
		vs.SslConfig = sslConfig
	}
	return vs
}
