package local_e2e

import (
	"os"
	"net/http"

	"github.com/onsi/ginkgo"

	"bytes"
	"context"
	"fmt"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/knative"
	"github.com/solo-io/gloo/pkg/protoutil"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

)

var _ = Describe("Knative routing works", func() {

	It("Receives a request with the correct headers", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		fakeIngress := NewTestHttpUpstream(ctx, envoyInstance.LocalAddr())
		os.Setenv("FAKE_KNATIVE_INGRESS", fakeIngress.Address)

		fmt.Fprintln(ginkgo.GinkgoWriter, "Running Envoy")
		err := envoyInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		fmt.Fprintln(ginkgo.GinkgoWriter, "Running Gloo")
		err = glooInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		envoyPort := glooInstance.EnvoyPort()
		fmt.Fprintln(ginkgo.GinkgoWriter, "Envoy Port: ", envoyPort)


		hostname:="test.exmaple.com"
		serviceSpec := knative.UpstreamSpec{
			ServiceName : "doesnt matter" ,
			ServiceNamespace : "doesnt matter" ,
			Hostname:    hostname,
			}

		v1Spec, err := protoutil.MarshalStruct(serviceSpec)
		Expect(err).NotTo(HaveOccurred())
		u := &v1.Upstream{
			Name:      "knative", 
			Type:      knative.UpstreamTypeKnative,
			Spec:      v1Spec,
		}
		fmt.Fprintln(ginkgo.GinkgoWriter, "adding upstream")
		fmt.Fprintln(ginkgo.GinkgoWriter, u)
		err = glooInstance.AddUpstream(u)
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
							Name: u.Name,
						},
					},
				},
			}},
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
		}, 30, 1).Should(BeNil())

		Eventually(fakeIngress.C).Should(Receive(PointTo(MatchFields(IgnoreExtras, Fields{
			"Host": Equal(hostname),
		}))))
	})

})
