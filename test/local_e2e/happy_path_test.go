package local_e2e

import (
	"net/http"

	"github.com/onsi/ginkgo"

	"bytes"
	"context"
	"fmt"

	"github.com/solo-io/gloo/pkg/api/types/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("HappyPath", func() {

	It("Receive proxied request", func() {
		fmt.Fprintln(ginkgo.GinkgoWriter, "Running Envoy")
		err := envoyInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		fmt.Fprintln(ginkgo.GinkgoWriter, "Running Gloo")
		err = glooInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		envoyPort := glooInstance.EnvoyPort()
		fmt.Fprintln(ginkgo.GinkgoWriter, "Envoy Port: ", envoyPort)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		fmt.Fprintln(ginkgo.GinkgoWriter, "adding upstream")
		tu := NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
		fmt.Fprintln(ginkgo.GinkgoWriter, tu.Upstream)
		err = glooInstance.AddUpstream(tu.Upstream)
		Expect(err).NotTo(HaveOccurred())

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
		}

		fmt.Fprintln(ginkgo.GinkgoWriter, "adding virtual host")
		err = glooInstance.AddVhost(v)
		Expect(err).NotTo(HaveOccurred())

		body := []byte("solo.io test")

		// wait for envoy to start receiving request
		Eventually(func() error {
			// send a request with a body
			var buf bytes.Buffer
			buf.Write(body)
			_, err = http.Post(fmt.Sprintf("http://%s:%d", "localhost", envoyPort), "application/octet-stream", &buf)
			return err
		}, 90, 1).Should(BeNil())

		expectedResponse := &ReceivedRequest{
			Method: "POST",
			Body:   body,
		}
		Eventually(tu.C).Should(Receive(Equal(expectedResponse)))

	})

})
