package proxyprotocol

import (
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoy_listener_proxy_protocol "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/proxy_protocol/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/proxy_protocol"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
)

var _ = Describe("Plugin", func() {

	var (
		p      *plugin
		params plugins.Params

		in  *v1.Listener
		out *envoy_config_listener_v3.Listener
	)

	BeforeEach(func() {
		p = NewPlugin()
		p.Init(plugins.InitParams{})

		in = &v1.Listener{}
		out = &envoy_config_listener_v3.Listener{}
	})

	When("UseProxyProto=true is defined on the listener", func() {

		BeforeEach(func() {
			in.UseProxyProto = &wrappers.BoolValue{Value: true}
		})

		It("appends ProxyProtocol listener filter", func() {
			err := p.ProcessListener(params, in, out)
			Expect(err).NotTo(HaveOccurred())

			Expect(out.ListenerFilters).To(HaveLen(1))
			Expect(out.ListenerFilters).To(HaveLen(1))
			Expect(out.ListenerFilters[0].GetName()).To(Equal(wellknown.ProxyProtocol))
		})

	})

	When("UseProxyProto=false is defined on the listener", func() {

		BeforeEach(func() {
			in.UseProxyProto = &wrappers.BoolValue{Value: false}
		})

		It("does not append ProxyProtocol listener filter", func() {
			err := p.ProcessListener(params, in, out)
			Expect(err).NotTo(HaveOccurred())

			Expect(out.ListenerFilters).To(HaveLen(0))
		})

		When("use deprecated and not-deprecated config", func() {

			BeforeEach(func() {
				in.Options = &v1.ListenerOptions{
					ProxyProtocol: &proxy_protocol.ProxyProtocol{
						Rules: []*proxy_protocol.ProxyProtocol_Rule{
							{
								TlvType: 123,
								OnTlvPresent: &proxy_protocol.ProxyProtocol_KeyValuePair{
									MetadataNamespace: "ns",
									Key:               "key",
								},
							},
						},
						AllowRequestsWithoutProxyProtocol: false,
					},
				}
			})

			It("allows override by non-deprecated config", func() {
				err := p.ProcessListener(params, in, out)
				Expect(err).NotTo(HaveOccurred())

				Expect(out.ListenerFilters).To(HaveLen(1))
				Expect(out.ListenerFilters).To(HaveLen(1))
				Expect(out.ListenerFilters[0].GetName()).To(Equal(wellknown.ProxyProtocol))

				var msg envoy_listener_proxy_protocol.ProxyProtocol
				err = translator.ParseTypedConfig(out.ListenerFilters[0], &msg)
				Expect(err).NotTo(HaveOccurred())
				Expect(msg.Rules[0].TlvType).To(Equal(uint32(123)))
				Expect(msg.Rules[0].GetOnTlvPresent().GetKey()).To(Equal("key"))
				Expect(msg.Rules[0].GetOnTlvPresent().GetMetadataNamespace()).To(Equal("ns"))
			})

			It("errors on enterprise only config", func() {
				in.Options.ProxyProtocol.AllowRequestsWithoutProxyProtocol = true
				err := p.ProcessListener(params, in, out)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Could not load configuration for the following Enterprise features"))
			})
		})

	})

	When("UseProxyProto is not defined on the listener", func() {

		BeforeEach(func() {
			in.UseProxyProto = nil
		})

		It("does not append ProxyProtocol listener filter", func() {
			err := p.ProcessListener(params, in, out)
			Expect(err).NotTo(HaveOccurred())

			Expect(out.ListenerFilters).To(HaveLen(0))
		})

	})

})
