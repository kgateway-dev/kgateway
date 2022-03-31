package protocoloptions_test

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_extensions_upstreams_http_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/protocoloptions"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Plugin", func() {

	var (
		p      plugins.UpstreamPlugin
		params plugins.Params
		out    *envoy_config_cluster_v3.Cluster
	)

	BeforeEach(func() {
		p = protocoloptions.NewPlugin()
		out = new(envoy_config_cluster_v3.Cluster)

	})
	Context("upstream", func() {
		It("should not use window sizes if UseHttp2 is not true", func() {
			falseVal := &v1.Upstream{
				MaxConcurrentStreams:        &wrappers.UInt32Value{Value: 123},
				InitialConnectionWindowSize: &wrappers.UInt32Value{Value: 7777777},
				UseHttp2:                    &wrappers.BoolValue{Value: false},
			}
			nilVal := &v1.Upstream{
				MaxConcurrentStreams:        &wrappers.UInt32Value{Value: 123},
				InitialConnectionWindowSize: &wrappers.UInt32Value{Value: 7777777},
			}
			var nilOptions *envoy_config_core_v3.Http2ProtocolOptions = nil

			err := p.ProcessUpstream(params, falseVal, out)
			Expect(err).NotTo(HaveOccurred())
			test, err := utils.AnyToMessage(out.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"])
			Expect(err).To(HaveOccurred())
			explicitHttpConfig, ok := test.(*envoy_extensions_upstreams_http_v3.HttpProtocolOptions)
			Expect(ok).To(BeFalse())
			Expect(explicitHttpConfig.GetExplicitHttpConfig().GetHttp2ProtocolOptions()).To(Equal(nilOptions))

			err = p.ProcessUpstream(params, nilVal, out)
			Expect(err).NotTo(HaveOccurred())
			test, err = utils.AnyToMessage(out.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"])
			Expect(err).To(HaveOccurred()) //If Http2 not true, TypedExtensionProtocolOptionsprotobuf is never set so AnyToMessage on should fail
			explicitHttpConfig, ok = test.(*envoy_extensions_upstreams_http_v3.HttpProtocolOptions)
			Expect(ok).To(BeFalse()) //TypedExtensionProtocolOptions is never set so trying to access it directly will fail as well
			Expect(explicitHttpConfig.GetExplicitHttpConfig().GetHttp2ProtocolOptions()).To(Equal(nilOptions))
		})

		It("should not accept connection streams that are too small", func() {
			tooSmall := &v1.Upstream{
				InitialConnectionWindowSize: &wrappers.UInt32Value{Value: 65534},
				UseHttp2:                    &wrappers.BoolValue{Value: true},
			}

			err := p.ProcessUpstream(params, tooSmall, out)
			Expect(err).To(HaveOccurred())
		})

		It("should not accept connection streams that are too large", func() {
			tooBig := &v1.Upstream{
				InitialStreamWindowSize: &wrappers.UInt32Value{Value: 2147483648},
				UseHttp2:                &wrappers.BoolValue{Value: true},
			}
			err := p.ProcessUpstream(params, tooBig, out)
			Expect(err).To(HaveOccurred())
		})

		It("should accept connection streams/max concurrent streams that are within the correct range", func() {
			validUpstream := &v1.Upstream{
				MaxConcurrentStreams:        &wrappers.UInt32Value{Value: 1234},
				InitialStreamWindowSize:     &wrappers.UInt32Value{Value: 268435457},
				InitialConnectionWindowSize: &wrappers.UInt32Value{Value: 65535},
				UseHttp2:                    &wrappers.BoolValue{Value: true},
			}

			err := p.ProcessUpstream(params, validUpstream, out)
			Expect(err).NotTo(HaveOccurred())
			test, err := utils.AnyToMessage(out.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"])
			Expect(err).NotTo(HaveOccurred())
			explicitHttpConfig, ok := test.(*envoy_extensions_upstreams_http_v3.HttpProtocolOptions)
			Expect(ok).To(BeTrue())
			Expect(explicitHttpConfig.GetExplicitHttpConfig().GetHttp2ProtocolOptions()).NotTo(BeNil())
			Expect(explicitHttpConfig.GetExplicitHttpConfig().GetHttp2ProtocolOptions().GetMaxConcurrentStreams()).
				To(Equal(&wrappers.UInt32Value{Value: 1234}))
			Expect(explicitHttpConfig.GetExplicitHttpConfig().GetHttp2ProtocolOptions().GetInitialStreamWindowSize()).
				To(Equal(&wrappers.UInt32Value{Value: 268435457}))
			Expect(explicitHttpConfig.GetExplicitHttpConfig().GetHttp2ProtocolOptions().GetInitialConnectionWindowSize()).
				To(Equal(&wrappers.UInt32Value{Value: 65535}))
		})
	})
})
