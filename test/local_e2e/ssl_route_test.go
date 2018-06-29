package local_e2e

import (
	"net/http"

	"bytes"
	"context"
	"fmt"

	"github.com/solo-io/gloo/pkg/api/types/v1"

	"crypto/tls"
	"crypto/x509"
	"io/ioutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	. "github.com/solo-io/gloo/test/helpers"
)

const (
	sslCertificateChainKey = "tls.crt"
	sslPrivateKeyKey       = "tls.key"
	sslRootCaKey           = "tls.root"
)

func getSslGoing(requireClientCert bool) (uint32, *TestUpstream, context.CancelFunc) {
	err := envoyInstance.Run()
	Expect(err).NotTo(HaveOccurred())

	err = glooInstance.Run()
	Expect(err).NotTo(HaveOccurred())

	envoyPort := glooInstance.EnvoySSLPort()

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		if r := recover(); r != nil {
			cancel()
			panic(r)
		}
	}()
	tu := NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
	err = glooInstance.AddUpstream(tu.Upstream)
	Expect(err).NotTo(HaveOccurred())
	sslCertChain, err := ioutil.ReadFile(ServerCert())
	Expect(err).NotTo(HaveOccurred())
	sslPrivateKey, err := ioutil.ReadFile(ServerKey())
	Expect(err).NotTo(HaveOccurred())
	secretData := map[string]string{
		sslCertificateChainKey: string(sslCertChain),
		sslPrivateKeyKey:       string(sslPrivateKey),
	}
	if requireClientCert {
		secretData[sslRootCaKey] = string(sslCertChain)
	}

	secretName := "ssl-secrets"

	glooInstance.AddSecret(secretName, secretData)

	v := &v1.VirtualService{
		Name: "default",
		Routes: []*v1.Route{{
			Matcher: &v1.Route_RequestMatcher{
				RequestMatcher: &v1.RequestMatcher{
					Path: &v1.RequestMatcher_PathPrefix{PathPrefix: "/"},
				},
			},
			SingleDestination: &v1.Destination{
				DestinationType: &v1.Destination_Upstream{
					Upstream: &v1.UpstreamDestination{
						Name: tu.Upstream.Name,
					},
				},
			},
		}},
		SslConfig: &v1.SSLConfig{
			SslSecrets: &v1.SSLConfig_SecretRef{SecretRef: secretName},
		},
	}

	err = glooInstance.AddvService(v)
	Expect(err).NotTo(HaveOccurred())
	return envoyPort, tu, cancel
}

func getClient(envoyPort uint32, transport http.RoundTripper) http.Client {
	req, err := http.NewRequest("GET", fmt.Sprintf("https://%s:%d", "localhost", envoyPort), nil)
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-type", "application/octet-stream")

	cli := http.Client{
		Transport: transport,
	}
	return cli

}

var _ = Describe("SSL Route", func() {
	It("Receive proxied ssl request with client cert validation", func() {
		envoyPort, tu, c := getSslGoing(true)
		defer c()

		cert, err := tls.LoadX509KeyPair(ClientCert(), ClientKey())
		Expect(err).NotTo(HaveOccurred())
		roots := x509.NewCertPool()
		sslCertChain, err := ioutil.ReadFile(ServerCert())
		Expect(err).NotTo(HaveOccurred())
		ok := roots.AppendCertsFromPEM(sslCertChain)
		Expect(ok).To(BeTrue())

		transport := &http.Transport{
			TLSClientConfig: &tls.Config{
				// this is for us to authenticate the server
				RootCAs: roots,
				// this is for the server to authenticate us
				Certificates: []tls.Certificate{cert},
				// this must match the server name in the cert
				ServerName: "test-ingress",
			},
		}
		cli := getClient(envoyPort, transport)
		req, err := http.NewRequest("GET", fmt.Sprintf("https://%s:%d", "localhost", envoyPort), nil)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() error {
			// send a request with a body
			res, err := cli.Do(req)
			if err != nil {
				return err
			}
			if res.StatusCode != 200 {
				return errors.Errorf("bad response: %v", res.StatusCode)
			}
			return nil
		}, 30, 1).Should(BeNil())

		expectedResponse := &ReceivedRequest{
			Method: "GET",
		}
		Eventually(tu.C).Should(Receive(Equal(expectedResponse)))

		// now make sure that a request without a client cert fails

		transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				// this is for us to authenticate the server
				RootCAs: roots,
				// this must match the server name in the cert
				ServerName: "test-ingress",
			},
		}

		cli = getClient(envoyPort, transport)

		r, err := cli.Do(req)
		fmt.Fprintf(GinkgoWriter, "response is: %v %v\n", transport, r)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("handshake failure"))
	})

	It("Receive proxied ssl request", func() {
		envoyPort, tu, c := getSslGoing(false)
		defer c()
		body := []byte("solo.io test")
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		cli := getClient(envoyPort, transport)

		// wait for envoy to start receiving request
		Eventually(func() error {
			// send a request with a body
			var buf bytes.Buffer
			buf.Write(body)
			req, err := http.NewRequest("POST", fmt.Sprintf("https://%s:%d", "localhost", envoyPort), &buf)
			if err != nil {
				return err
			}
			req.Header.Set("Content-type", "application/octet-stream")
			res, err := cli.Do(req)
			if err != nil {
				return err
			}
			if res.StatusCode != 200 {
				return errors.Errorf("bad response: %v", res.StatusCode)
			}
			return nil
		}, 90, 1).Should(BeNil())

		expectedResponse := &ReceivedRequest{
			Method: "POST",
			Body:   body,
		}
		Eventually(tu.C).Should(Receive(Equal(expectedResponse)))

	})

})
