package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/solo-io/gloo/test/services/envoy"

	"github.com/solo-io/gloo/test/testutils"

	testmatchers "github.com/solo-io/gloo/test/gomega/matchers"

	"github.com/solo-io/gloo/test/v1helpers"

	"github.com/golang/protobuf/ptypes/wrappers"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	static_plugin_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	testhelpers "github.com/solo-io/gloo/test/helpers"

	pprotocol "github.com/pires/go-proxyproto"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = FDescribe("tunneling", func() {

	var (
		ctx               context.Context
		cancel            context.CancelFunc
		testClients       services.TestClients
		envoyInstance     *envoy.Instance
		up                *gloov1.Upstream
		tuPort            uint32
		vs                *gatewayv1.VirtualService
		tlsRequired       v1helpers.UpstreamTlsRequired = v1helpers.NO_TLS
		tlsHttpConnect    bool
		proxyProtocolUsed bool
	)

	checkProxy := func() {
		// ensure the proxy is created
		Eventually(func() (*gloov1.Proxy, error) {
			return testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
		}, "5s", "0.1s").ShouldNot(BeNil())
	}

	checkVirtualService := func(testVs *gatewayv1.VirtualService) {
		Eventually(func() (*gatewayv1.VirtualService, error) {
			var err error
			vs, err = testClients.VirtualServiceClient.Read(testVs.Metadata.GetNamespace(), testVs.Metadata.GetName(), clients.ReadOpts{})
			return vs, err
		}, "5s", "0.1s").ShouldNot(BeNil())
	}

	BeforeEach(func() {
		testutils.ValidateRequirementsAndNotifyGinkgo(
			testutils.LinuxOnly("Relies on using an in-memory pipe to ourselves"),
		)

		tlsRequired = v1helpers.NO_TLS
		tlsHttpConnect = false
		proxyProtocolUsed = false

		var err error
		ctx, cancel = context.WithCancel(context.Background())

		envoyInstance = envoyFactory.NewInstance()

		// run gloo
		ro := &services.RunOptions{
			NsToWrite: writeNamespace,
			NsToWatch: []string{"default", writeNamespace},
			WhatToRun: services.What{
				DisableFds: true,
				DisableUds: true,
			},
		}
		testClients = services.RunGlooGatewayUdsFds(ctx, ro)

		// write gateways and wait for them to be created
		err = testhelpers.WriteDefaultGateways(writeNamespace, testClients.GatewayClient)
		Expect(err).NotTo(HaveOccurred(), "Should be able to write default gateways")
		Eventually(func() (gatewayv1.GatewayList, error) {
			return testClients.GatewayClient.List(writeNamespace, clients.ListOpts{})
		}, "10s", "0.1s").Should(HaveLen(2), "Gateways should be present")

		// run envoy
		err = envoyInstance.RunWithRoleAndRestXds(writeNamespace+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())
	})

	JustBeforeEach(func() {
		// start http proxy and setup upstream that points to it
		port := startHttpProxy(ctx, tlsHttpConnect, proxyProtocolUsed)

		tu := v1helpers.NewTestHttpUpstreamWithTls(ctx, envoyInstance.LocalAddr(), tlsRequired)
		tuPort = tu.Upstream.UpstreamType.(*gloov1.Upstream_Static).Static.Hosts[0].Port

		up = &gloov1.Upstream{
			Metadata: &core.Metadata{
				Name:      "local-1",
				Namespace: "default",
			},
			UpstreamType: &gloov1.Upstream_Static{
				Static: &static_plugin_gloo.UpstreamSpec{
					Hosts: []*static_plugin_gloo.Host{
						{
							Addr: envoyInstance.LocalAddr(),
							Port: uint32(port),
						},
					},
				},
			},
			HttpProxyHostname: &wrappers.StringValue{Value: fmt.Sprintf("%s:%d", envoyInstance.LocalAddr(), tuPort)}, // enable HTTP tunneling,
		}

		// write a virtual service so we have a proxy to our test upstream
		vs = getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
		vs, err := testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
		Expect(err).NotTo(HaveOccurred())
		checkVirtualService(vs)
	})

	AfterEach(func() {
		envoyInstance.Clean()
		cancel()
	})

	expectResponseBodyOnRequest := func(requestJsonBody string, expectedResponseStatusCode int, expectedResponseBody interface{}) {
		EventuallyWithOffset(1, func(g Gomega) {
			var json = []byte(requestJsonBody)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://%s:%d/test", "localhost", envoyInstance.HttpPort), bytes.NewBuffer(json))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(http.DefaultClient.Do(req)).Should(testmatchers.HaveHttpResponse(&testmatchers.HttpResponse{
				StatusCode: expectedResponseStatusCode,
				Body:       expectedResponseBody,
			}))
		}, "10s", "0.5s").Should(Succeed())
	}

	Context("plaintext", func() {

		JustBeforeEach(func() {
			_, err := testClients.UpstreamClient.Write(up, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
			Expect(err).NotTo(HaveOccurred())

			checkProxy()
		})

		It("should proxy http", func() {
			// the request path here is envoy -> local HTTP proxy (HTTP CONNECT) -> test upstream
			// and back. The HTTP proxy is sending unencrypted HTTP bytes over
			// TCP to the test upstream (an echo server)
			jsonStr := `{"value":"Hello, world!"}`
			expectResponseBodyOnRequest(jsonStr, http.StatusOK, ContainSubstring(jsonStr))
		})
	})

	Context("with TLS", func() {

		JustBeforeEach(func() {

			secret := &gloov1.Secret{
				Metadata: &core.Metadata{
					Name:      "secret",
					Namespace: "default",
				},
				Kind: &gloov1.Secret_Tls{
					Tls: &gloov1.TlsSecret{
						CertChain:  testhelpers.Certificate(),
						PrivateKey: testhelpers.PrivateKey(),
						RootCa:     testhelpers.Certificate(),
					},
				},
			}

			// set mTLS certs to be used by Envoy so we can talk to mTLS test server
			if tlsRequired == v1helpers.MTLS {
				secret.GetTls().CertChain = testhelpers.MtlsCertificate()
				secret.GetTls().PrivateKey = testhelpers.MtlsPrivateKey()
				secret.GetTls().RootCa = testhelpers.MtlsCertificate()
			}

			_, err := testClients.SecretClient.Write(secret, clients.WriteOpts{OverwriteExisting: true})
			Expect(err).NotTo(HaveOccurred())

			sslCfg := &ssl.UpstreamSslConfig{
				SslSecrets: &ssl.UpstreamSslConfig_SecretRef{
					SecretRef: &core.ResourceRef{Name: "secret", Namespace: "default"},
				},
			}

			if tlsRequired > v1helpers.NO_TLS {
				up.SslConfig = sslCfg
			}
			up.HttpProxyHostname = &wrappers.StringValue{Value: fmt.Sprintf("%s:%d", envoyInstance.LocalAddr(), tuPort)} // enable HTTP tunneling,
			if tlsHttpConnect {
				up.HttpConnectSslConfig = sslCfg
			}
			_, err = testClients.UpstreamClient.Write(up, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
			Expect(err).NotTo(HaveOccurred())

			checkProxy()
		})

		Context("with front TLS", func() {

			BeforeEach(func() {
				tlsHttpConnect = true
			})

			It("should proxy plaintext bytes over encrypted HTTP Connect", func() {
				// the request path here is [envoy] -- encrypted --> [local HTTP Connect proxy] -- plaintext --> TLS upstream
				jsonStr := `{"value":"Hello, world!"}`
				expectResponseBodyOnRequest(jsonStr, http.StatusOK, ContainSubstring(jsonStr))
			})
		})

		Context("with back TLS", func() {

			BeforeEach(func() {
				tlsRequired = v1helpers.TLS
			})

			It("should proxy encrypted bytes over plaintext HTTP Connect", func() {
				// the request path here is [envoy] -- plaintext --> [local HTTP Connect proxy] -- encrypted --> TLS upstream
				jsonStr := `{"value":"Hello, world!"}`
				expectResponseBodyOnRequest(jsonStr, http.StatusOK, ContainSubstring(jsonStr))
			})

			Context("with multiple routes to one upstream", func() {
				JustBeforeEach(func() {
					vs.GetVirtualHost().Routes = append(vs.GetVirtualHost().Routes, &gatewayv1.Route{
						Matchers: []*matchers.Matcher{
							{
								PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/1"},
							},
						},
						Action: &gatewayv1.Route_RouteAction{
							RouteAction: &gloov1.RouteAction{
								Destination: &gloov1.RouteAction_Single{
									Single: &gloov1.Destination{
										DestinationType: &gloov1.Destination_Upstream{
											Upstream: up.Metadata.Ref(),
										},
									},
								},
							},
						},
					})
					err := testClients.VirtualServiceClient.Delete(vs.GetMetadata().Namespace, vs.GetMetadata().Name, clients.DeleteOpts{})
					vs, err = testClients.VirtualServiceClient.Write(vs, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
					Expect(err).NotTo(HaveOccurred())
					checkVirtualService(vs)
				})
				It("should allow multiple routes to TLS upstream", func() {
					// the request path here is [envoy] -- plaintext --> [local HTTP Connect proxy] -- encrypted --> TLS upstream
					jsonStr := `{"value":"Hello, world!"}`
					expectResponseBodyOnRequest(jsonStr, http.StatusOK, ContainSubstring(jsonStr))
				})
			})
		})

		Context("with back mTLS", func() {
			BeforeEach(func() {
				tlsRequired = v1helpers.MTLS
			})

			It("should proxy encrypted bytes over plaintext HTTP Connect", func() {
				// the request path here is [envoy] -- plaintext --> [local HTTP Connect proxy] -- encrypted --> mTLS upstream
				jsonStr := `{"value":"Hello, world!"}`
				expectResponseBodyOnRequest(jsonStr, http.StatusOK, ContainSubstring(jsonStr))
			})
		})

		Context("with front and back TLS", func() {

			BeforeEach(func() {
				tlsRequired = v1helpers.TLS
			})

			It("should proxy encrypted bytes over encrypted HTTP Connect", func() {
				// the request path here is [envoy] -- encrypted --> [local HTTP Connect proxy] -- encrypted --> TLS upstream
				jsonStr := `{"value":"Hello, world!"}`
				expectResponseBodyOnRequest(jsonStr, http.StatusOK, ContainSubstring(jsonStr))
			})
		})
	})

	Context("with Proxy Authorization", func() {
		var (
			proxyAuthorizationUsername string
			proxyAuthorizationPassword string
		)
		JustBeforeEach(func() {
			up.HttpConnectHeaders = []*gloov1.HeaderValue{
				{
					Key:   "Proxy-Authorization",
					Value: "Basic " + base64.StdEncoding.EncodeToString([]byte(proxyAuthorizationUsername+":"+proxyAuthorizationPassword)),
				},
			}

			_, err := testClients.UpstreamClient.Write(up, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
			Expect(err).NotTo(HaveOccurred())

			checkProxy()
		})

		When("using invalid credentials", func() {
			BeforeEach(func() {
				proxyAuthorizationUsername = "somebody"
				proxyAuthorizationPassword = "wrong"
			})

			It("should not proxy", func() {
				jsonStr := `{"value":"Hello, world!"}`
				expectResponseBodyOnRequest(jsonStr, http.StatusServiceUnavailable, "upstream connect error or disconnect/reset before headers. reset reason: connection termination")
			})
		})

		When("using valid credentials", func() {
			BeforeEach(func() {
				proxyAuthorizationUsername = "test"
				proxyAuthorizationPassword = "secret"
			})

			It("should proxy", func() {
				jsonStr := `{"value":"Hello, world!"}`
				expectResponseBodyOnRequest(jsonStr, http.StatusOK, ContainSubstring(jsonStr))
			})
		})
	})
})

func startHttpProxy(ctx context.Context, useTLS, consumeProxyProtocol bool) int {
	listener, err := net.Listen("tcp", ":0")
	Expect(err).ToNot(HaveOccurred())

	addr := listener.Addr().String()
	_, portStr, err := net.SplitHostPort(addr)
	Expect(err).ToNot(HaveOccurred())

	port, err := strconv.Atoi(portStr)
	Expect(err).ToNot(HaveOccurred())

	fmt.Fprintln(GinkgoWriter, "go proxy addr", addr)

	go func(useTLS, consumeProxyProtocol bool) {
		defer GinkgoRecover()
		server := &http.Server{Addr: addr, Handler: http.HandlerFunc(connectProxy)}
		toListen := listener
		if useTLS {
			cert := []byte(testhelpers.Certificate())
			key := []byte(testhelpers.PrivateKey())
			cer, err := tls.X509KeyPair(cert, key)
			Expect(err).NotTo(HaveOccurred())

			tlsCfg := &tls.Config{
				GetCertificate: func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
					return &cer, nil
				},
			}
			tlsListener := tls.NewListener(listener, tlsCfg)
			toListen = tlsListener
		}

		// consume proxy protocol
		if consumeProxyProtocol {
			proxyPWrappedListener := &pprotocol.Listener{
				Listener:          toListen,
				ReadHeaderTimeout: 10 * time.Second,
			}
			toListen = proxyPWrappedListener
		}

		server.Serve(toListen)

		<-ctx.Done()
		server.Close()
	}(useTLS, consumeProxyProtocol)

	return port
}

func isEof(r *bufio.Reader) bool {
	_, err := r.Peek(1)
	if err == io.EOF {
		return true
	}
	return false
}

func connectProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != "CONNECT" {
		http.Error(w, "not connect", 400)
		return
	}

	if r.TLS != nil {
		fmt.Fprintf(GinkgoWriter, "handshake complete %v\n", r.TLS.HandshakeComplete)
		fmt.Fprintf(GinkgoWriter, "tls version %v\n", r.TLS.Version)
		fmt.Fprintf(GinkgoWriter, "cipher suite %v\n", r.TLS.CipherSuite)
		fmt.Fprintf(GinkgoWriter, "negotiated protocol %v\n", r.TLS.NegotiatedProtocol)
	}

	if proxyAuth := r.Header.Get("Proxy-Authorization"); proxyAuth != "" {
		fmt.Fprintf(GinkgoWriter, "proxy authorization: %s\n", proxyAuth)
		if username, password := parseBasicAuth(proxyAuth); username != "test" || password != "secret" {
			w.WriteHeader(http.StatusProxyAuthRequired)
			return
		}
	}

	hij, ok := w.(http.Hijacker)
	if !ok {
		Fail("no hijacker")
	}
	host := r.URL.Host

	targetConn, err := net.Dial("tcp", host)
	if err != nil {
		http.Error(w, "can't connect", 500)
		return
	}
	defer targetConn.Close()

	conn, buf, err := hij.Hijack()
	if err != nil {
		Expect(err).ToNot(HaveOccurred())
	}
	defer conn.Close()

	fmt.Fprintf(GinkgoWriter, "Accepting CONNECT to %s\n", host)
	// note to devs! will only work with HTTP 1.1 request from envoy!
	conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))

	// now just copy:
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer GinkgoRecover()
		for {
			// read bytes from buf.Reader until EOF
			bts := []byte{1}
			_, err := targetConn.Read(bts)
			if errors.Is(err, io.EOF) {
				break
			}
			Expect(err).NotTo(HaveOccurred())
			_, err = conn.Write(bts)
			if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, syscall.EPIPE) {
				fmt.Fprintf(GinkgoWriter, "error writing from upstream to envoy %v\n", err)
				Fail("error writing from upstream to envoy")
			}
		}
		err = buf.Flush()
		Expect(err).NotTo(HaveOccurred())
		wg.Done()
	}()
	go func() {
		defer GinkgoRecover()
		for !isEof(buf.Reader) {
			// read bytes from buf.Reader until EOF
			bts := []byte{1}
			_, err := buf.Read(bts)
			Expect(err).NotTo(HaveOccurred())
			_, err = targetConn.Write(bts)
			if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, syscall.EPIPE) {
				fmt.Fprintf(GinkgoWriter, "error writing from envoy to upstream %v\n", err)
				Fail("error writing from envoy to upstream")
			}
		}
		wg.Done()
	}()

	wg.Wait()
	fmt.Fprintf(GinkgoWriter, "done proxying\n")
}

func parseBasicAuth(auth string) (string, string) {
	const basicPrefix = "Basic "
	if !strings.HasPrefix(auth, basicPrefix) {
		return "", ""
	}
	decodedAuth, err := base64.StdEncoding.DecodeString(auth[len(basicPrefix):])
	if err != nil {
		return "", ""
	}
	decodedAuthString := string(decodedAuth)
	username, password, ok := strings.Cut(decodedAuthString, ":")
	if !ok {
		return "", ""
	}
	return username, password
}
