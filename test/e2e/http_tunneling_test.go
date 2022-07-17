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
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *services.EnvoyInstance
		up            *gloov1.Upstream
		tuPort        uint32
		// sslPort       uint32

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

		// start http proxy and setup upstream that points to it
		port := startHttpProxy(ctx)

		tu := v1helpers.NewTestHttpUpstreamWithTls(ctx, envoyInstance.LocalAddr())
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
	})

	JustBeforeEach(func() {

		_, err := testClients.UpstreamClient.Write(up, clients.WriteOpts{OverwriteExisting: true})
		Expect(err).NotTo(HaveOccurred())

		// write a virtual service so we have a proxy to our test upstream
		testVs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
		_, err = testClients.VirtualServiceClient.Write(testVs, clients.WriteOpts{})
		Expect(err).NotTo(HaveOccurred())

		checkProxy()
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
			ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s://%s:%d/test", scheme, "localhost", defaults.HttpPort), bytes.NewBuffer(json))
			if err != nil {
				return err
			}
			res, err := client.Do(req)
			if err != nil {
				fmt.Printf("COPILOT error: %v\n", err)
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
			// res.Header.Get("")
			responseBody = p.String()
			return nil
		}, "1s", "0.8s").Should(BeNil())
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

	Context("with SSL", func() {

		BeforeEach(func() {

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

			up.SslConfig = &gloov1.UpstreamSslConfig{
				SslSecrets: &gloov1.UpstreamSslConfig_SecretRef{
					SecretRef: &core.ResourceRef{Name: "secret", Namespace: "default"},
				},
			}
			// sslPort = v1helpers.StartSslProxy(ctx, tuPort)
			up.HttpProxyHostname = &wrappers.StringValue{Value: fmt.Sprintf("%s:%d", envoyInstance.LocalAddr(), tuPort)} // enable HTTP tunneling,
		})

		FIt("should proxy HTTPS", func() {
			// the request path here is envoy -> local HTTP proxy (HTTP CONNECT) -> test TLS upstream
			// and back. TLS origination happens in envoy, the HTTP proxy is sending TLS-encrypted HTTP bytes over
			// TCP to the local SSL proxy, which decrypts and sends to the test upstream (an echo server)
			jsonStr := `{"value":"Hello, world!"}`
			time.Sleep(time.Second * 5)
			testReq := testRequest(jsonStr)
			Expect(testReq).Should(ContainSubstring(jsonStr))
		})
	})

})

