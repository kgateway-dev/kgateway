package cloudfoundry_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	copilotapi "code.cloudfoundry.org/copilot/api"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	. "github.com/solo-io/gloo/pkg/plugins/cloudfoundry"
)

var _ = Describe("Common", func() {
	const hostname = "solo.io"
	var resp copilotapi.RoutesResponse
	var us v1.Upstream
	BeforeEach(func() {
		resp.Backends = make(map[string]*copilotapi.BackendSet)
		resp.Backends[hostname] = new(copilotapi.BackendSet)
		resp.Backends[hostname].Backends = []*copilotapi.Backend{
			{
				Address: "1.2.3.4",
				Port:    1234,
			},
		}

		us = v1.Upstream{
			Name: "doesnt matter",
			Type: UpstreamTypeCF,
			Spec: EncodeUpstreamSpec(UpstreamSpec{
				Hostname: hostname,
			}),
		}
	})

	It("get upstreams from co pilot response", func() {
		uss, err := GetUpstreamsFromResponse(&resp)
		Expect(err).NotTo(HaveOccurred())
		Expect(uss).To(HaveLen(1))

		spec, err := DecodeUpstreamSpec(uss[0].Spec)
		Expect(err).NotTo(HaveOccurred())
		Expect(spec.Hostname).To(Equal(hostname))

	})

	It("get endpoints from co pilot response", func() {
		endpoints, err := GetEndpointsFromResponse(&resp, &us)
		Expect(err).NotTo(HaveOccurred())
		Expect(endpoints).To(HaveLen(1))

		Expect(endpoints[0].Address).To(Equal("1.2.3.4"))
		Expect(endpoints[0].Port).To(BeEquivalentTo(1234))

	})

})
