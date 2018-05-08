package local_e2e

import (
	"net/http"

	"github.com/onsi/ginkgo"

	"bytes"
	"context"
	"fmt"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	extensions "github.com/solo-io/gloo/pkg/coreplugins/route-extensions"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("GZip Test", func() {

	It("should returned gzipped response when enabled", func() {
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
				Extensions: extensions.EncodeRouteExtensionSpec(extensions.RouteExtensionSpec{
					Gzip: true,
				}),
			}},
		}

		fmt.Fprintln(ginkgo.GinkgoWriter, "adding virtual service")
		err = glooInstance.AddvService(v)
		Expect(err).NotTo(HaveOccurred())

		// wait for envoy to start receiving request
		Eventually(func() error {
			// send a request with a body
			var buf bytes.Buffer
			for i := 0; i < 30; i++ {
				buf.WriteString(fmt.Sprintf("hello, gloo this is message %d\n", i))
			}
			req, err := http.NewRequest("POST", fmt.Sprintf("http://%s:%d", "localhost", envoyPort), &buf)
			if err != nil {
				return err
			}
			req.Header.Set("content-type", "text/plain")
			req.Header.Set("accept-encoding", "gzip")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			Expect(resp.Header.Get("content-encoding")).To(Equal("gzip"))
			return nil
		}, 90, 1).Should(BeNil())
	})

})
