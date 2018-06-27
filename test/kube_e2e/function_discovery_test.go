package kube_e2e

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/grpc"
	"github.com/solo-io/gloo/pkg/plugins/nats-streaming"
	"github.com/solo-io/gloo/pkg/plugins/rest"
)

var _ = Describe("Function Discovery Service Detection", func() {
	for _, test := range []struct {
		description         string
		upstreamName        string
		expectedServiceInfo *v1.ServiceInfo
	}{
		{
			description:         "grpc",
			upstreamName:        namespace + "-grpc-test-service-8080",
			expectedServiceInfo: &v1.ServiceInfo{Type: grpc.ServiceTypeGRPC},
		},
		{
			description:         "nats",
			upstreamName:        namespace + "-nats-streaming-4222",
			expectedServiceInfo: &v1.ServiceInfo{Type: natsstreaming.ServiceTypeNatsStreaming},
		},
		{
			description:         "swagger",
			upstreamName:        namespace + "-petstore-9090",
			expectedServiceInfo: &v1.ServiceInfo{Type: rest.ServiceTypeREST},
		},
	} {
		Context("discovery for "+test.description+" upstreams", func() {
			It("should detect the upstream service info", func() {
				Eventually(func() (*v1.ServiceInfo, error) {
					list, err := gloo.V1().Upstreams().List()
					if err != nil {
						return nil, err
					}
					var upstreamToTest *v1.Upstream
					for _, us := range list {
						if us.Name == test.upstreamName {
							upstreamToTest = us
							break
						}
					}

					if upstreamToTest == nil {
						return nil, errors.New("could not find upstream " + test.upstreamName)
					}
					return upstreamToTest.ServiceInfo, nil
				}, "2m", "5s").Should(Equal(test.expectedServiceInfo))
			})
		})
	}
})
