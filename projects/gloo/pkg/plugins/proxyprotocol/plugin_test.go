package proxyprotocol

import (
	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
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

		err := p.Init(plugins.InitParams{})
		Expect(err).NotTo(HaveOccurred())

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
