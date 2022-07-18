package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/solo-io/gloo/test/v1helpers"

	"github.com/golang/protobuf/ptypes/wrappers"

	static_plugin_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloohelpers "github.com/solo-io/gloo/test/helpers"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = Describe("tunneling", func() {

	var (
		ctx            context.Context
		cancel         context.CancelFunc
		testClients    services.TestClients
		envoyInstance  *services.EnvoyInstance
		up             *gloov1.Upstream
		tuPort         uint32
		tlsUpstream    bool
		tlsHttpConnect bool
		writeNamespace = defaults.GlooSystem
	)

	checkProxy := func() {
		// ensure the proxy is created
		Eventually(func() (*gloov1.Proxy, error) {
			return testClients.ProxyClient.Read(writeNamespace, gatewaydefaults.GatewayProxyName, clients.ReadOpts{})
		}, "5s", "0.1s").ShouldNot(BeNil())
	}

	checkVirtualService := func(testVs *gatewayv1.VirtualService) {
		Eventually(func() (*gatewayv1.VirtualService, error) {
			return testClients.VirtualServiceClient.Read(testVs.Metadata.GetNamespace(), testVs.Metadata.GetName(), clients.ReadOpts{})
		}, "5s", "0.1s").ShouldNot(BeNil())
	}

	BeforeEach(func() {
		tlsUpstream = false
		tlsHttpConnect = false
		var err error
		ctx, cancel = context.WithCancel(context.Background())
		defaults.HttpPort = services.NextBindPort()

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
		err = gloohelpers.WriteDefaultGateways(writeNamespace, testClients.GatewayClient)
		Expect(err).NotTo(HaveOccurred(), "Should be able to write default gateways")
		Eventually(func() (gatewayv1.GatewayList, error) {
			return testClients.GatewayClient.List(writeNamespace, clients.ListOpts{})
		}, "10s", "0.1s").Should(HaveLen(2), "Gateways should be present")

		// run envoy
		envoyInstance, err = envoyFactory.NewEnvoyInstance()
		Expect(err).NotTo(HaveOccurred())
		err = envoyInstance.RunWithRoleAndRestXds(writeNamespace+"~"+gatewaydefaults.GatewayProxyName, testClients.GlooPort, testClients.RestXdsPort)
		Expect(err).NotTo(HaveOccurred())
	})

	JustBeforeEach(func() {
		// start http proxy and setup upstream that points to it
		port := startHttpProxy(ctx, tlsHttpConnect)

		tu := v1helpers.NewTestHttpUpstreamWithTls(ctx, envoyInstance.LocalAddr(), tlsUpstream)
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
		testVs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
		_, err := testClients.VirtualServiceClient.Write(testVs, clients.WriteOpts{Ctx: ctx, OverwriteExisting: true})
		Expect(err).NotTo(HaveOccurred())
		checkVirtualService(testVs)
	})

	AfterEach(func() {
		envoyInstance.Clean()
		cancel()
	})

	testRequest := func(jsonStr string) string {
		By("Make request")
		responseBody := ""
		EventuallyWithOffset(1, func() error {
			var client http.Client
			scheme := "http"
			var json = []byte(jsonStr)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s://%s:%d/test", scheme, "localhost", defaults.HttpPort), bytes.NewBuffer(json))
			if err != nil {
				return err
			}
			res, err := client.Do(req)
			if err != nil {
				return err
			}
			if res.StatusCode != http.StatusOK {
				return fmt.Errorf("not ok")
			}
			p := new(bytes.Buffer)
			if _, err := io.Copy(p, res.Body); err != nil {
				return err
			}
			defer res.Body.Close()
			responseBody = p.String()
			return nil
		}, "10s", "0.5s").Should(BeNil())
		return responseBody
	}

	It("should proxy http", func() {
		// the request path here is envoy -> local HTTP proxy (HTTP CONNECT) -> test upstream
		// and back. The HTTP proxy is sending unencrypted HTTP bytes over
		// TCP to the test upstream (an echo server)
		jsonStr := `{"value":"Hello, world!"}`
		testReq := testRequest(jsonStr)
		Expect(testReq).Should(ContainSubstring(jsonStr))
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
						CertChain:  gloohelpers.Certificate(),
						PrivateKey: gloohelpers.PrivateKey(),
						RootCa:     gloohelpers.Certificate(),
					},
				},
			}

			_, err := testClients.SecretClient.Write(secret, clients.WriteOpts{OverwriteExisting: true})
			Expect(err).NotTo(HaveOccurred())

			sslCfg := &gloov1.UpstreamSslConfig{
				SslSecrets: &gloov1.UpstreamSslConfig_SecretRef{
					SecretRef: &core.ResourceRef{Name: "secret", Namespace: "default"},
				},
			}

			if tlsUpstream {
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
				testReq := testRequest(jsonStr)
				Expect(testReq).Should(ContainSubstring(jsonStr))
			})
		})

		Context("with back TLS", func() {

			BeforeEach(func() {
				tlsUpstream = true
			})

			It("should proxy encrypted bytes over plaintext HTTP Connect", func() {
				// the request path here is [envoy] -- plaintext --> [local HTTP Connect proxy] -- encrypted --> TLS upstream
				jsonStr := `{"value":"Hello, world!"}`
				testReq := testRequest(jsonStr)
				Expect(testReq).Should(ContainSubstring(jsonStr))
			})
		})

		Context("with front and back TLS", func() {

			BeforeEach(func() {
				tlsHttpConnect = true
				tlsUpstream = true
			})

			It("should proxy encrypted bytes over encrypted HTTP Connect", func() {
				// the request path here is [envoy] -- encrypted --> [local HTTP Connect proxy] -- encrypted --> TLS upstream
				jsonStr := `{"value":"Hello, world!"}`
				testReq := testRequest(jsonStr)
				Expect(testReq).Should(ContainSubstring(jsonStr))
			})
		})
	})

})

func startHttpProxy(ctx context.Context, useTLS bool) int {
	listener, err := net.Listen("tcp", ":0")
	Expect(err).ToNot(HaveOccurred())

	addr := listener.Addr().String()
	_, portStr, err := net.SplitHostPort(addr)
	Expect(err).ToNot(HaveOccurred())

	port, err := strconv.Atoi(portStr)
	Expect(err).ToNot(HaveOccurred())

	go func(useTLS bool) {
		defer GinkgoRecover()
		server := &http.Server{Addr: addr, Handler: http.HandlerFunc(connectProxy)}
		if useTLS {
			cert := []byte(gloohelpers.Certificate())
			key := []byte(gloohelpers.PrivateKey())
			cer, err := tls.X509KeyPair(cert, key)
			Expect(err).NotTo(HaveOccurred())

			tlsCfg := &tls.Config{
				GetCertificate: func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
					return &cer, nil
				},
			}
			tlsListener := tls.NewListener(listener, tlsCfg)
			server.Serve(tlsListener)
		} else {
			server.Serve(listener)
		}
		<-ctx.Done()
		server.Close()
	}(useTLS)

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
