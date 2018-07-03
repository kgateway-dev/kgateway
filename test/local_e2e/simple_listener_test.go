package local_e2e

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/solo-io/gloo/pkg/storage/file"

	"github.com/solo-io/gloo/pkg/api/types/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Listener Test", func() {
	It("Proxies HTTP using Listeners and Attributes", func() {
		fmt.Fprintln(ginkgo.GinkgoWriter, "Running Envoy")
		roleName := "listener-test-role"
		err := envoyInstance.RunWithId(roleName + "~1234")
		Expect(err).NotTo(HaveOccurred())

		fmt.Fprintln(ginkgo.GinkgoWriter, "Running Gloo")
		err = glooInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		envoyPort := glooInstance.EnvoyPort()
		fmt.Fprintln(ginkgo.GinkgoWriter, "Envoy Port: ", envoyPort)

		role := &v1.Role{
			Name: roleName,
			Listeners: []*v1.Listener{
				{
					Name:        "listener",
					BindAddress: "127.0.0.1",
					BindPort:    envoyPort,
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
		}

		attr := &v1.Attribute{
			Name: "route-config",
			AttributeType: &v1.Attribute_ListenerAttribute{
				ListenerAttribute: &v1.ListenerAttribute{
					Selector: map[string]string{
						"foo": "bar",
					},
					VirtualServices: []string{"default"},
				},
			},
		}

		store, err := file.NewStorage(glooInstance.ConfigDir(), time.Second)
		Expect(err).NotTo(HaveOccurred())

		_, err = store.V1().Roles().Create(role)
		Expect(err).NotTo(HaveOccurred())
		_, err = store.V1().Attributes().Create(attr)
		Expect(err).NotTo(HaveOccurred())

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
			DisableForGateways: true,
		}

		fmt.Fprintln(ginkgo.GinkgoWriter, "adding virtual service")
		err = glooInstance.AddvService(v)
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