func startHttpProxy(ctx context.Context) int {

	// clientCerts, err := tls.X509KeyPair([]byte(gloohelpers.Certificate()), []byte(gloohelpers.PrivateKey()))
	// Expect(err).ToNot(HaveOccurred())

	// caCertPool := x509.NewCertPool()
	// ok := caCertPool.AppendCertsFromPEM([]byte(gloohelpers.Certificate())) // in prod this would not be the client cert
	// if !ok {
	// 	Fail("unable to append ca certs to cert pool")
	// }

	// tlsCfg := &tls.Config{
	// 	Certificates: []tls.Certificate{clientCerts},
	// 	RootCAs:      caCertPool,
	// 	ClientCAs:    caCertPool,
	// 	// MinVersion:   tls.VersionTLS11,
	// 	// MaxVersion:   tls.VersionTLS11,
	// }

	cert := []byte(gloohelpers.Certificate())
	key := []byte(gloohelpers.PrivateKey())
	cer, err := tls.X509KeyPair(cert, key)
	Expect(err).NotTo(HaveOccurred())

	tlsCfg := &tls.Config{
		GetCertificate: func(chi *tls.ClientHelloInfo) (*tls.Certificate, error) {
			// if cb != nil {
			// 	cb(chi)
			// }
			return &cer, nil
		},
	}

	// listener, err := tls.Listen("tcp", ":0", tlsCfg)

	listener, err := net.Listen("tcp", ":0")
	Expect(err).ToNot(HaveOccurred())

	addr := listener.Addr().String()
	_, portStr, err := net.SplitHostPort(addr)
	Expect(err).ToNot(HaveOccurred())

	port, err := strconv.Atoi(portStr)
	Expect(err).ToNot(HaveOccurred())

	fmt.Fprintln(GinkgoWriter, "go proxy addr", addr)

	cstate := func(conn net.Conn, newState http.ConnState) {
		switch newState {
		case http.StateNew:
			fmt.Fprintf(GinkgoWriter, "***** new state %s\n", "KDOROSH ******")
		case http.StateClosed:
			fmt.Fprintf(GinkgoWriter, "***** state closed %s\n", "KDOROSH ******")
		case http.StateHijacked:
			fmt.Fprintf(GinkgoWriter, "***** state hijacked %s\n", "KDOROSH ******")
		case http.StateActive:
			fmt.Fprintf(GinkgoWriter, "***** state active %s\n", "KDOROSH ******")
		case http.StateIdle:
			fmt.Fprintf(GinkgoWriter, "***** state idle %s\n", "KDOROSH ******")
		}
	}

	go func() {
		defer GinkgoRecover()
		// server := &http.Server{Addr: addr, Handler: http.HandlerFunc(connectProxyTls), ConnState: cstate} //, TLSConfig: tlsCfg}
		server := &http.Server{Addr: addr, Handler: http.HandlerFunc(connectProxy), ConnState: cstate} //, TLSConfig: tlsCfg}
		tlsListener := tls.NewListener(listener, tlsCfg)
		// server.Serve(tlsListener)
		fmt.Printf("%v", tlsListener)
		server.Serve(listener)
		<-ctx.Done()
		server.Close()
	}()

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
	fmt.Fprintf(GinkgoWriter, "Accepting CONNECT to %s\n", "KDOROSH")
	if r.Method != "CONNECT" {
		http.Error(w, "not connect", 400)
		return
	}

	hij, ok := w.(http.Hijacker)
	if !ok {
		Fail("no hijacker")
	}
	host := r.URL.Host

	// clientCerts, err := tls.X509KeyPair([]byte(gloohelpers.Certificate()), []byte(gloohelpers.PrivateKey()))
	// Expect(err).NotTo(HaveOccurred())

	// caCertPool := x509.NewCertPool()
	// ok = caCertPool.AppendCertsFromPEM([]byte(gloohelpers.Certificate())) // in prod this would not be the client cert
	// if !ok {
	// 	Fail("unable to append ca certs to cert pool")
	// }
	// targetConn, err := tls.Dial("tcp", host, &tls.Config{
	// 	Certificates: []tls.Certificate{clientCerts},
	// 	RootCAs:      caCertPool,
	// 	ClientCAs:    caCertPool,
	// })
	targetConn, err := net.Dial("tcp", host)
	if err != nil {
		http.Error(w, "can't connect", 500)
		return
	}

	conn, buf, err := hij.Hijack()
	if err != nil {
		Expect(err).ToNot(HaveOccurred())
	}
	defer conn.Close()

	fmt.Fprintf(GinkgoWriter, "Accepting CONNECT to %s\n", host)
	conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))

	// no just copy:
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		io.Copy(buf, targetConn)
		buf.Flush()
		wg.Done()
	}()
	go func() {
		io.Copy(targetConn, buf)
		wg.Done()
	}()

	wg.Wait()
	fmt.Fprintf(GinkgoWriter, "done proxying\n")
}

