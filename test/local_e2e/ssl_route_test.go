package local_e2e

import (
	"net/http"

	"bytes"
	"context"
	"fmt"

	"github.com/solo-io/gloo/pkg/api/types/v1"

	"crypto/tls"
	"io/ioutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	. "github.com/solo-io/gloo/test/helpers"
)

const (
	sslCertificateChainKey = "ca_chain"
	sslPrivateKeyKey       = "private_key"
)

var _ = Describe("SSL Route", func() {
	It("Receive proxied request", func() {
		err := envoyInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		err = glooInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		envoyPort := glooInstance.EnvoySSLPort()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		tu := NewTestHttpUpstream(ctx)
		err = glooInstance.AddUpstream(tu.Upstream)
		Expect(err).NotTo(HaveOccurred())
		sslCertChain, err := ioutil.ReadFile(ServerCert())
		Must(err)
		sslPrivateKey, err := ioutil.ReadFile(ServerKey())
		Must(err)

		secretName := "ssl-secrets"

		glooInstance.AddSecret(secretName, map[string]string{
			sslCertificateChainKey: string(sslCertChain),
			sslPrivateKeyKey:       string(sslPrivateKey),
		})

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
				SecretRef: secretName,
			},
		}

		err = glooInstance.AddvService(v)
		Expect(err).NotTo(HaveOccurred())

		body := []byte("solo.io test")

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
			cli := http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				},
			}
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
