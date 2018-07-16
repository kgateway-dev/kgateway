package nats_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"fmt"
	"time"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/function-discovery"
	. "github.com/solo-io/gloo/pkg/function-discovery/nats-streaming"
	"github.com/solo-io/gloo/pkg/plugins/nats"
)

var _ = Describe("DiscoverNats", func() {
	Describe("happy path", func() {
		Context("upstream for a nats-streaming server", func() {
			It("returns service info for nats-streaming", func() {
				err = natsStreamingInstance.Run()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(2 * time.Second)
				detector := NewNatsDetector(natsStreamingInstance.ClusterId())
				svcInfo, annotations, err := detector.DetectFunctionalService(&v1.Upstream{Name: "Test"}, fmt.Sprintf("localhost:%v", natsStreamingInstance.NatsPort()))
				Expect(err).To(BeNil())
				Expect(annotations).To(HaveKey(functiondiscovery.DiscoveryTypeAnnotationKey))
				Expect(svcInfo).To(Equal(&v1.ServiceInfo{
					Type: nats.ServiceTypeNatsStreaming,
					Properties: nats.EncodeServiceProperties(nats.ServiceProperties{
						ClusterId: natsStreamingInstance.ClusterId(),
					}),
				}))
			})
		})
	})
})
