package e2e_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	static_plugin_gloo "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gatewaydefaults "github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloohelpers "github.com/solo-io/gloo/test/helpers"

	"github.com/elazarl/goproxy"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/test/services"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

var _ = FDescribe("tunneling", func() {

	var caCert = []byte(`-----BEGIN CERTIFICATE-----
MIIDkzCCAnugAwIBAgIJAKe/ZGdfcHdPMA0GCSqGSIb3DQEBCwUAMGAxCzAJBgNV
BAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBX
aWRnaXRzIFB0eSBMdGQxGTAXBgNVBAMMEGRlbW8gZm9yIGdvcHJveHkwHhcNMTYw
OTI3MTQzNzQ3WhcNMTkwOTI3MTQzNzQ3WjBgMQswCQYDVQQGEwJBVTETMBEGA1UE
CAwKU29tZS1TdGF0ZTEhMB8GA1UECgwYSW50ZXJuZXQgV2lkZ2l0cyBQdHkgTHRk
MRkwFwYDVQQDDBBkZW1vIGZvciBnb3Byb3h5MIIBIjANBgkqhkiG9w0BAQEFAAOC
AQ8AMIIBCgKCAQEA2+W48YZoch72zj0a+ZlyFVY2q2MWmqsEY9f/u53fAeTxvPE6
1/DnqsydnA3FnGvxw9Dz0oZO6xG+PZvp+lhN07NZbuXK1nie8IpxCa342axpu4C0
69lZwxikpGyJO4IL5ywp/qfb5a2DxPTAyQOQ8ROAaydoEmktRp25yicnQ2yeZW//
1SIQxt7gRxQIGmuOQ/Gqr/XN/z2cZdbGJVRUvQXk7N6NhQiCX1zlmp1hzUW9jwC+
JEKKF1XVpQbc94Bo5supxhkKJ70CREPy8TH9mAUcQUZQRohnPvvt/lKneYAGhjHK
vhpajwlbMMSocVXFvY7o/IqIE/+ZUeQTs1SUwQIDAQABo1AwTjAdBgNVHQ4EFgQU
GnlWcIbfsWJW7GId+6xZIK8YlFEwHwYDVR0jBBgwFoAUGnlWcIbfsWJW7GId+6xZ
IK8YlFEwDAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEAoFUjSD15rKlY
xudzyVlr6n0fRNhITkiZMX3JlFOvtHNYif8RfK4TH/oHNBTmle69AgixjMgy8GGd
H90prytGQ5zCs1tKcCFsN5gRSgdAkc2PpRFOK6u8HwOITV5lV7sjucsddXJcOJbQ
4fyVe47V9TTxI+A7lRnUP2HYTR1Bd0R/IgRAH57d1ZHs7omHIuQ+Ea8ph2ppXMnP
DXVOlZ9zfczSnPnQoomqULOU9Fq2ycyi8Y/ROtAHP6O7wCFbYHXhxojdaHSdhkcd
troTflFMD2/4O6MtBKbHxSmEG6H0FBYz5xUZhZq7WUH24V3xYsfge29/lOCd5/Xf
A+j0RJc/lQ==
-----END CERTIFICATE-----`)

	var caKey = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA2+W48YZoch72zj0a+ZlyFVY2q2MWmqsEY9f/u53fAeTxvPE6
1/DnqsydnA3FnGvxw9Dz0oZO6xG+PZvp+lhN07NZbuXK1nie8IpxCa342axpu4C0
69lZwxikpGyJO4IL5ywp/qfb5a2DxPTAyQOQ8ROAaydoEmktRp25yicnQ2yeZW//
1SIQxt7gRxQIGmuOQ/Gqr/XN/z2cZdbGJVRUvQXk7N6NhQiCX1zlmp1hzUW9jwC+
JEKKF1XVpQbc94Bo5supxhkKJ70CREPy8TH9mAUcQUZQRohnPvvt/lKneYAGhjHK
vhpajwlbMMSocVXFvY7o/IqIE/+ZUeQTs1SUwQIDAQABAoIBAHK94ww8W0G5QIWL
Qwkc9XeGvg4eLUxVknva2Ll4fkZJxY4WveKx9OCd1lv4n7WoacYIwUGIDaQBZShW
s/eKnkmqGy+PvpC87gqL4sHvQpuqqJ1LYpxylLEFqduWOuGPUVC2Lc+QnWCycsCS
CgqZzsbMq0S+kkKRGSvw32JJneZCzqLgLNssQNVk+Gm6SI3s4jJsGPesjhnvoPaa
xZK14uFpltaA05GSTDaQeZJFEdnnb3f/eNPc2xMEfi0S2ZlJ6Q92WJEOepAetDlR
cRFi004bNyTb4Bphg8s4+9Cti5is199aFkGCRDWxeqEnc6aMY3Ezu9Qg3uttLVUd
uy830GUCgYEA7qS0X+9UH1R02L3aoANyADVbFt2ZpUwQGauw9WM92pH52xeHAw1S
ohus6FI3OC8xQq2CN525tGLUbFDZnNZ3YQHqFsfgevfnTs1//gbKXomitev0oFKh
VT+WYS4lkgYtPlXzhdGuk32q99T/wIocAguvCUY3PiA7yBz93ReyausCgYEA6+P8
bugMqT8qjoiz1q/YCfxsw9bAGWjlVqme2xmp256AKtxvCf1BPsToAaJU3nFi3vkw
ICLxUWAYoMBODJ3YnbOsIZOavdXZwYHv54JqwqFealC3DG0Du6fZYZdiY8pK+E6m
3fiYzP1WoVK5tU4bH8ibuIQvpcI8j7Gy0cV6/AMCgYBHl7fZNAZro72uLD7DVGVF
9LvP/0kR0uDdoqli5JPw12w6szM40i1hHqZfyBJy042WsFDpeHL2z9Nkb1jpeVm1
C4r7rJkGqwqElJf6UHUzqVzb8N6hnkhyN7JYkyyIQzwdgFGfaslRzBiXYxoa3BQM
9Q5c3OjDxY3JuhDa3DoVYwKBgDNqrWJLSD832oHZAEIicBe1IswJKjQfriWWsV6W
mHSbdtpg0/88aZVR/DQm+xLFakSp0jifBTS0momngRu06Dtvp2xmLQuF6oIIXY97
2ON1owvPbibSOEcWDgb8pWCU/oRjOHIXts6vxctCKeKAFN93raGphm0+Ck9T72NU
BTubAoGBAMEhI/Wy9wAETuXwN84AhmPdQsyCyp37YKt2ZKaqu37x9v2iL8JTbPEz
pdBzkA2Gc0Wdb6ekIzRrTsJQl+c/0m9byFHsRsxXW2HnezfOFX1H4qAmF6KWP0ub
M8aIn6Rab4sNPSrvKGrU6rFpv/6M33eegzldVnV9ku6uPJI1fFTC
-----END RSA PRIVATE KEY-----`)

	var (
		ctx           context.Context
		cancel        context.CancelFunc
		testClients   services.TestClients
		envoyInstance *services.EnvoyInstance
		up            *gloov1.Upstream

		writeNamespace = defaults.GlooSystem
	)

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

		// write a test upstream
		// this is the upstream that will handle requests
		proxy := goproxy.NewProxyHttpServer()
		proxy.Verbose = true
		//err = setCA([]byte(gloohelpers.Certificate()), []byte(gloohelpers.PrivateKey()))
		err = setCA(caCert, caKey)
		Expect(err).ToNot(HaveOccurred())
		proxy.OnRequest().HandleConnect(goproxy.AlwaysMitm)

		listener, err := net.Listen("tcp", ":0")
		if err != nil {
			panic(err)
		}

		addr := listener.Addr().String()
		_, portStr, err := net.SplitHostPort(addr)
		if err != nil {
			panic(err)
		}

		port, err := strconv.Atoi(portStr)
		if err != nil {
			panic(err)
		}

		go func() {
			defer GinkgoRecover()

			server := &http.Server{Addr: addr, Handler: proxy}
			server.Serve(listener)

			//http.ListenAndServe(envoyInstance.LocalAddr(), proxy)
		}()
		//testUs := v1helpers.NewTestHttpUpstreamWithReply(ctx, envoyInstance.LocalAddr(), "HTTP/1.1 200 OK\n\n")
		//up = testUs.Upstream

		secret := &gloov1.Secret{
			Metadata: &core.Metadata{
				Name:      "secret",
				Namespace: "default",
			},
			Kind: &gloov1.Secret_Tls{
				Tls: &gloov1.TlsSecret{
					CertChain:  string(caCert),//gloohelpers.Certificate(),
					PrivateKey: string(caKey),//gloohelpers.PrivateKey(),
				},
			},
		}

		_, err = testClients.SecretClient.Write(secret, clients.WriteOpts{OverwriteExisting: true})
		Expect(err).NotTo(HaveOccurred())

		up = &gloov1.Upstream{
			Metadata: &core.Metadata{
				Name:      "local-1",
				Namespace: "default",
			},
			UpstreamType: &gloov1.Upstream_Static{
				Static: &static_plugin_gloo.UpstreamSpec{
					Hosts: []*static_plugin_gloo.Host{
						{
							Addr:              envoyInstance.LocalAddr(),
							Port:              uint32(port),
							SniAddr:           "",
							HealthCheckConfig: nil,
						},
					},
				},
			},
		}

		up.HttpProxyHostname = "host.com:443" // enable HTTP tunneling
		up.SslConfig =&gloov1.UpstreamSslConfig{
			SslSecrets:           &gloov1.UpstreamSslConfig_SecretRef{
				SecretRef: &core.ResourceRef{Name:"secret", Namespace:"default"},
			},
			Sni:                  "",
			VerifySubjectAltName: nil,
			//Parameters:           &gloov1.SslParameters{
			//	MinimumProtocolVersion:gloov1.SslParameters_TLSv1_0,
			//	MaximumProtocolVersion:gloov1.SslParameters_TLSv1_0,
			//	CipherSuites: []string{
			//		"[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]:",
			//		"[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]:",
			//		"ECDHE-ECDSA-AES256-GCM-SHA384:",
			//		"ECDHE-RSA-AES256-GCM-SHA384:",
			//
			//		"ECDHE-ECDSA-AES128-SHA:",
			//		"ECDHE-RSA-AES128-SHA:",
			//		"AES128-GCM-SHA256:",
			//		"AES128-SHA:",
			//		"ECDHE-ECDSA-AES256-SHA:",
			//		"ECDHE-RSA-AES256-SHA:",
			//		"AES256-GCM-SHA384:",
			//		"AES256-SHA"},
			//},
			AlpnProtocols: []string{"http/1.1"},
		}
		_, err = testClients.UpstreamClient.Write(up, clients.WriteOpts{OverwriteExisting: true})
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if envoyInstance != nil {
			_ = envoyInstance.Clean()
		}
		cancel()
	})

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

	testRequest := func() string {
		By("Make request")
		responseBody := ""
		EventuallyWithOffset(1, func() error {

			var client http.Client

			scheme := "http"

			//cert := gloohelpers.Certificate()
			//rootca := &cert
			//if rootca != nil {
			//	scheme = "https"
			//	caCertPool := x509.NewCertPool()
			//	ok := caCertPool.AppendCertsFromPEM([]byte(*rootca))
			//	if !ok {
			//		return fmt.Errorf("ca cert is not OK")
			//	}
			//
			//	client.Transport = &http.Transport{
			//		TLSClientConfig: &tls.Config{
			//			RootCAs:            caCertPool,
			//			InsecureSkipVerify: true,
			//		},
			//	}
			//}

			req, err := http.NewRequest("GET", fmt.Sprintf("%s://%s:%d/test", scheme, "localhost", defaults.HttpPort), nil)
			if err != nil {
				return err
			}
			res, err := client.Do(req)
			if err != nil {
				return err
			}
			p := new(bytes.Buffer)
			if _, err := io.Copy(p, res.Body); err != nil {
				return err
			}
			defer res.Body.Close()
			responseBody = p.String()
			return nil
		}, "10s", ".1s").Should(BeNil())
		return responseBody
	}

	Context("filter undefined", func() {

		JustBeforeEach(func() {
			// write a virtual service so we have a proxy to our test upstream
			testVs := getTrivialVirtualServiceForUpstream(writeNamespace, up.Metadata.Ref())
			_, err := testClients.VirtualServiceClient.Write(testVs, clients.WriteOpts{})
			Expect(err).NotTo(HaveOccurred())

			checkProxy()
			checkVirtualService(testVs)
		})

		PIt("should return uncompressed json", func() {
			time.Sleep(1 * time.Second) //TODO(kdorosh) remove
			testReq := testRequest()
			Expect(testReq).Should(ContainSubstring("400 The plain HTTP request was sent to HTTPS port"))
		})

		Context("with SSL", func() {
			FIt("should return uncompressed json", func() {
				time.Sleep(1 * time.Second) //TODO(kdorosh) remove
				testReq := testRequest()
				Expect(testReq).Should(ContainSubstring("403 Forbidden")) //TODO(kdorosh) change to real site
			})
		})
	})

})

func setCA(caCert, caKey []byte) error {
	goproxyCa, err := tls.X509KeyPair(caCert, caKey)
	if err != nil {
		return err
	}
	if goproxyCa.Leaf, err = x509.ParseCertificate(goproxyCa.Certificate[0]); err != nil {
		return err
	}
	goproxy.GoproxyCa = goproxyCa
	goproxy.OkConnect = &goproxy.ConnectAction{Action: goproxy.ConnectAccept, TLSConfig: goproxy.TLSConfigFromCA(&goproxyCa)}
	goproxy.MitmConnect = &goproxy.ConnectAction{Action: goproxy.ConnectMitm, TLSConfig: goproxy.TLSConfigFromCA(&goproxyCa)}
	goproxy.HTTPMitmConnect = &goproxy.ConnectAction{Action: goproxy.ConnectHTTPMitm, TLSConfig: goproxy.TLSConfigFromCA(&goproxyCa)}
	goproxy.RejectConnect = &goproxy.ConnectAction{Action: goproxy.ConnectReject, TLSConfig: goproxy.TLSConfigFromCA(&goproxyCa)}
	return nil
}