func connectProxyTls(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(GinkgoWriter, "***** Accepting CONNECT to %s\n", "KDOROSH ******")
	if r.Method != "CONNECT" {
		http.Error(w, "not connect", 400)
		return
	}

	// w.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
	// w.Write([]byte("kdorosh manual\r\n\r\n"))

	if r.TLS != nil {
		fmt.Fprintf(GinkgoWriter, "***** handshake complete %v\n", r.TLS.HandshakeComplete)
		fmt.Fprintf(GinkgoWriter, "***** tls version %v\n", r.TLS.Version)
		fmt.Fprintf(GinkgoWriter, "***** cipher suite %v\n", r.TLS.CipherSuite)
		fmt.Fprintf(GinkgoWriter, "***** negotiated protocol %v\n", r.TLS.NegotiatedProtocol)
	}
	fmt.Fprintf(GinkgoWriter, "***** entire tls %v\n", r.TLS)

	// seems to be empty regardless...
	// var body string
	// if r.Body != nil {
	// 	if data, err := ioutil.ReadAll(r.Body); err == nil {
	// 		body = string(data)
	// 	}
	// 	defer r.Body.Close()
	// }

	// hij, ok := w.(http.Hijacker)
	// if !ok {
	// 	Fail("no hijacker")
	// }
	// conn, buf, err := hij.Hijack()
	// if err != nil {
	// 	Expect(err).ToNot(HaveOccurred())
	// }
	// defer conn.Close()

	// ioutil.ReadAll(buf.Reader)
	// // ioutil.ReadAll(buf.Writer)

	// for !isEof(buf.Reader) {
	// 	fmt.Fprintf(GinkgoWriter, "***** waiting for reader to be done %s\n", "KDOROSH ******")
	// }

	// conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
	// conn.Write([]byte("kdorosh manual2\r\n\r\n"))
	// fmt.Fprintf(GinkgoWriter, "***** Done writing CONNECT to %s\n", "KDOROSH ******")
	// return

	hij, ok := w.(http.Hijacker)
	if !ok {
		Fail("no hijacker")
	}
	host := r.URL.Host

	// clientCerts, err := tls.X509KeyPair([]byte(gloohelpers.Certificate()), []byte(gloohelpers.PrivateKey()))
	// Expect(err).ToNot(HaveOccurred())

	// caCertPool := x509.NewCertPool()
	// ok = caCertPool.AppendCertsFromPEM([]byte(gloohelpers.Certificate())) // in prod this would not be the client cert
	// if !ok {
	// 	Fail("unable to append ca certs to cert pool")
	// }
	// tlsCfg := &tls.Config{
	// 	Certificates: []tls.Certificate{clientCerts},
	// 	RootCAs:      caCertPool,
	// 	ClientCAs:    caCertPool,
	// }

	fmt.Fprintf(GinkgoWriter, "***** pre dial upstream \n")
	targetConn, err := net.Dial("tcp", host)
	// targetConn, err := tls.Dial("tcp", host, tlsCfg)
	if err != nil {
		http.Error(w, "can't connect", 500)
		return
	}
	defer targetConn.Close()
	fmt.Fprintf(GinkgoWriter, "***** post dial upstream \n")

	conn, buf, err := hij.Hijack()
	if err != nil {
		Expect(err).ToNot(HaveOccurred())
	}
	defer conn.Close()
	// tlsConn, ok := conn.(*tls.Conn)
	// if !ok {
	// 	Fail("connection was not a tls connection")
	// }
	// // need to call so peer certs are present
	// err = tlsConn.Handshake()

	fmt.Fprintf(GinkgoWriter, "Accepting CONNECT to %s\n", host)
	// will only work HTTP1.1
	conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))

	// no just copy:
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer GinkgoRecover()
		fmt.Fprintf(GinkgoWriter, "***** start copy from upstream to envoy *****\n")
		buf.Flush()

		for {
			// read bytes from buf.Reader until EOF
			bts := []byte{1}
			_, err := targetConn.Read(bts)
			if errors.Is(err, io.EOF) {
				break
			}
			Expect(err).NotTo(HaveOccurred())
			numWritten, err := buf.Write(bts)
			if err != nil && !errors.Is(err, io.EOF) {
				fmt.Fprintf(GinkgoWriter, "***** copy err %v *****\n", err)
				Fail("no good")
			}
			fmt.Fprintf(GinkgoWriter, "***** partial: copied %v bytes from upstream to envoy *****\n", numWritten)
			buf.Flush()
			Expect(err).NotTo(HaveOccurred())
		}

		// Expect(err).NotTo(HaveOccurred())
		// numBytes, err := io.CopyBuffer(buf, targetConn, []byte{1})
		// Expect(err).NotTo(HaveOccurred())
		// fmt.Fprintf(GinkgoWriter, "***** copied %v bytes from upstream to envoy *****\n", numBytes)
		err = buf.Flush()
		Expect(err).NotTo(HaveOccurred())
		wg.Done()
	}()
	go func() {
		defer GinkgoRecover()
		fmt.Fprintf(GinkgoWriter, "***** start copy from  envoy to upstream *****\n")

		for !isEof(buf.Reader) {
			// read bytes from buf.Reader until EOF
			bts := []byte{1}
			_, err := buf.Read(bts)
			Expect(err).NotTo(HaveOccurred())
			numWritten, err := targetConn.Write(bts)
			if err != nil && !errors.Is(err, io.EOF) {
				fmt.Fprintf(GinkgoWriter, "***** copy err %v *****\n", err)
				// Fail("no good")
			}
			fmt.Fprintf(GinkgoWriter, "***** partial: copied %v bytes from envoy to upstream *****\n", numWritten)
		}
		fmt.Fprintf(GinkgoWriter, "***** done copied bytes from envoy to upstream *****\n")

		// numBytes, err := io.CopyBuffer(targetConn, buf, []byte{1})
		// Expect(err).NotTo(HaveOccurred())
		// fmt.Fprintf(GinkgoWriter, "***** copied %v bytes from envoy to upstream *****\n", numBytes)
		wg.Done()
	}()

	wg.Wait()
	fmt.Fprintf(GinkgoWriter, "***** done proxying *****\n")
}
