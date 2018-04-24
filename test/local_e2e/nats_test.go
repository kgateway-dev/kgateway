package local_e2e

import (
	"net/http"

	"bytes"
	"errors"
	"fmt"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/coreplugins/service"
	"github.com/solo-io/gloo/pkg/plugins/nats-streaming"

	"github.com/nats-io/go-nats-streaming"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Nats streaming test", func() {

	It("Receive proxied request", func() {
		err := envoyInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		err = glooInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		err = natsStreamingInstance.Run()
		Expect(err).NotTo(HaveOccurred())

		envoyPort := glooInstance.EnvoyPort()

		serviceSpec := service.UpstreamSpec{
			Hosts: []service.Host{{
				Addr: envoyInstance.LocalAddr(),
				Port: natsStreamingInstance.NatsPort(),
			}},
		}

		u := &v1.Upstream{
			Name: "local", // TODO: randomize
			Type: "service",
			Spec: service.EncodeUpstreamSpec(serviceSpec),
			ServiceInfo: &v1.ServiceInfo{
				Type: natsstreaming.ServiceTypeNatsStreaming,
			},
		}
		err = glooInstance.AddUpstream(u)
		Expect(err).NotTo(HaveOccurred())

		v := &v1.VirtualService{
			Name: "default",
			Routes: []*v1.Route{{
				Matcher: &v1.Route_EventMatcher{
					EventMatcher: &v1.EventMatcher{
						EventType: "some-event",
					},
				},
				SingleDestination: &v1.Destination{
					DestinationType: &v1.Destination_Function{
						Function: &v1.FunctionDestination{
							UpstreamName: u.Name,
							FunctionName: "nats-subject",
						},
					},
				},
			}, {
				Matcher: &v1.Route_RequestMatcher{
					RequestMatcher: &v1.RequestMatcher{
						Path: &v1.RequestMatcher_PathPrefix{
							PathPrefix: "/nats",
						},
					},
				},
				SingleDestination: &v1.Destination{
					DestinationType: &v1.Destination_Function{
						Function: &v1.FunctionDestination{
							UpstreamName: u.Name,
							FunctionName: "nats-subject",
						},
					},
				},
			}},
		}

		err = glooInstance.AddVhost(v)
		Expect(err).NotTo(HaveOccurred())

		body := []byte("solo.io test")

		// TODO subscribe for nats streaming
		sc, err := stan.Connect(natsStreamingInstance.ClusterId(), "clientID", stan.NatsURL(fmt.Sprintf("nats://localhost:%d", natsStreamingInstance.NatsPort())))
		Expect(err).NotTo(HaveOccurred())
		defer sc.Close()
		recvied := make(chan struct{}, 10)
		sub, err := sc.Subscribe("nats-subject", func(m *stan.Msg) {
			fmt.Printf("Received a message: %s\n", string(m.Data))
			recvied <- struct{}{}
		})
		Expect(err).NotTo(HaveOccurred())
		defer sub.Unsubscribe()

		// wait for envoy to start receiving request
		Eventually(func() error {
			// send a request with a body
			// replace with go-sdk-send-event
			var buf bytes.Buffer
			buf.Write(body)
			resp, err := http.Post(fmt.Sprintf("http://%s:%d/nats", "localhost", envoyPort), "application/octet-stream", &buf)
			if err != nil {
				return err
			}
			if resp.StatusCode != 200 {
				return errors.New("invalid code")
			}
			return nil
		}, "1m", "1s").Should(BeNil())

		// expecct that a response was received.
		Eventually(recvied).Should(Receive())

	})

})
