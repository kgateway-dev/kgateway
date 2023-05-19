package upstream_proxy_protocol

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("upstream proxyprotocol", func() {
	var ts *envoy_config_core_v3.TransportSocket
	BeforeEach(func() {
		ts = &envoy_config_core_v3.TransportSocket{}
	})
	Context("existing transport socket", func() {
		It("can set protocol ", func() {

			protocolV := "V1"
			newTs, err := WrapWithPProtocol(ts, protocolV)
			Expect(err).NotTo(HaveOccurred())
			Expect(newTs.Name).To(Equal("envoy.transport_sockets.upstream_proxy_protocol"))
		})

		It("doesnt always set protocol ", func() {
			protocolV := ""
			newTs, err := WrapWithPProtocol(ts, protocolV)
			Expect(err).NotTo(HaveOccurred())
			Expect(*newTs).To(Equal(envoy_config_core_v3.TransportSocket{}))
		})
		It("rejects bad protocols ", func() {
			protocolV := "not a protocol"
			newTs, err := WrapWithPProtocol(ts, protocolV)
			Expect(err).To(HaveOccurred())
			Expect(newTs).To(Equal(ts))
		})
	})

	Context("when it has no existing scket", func() {
		var empty *envoy_config_core_v3.TransportSocket
		It("can set protocol ", func() {

			protocolV := "V1"
			newTs, err := WrapWithPProtocol(empty, protocolV)
			Expect(err).NotTo(HaveOccurred())
			Expect(newTs.Name).To(Equal("envoy.transport_sockets.upstream_proxy_protocol"))

		})
		It("doesnt always set protocol ", func() {
			protocolV := ""
			newTs, err := WrapWithPProtocol(empty, protocolV)
			Expect(err).NotTo(HaveOccurred())
			Expect(newTs).To(BeNil(), newTs)
		})
	})

})
