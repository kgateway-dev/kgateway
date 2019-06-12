package upstreams

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Conversions", func() {

	It("correctly builds service-derived upstream name", func() {
		name := buildFakeUpstreamName("my-service", 8080)
		Expect(name).To(Equal(ServiceUpstreamNamePrefix + "my-service-8080"))
	})

	It("correctly reconstructs a service name", func() {
		svcName, port, err := reconstructServiceName(ServiceUpstreamNamePrefix + "my-service-8080")
		Expect(err).NotTo(HaveOccurred())
		Expect(svcName).To(Equal("my-service"))
		Expect(port).To(BeEquivalentTo(8080))
	})

	It("fails reconstructing a malformed service name", func() {
		_, _, err := reconstructServiceName(ServiceUpstreamNamePrefix + "my-service")
		Expect(err).To(HaveOccurred())
	})

	It("correctly detects service-derived upstreams", func() {
		Expect(isRealUpstream(ServiceUpstreamNamePrefix + "my-service-8080")).To(BeFalse())
		Expect(isRealUpstream("my-" + ServiceUpstreamNamePrefix + "service-8080")).To(BeTrue())
		Expect(isRealUpstream("my-service-8080")).To(BeTrue())
	})

})
