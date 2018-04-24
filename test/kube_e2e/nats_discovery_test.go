package kube_e2e

import (
	"time"

	"bytes"

	. "github.com/onsi/ginkgo"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	. "github.com/solo-io/gloo/test/helpers"
)

var _ = Describe("Function Discovery for NATS upstream", func() {
	natsUpstreamName := namespace + "-nats-streaming-4222"
	Context("creating a vService with a route to a NATS upstream discovered by "+
		" gloo-function-discovery", func() {
		natsPath := "/nats"
		natsTopic := "my-topic"
		vServiceName := "default"

		BeforeEach(func() {
			_, err := gloo.V1().VirtualServices().Create(&v1.VirtualService{
				Name: vServiceName,
				Routes: []*v1.Route{
					{
						Matcher: &v1.Route_RequestMatcher{
							RequestMatcher: &v1.RequestMatcher{
								Path: &v1.RequestMatcher_PathPrefix{
									PathPrefix: natsPath,
								},
							},
						},
						SingleDestination: &v1.Destination{
							DestinationType: &v1.Destination_Function{
								Function: &v1.FunctionDestination{
									FunctionName: natsTopic,
									UpstreamName: natsUpstreamName,
								},
							},
						},
					},
				},
			})
			Must(err)
		})
		AfterEach(func() {
			gloo.V1().Upstreams().Delete(natsUpstreamName)
			gloo.V1().VirtualServices().Delete(vServiceName)
		})
		It("should route to the nats topic", func() {
			//messageBuffer, done, err := readNatsMessage(namespace, natsTopic)
			//Expect(err).NotTo(HaveOccurred())
			//defer close(done)
			curlEventuallyShouldRespond(curlOpts{
				path: natsPath,
				body: `{"some": "data"}`,
			}, "< HTTP/1.1 200", time.Minute*5)
			//Expect(messageBuffer.String()).To(ContainSubstring(`{"some": "data"}`))
		})
	})
})

func readNatsMessage(namespace, channel string) (*bytes.Buffer, chan struct{}, error) {
	return TestRunnerAsync("stan-sub", "-s",
		"nats://nats-streaming."+namespace+".svc.cluster.local:4222",
		"-c", "test-cluster",
		"-id", "foo", channel)
}
