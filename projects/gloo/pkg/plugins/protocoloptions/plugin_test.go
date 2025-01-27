package protocoloptions_test

import (
	"time"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_extensions_upstreams_http_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/protocoloptions"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/protocol"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"
)

var _ = Describe("Plugin", func() {

	var (
		p                           plugins.UpstreamPlugin
		params                      plugins.Params
		out                         *envoy_config_cluster_v3.Cluster
		expectedProtocolErrorString string
	)

	BeforeEach(func() {
		p = protocoloptions.NewPlugin()
		out = new(envoy_config_cluster_v3.Cluster)
		expectedProtocolErrorString = "Both HTTP1 and HTTP2 options may only be configured with non-default 'Upstream_USE_DOWNSTREAM_PROTOCOL' specified for Protocol Selection"

	})
	Context("upstream", func() {
		Context("USE_DOWNSTREAM_PROTOCOL is set", func() {
			It("should account for `nil` Http1ProtocolOptions", func() {
				upstream := createTestUpstreamWithProtocolOptions(true, nil, v1.Upstream_USE_DOWNSTREAM_PROTOCOL)
				upstream.ConnectionConfig = nil

				err := p.ProcessUpstream(params, upstream, out)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should unpack Http1ProtocolOptions to envoy object", func() {
				upstream := createTestUpstreamWithProtocolOptions(true, nil, v1.Upstream_USE_DOWNSTREAM_PROTOCOL)
				upstream.ConnectionConfig.Http1ProtocolOptions = &protocol.Http1ProtocolOptions{
					EnableTrailers: true,
				}

				err := p.ProcessUpstream(params, upstream, out)
				Expect(err).ToNot(HaveOccurred())

				test, err := utils.AnyToMessage(out.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"])
				Expect(err).ToNot(HaveOccurred())
				protocolOptions, ok := test.(*envoy_extensions_upstreams_http_v3.HttpProtocolOptions)
				Expect(ok).To(BeTrue())

				Expect(protocolOptions.GetUseDownstreamProtocolConfig().GetHttpProtocolOptions().GetEnableTrailers()).To(BeTrue())
			})

			It("should unpack Http2ProtocolOptions to envoy object", func() {
				upstream := createTestUpstreamWithProtocolOptions(true, nil, v1.Upstream_USE_DOWNSTREAM_PROTOCOL)

				err := p.ProcessUpstream(params, upstream, out)
				Expect(err).ToNot(HaveOccurred())

				test, err := utils.AnyToMessage(out.GetTypedExtensionProtocolOptions()["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"])
				Expect(err).ToNot(HaveOccurred())
				protocolOptions, ok := test.(*envoy_extensions_upstreams_http_v3.HttpProtocolOptions)
				Expect(ok).To(BeTrue())

				Expect(protocolOptions.GetUseDownstreamProtocolConfig().GetHttp2ProtocolOptions().GetMaxConcurrentStreams()).To(Equal(&wrappers.UInt32Value{Value: 1234}))
			})
		})

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

		It("should accept valid values for all http2 connection settings", func() {
			validUpstream := &v1.Upstream{
				UseHttp2:                                &wrappers.BoolValue{Value: true},
				MaxConcurrentStreams:                    &wrappers.UInt32Value{Value: 1234},
				InitialStreamWindowSize:                 &wrappers.UInt32Value{Value: 268435457},
				InitialConnectionWindowSize:             &wrappers.UInt32Value{Value: 65535},
				OverrideStreamErrorOnInvalidHttpMessage: &wrappers.BoolValue{Value: true},
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
			Expect(explicitHttpConfig.GetExplicitHttpConfig().GetHttp2ProtocolOptions().GetOverrideStreamErrorOnInvalidHttpMessage()).
				To(Equal(&wrappers.BoolValue{Value: true}))
		})

		// the configuration should only be rejected if all of the conditions are met:
		//    1. useHttp2 == true
		//    2. Http1ProtocolOptions != nil
		//    3. protocolSelection == CONFIGURED
		// otherwise, all configurations should be accepted
		It("Should only deny configuration if (1) useHttp2 is true AND (2) Http1ProtocolOptions != nil AND (3) ProtocolSelection == CONFIGURED", func() {
			var http1Opt *protocol.Http1ProtocolOptions
			useHttp2 := true
			http1Opt = &protocol.Http1ProtocolOptions{}
			protocolSelection := v1.Upstream_USE_CONFIGURED_PROTOCOL
			upstream := createTestUpstreamWithProtocolOptions(useHttp2, http1Opt, protocolSelection)
			err := p.ProcessUpstream(params, upstream, out)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(BeEquivalentTo(expectedProtocolErrorString))
		})

		It("Should allow configuration if useHttp2 is false", func() {
			var http1Opt *protocol.Http1ProtocolOptions
			useHttp2 := false
			http1Opt = &protocol.Http1ProtocolOptions{}
			protocolSelection := v1.Upstream_USE_CONFIGURED_PROTOCOL
			upstream := createTestUpstreamWithProtocolOptions(useHttp2, http1Opt, protocolSelection)
			err := p.ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should allow configuration if Http1ProtocolOptions is nil", func() {
			var http1Opt *protocol.Http1ProtocolOptions
			useHttp2 := true
			http1Opt = nil
			protocolSelection := v1.Upstream_USE_CONFIGURED_PROTOCOL
			upstream := createTestUpstreamWithProtocolOptions(useHttp2, http1Opt, protocolSelection)
			err := p.ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should allow configuration if Upstream_ClusterProtocolSelection is USE_DOWNSTREAM_PROTOCOL", func() {
			var http1Opt *protocol.Http1ProtocolOptions
			useHttp2 := true
			http1Opt = &protocol.Http1ProtocolOptions{}
			protocolSelection := v1.Upstream_USE_DOWNSTREAM_PROTOCOL
			upstream := createTestUpstreamWithProtocolOptions(useHttp2, http1Opt, protocolSelection)
			err := p.ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

func createTestUpstreamWithProtocolOptions(useHttp2 bool, http1ProtocolOptions *protocol.Http1ProtocolOptions, protocolSelection v1.Upstream_ClusterProtocolSelection) *v1.Upstream {
	upstream := &v1.Upstream{
		MaxConcurrentStreams:        &wrappers.UInt32Value{Value: 1234},
		InitialStreamWindowSize:     &wrappers.UInt32Value{Value: 268435457},
		InitialConnectionWindowSize: &wrappers.UInt32Value{Value: 65535},
		UseHttp2:                    &wrappers.BoolValue{Value: useHttp2},
		ProtocolSelection:           protocolSelection,
	}

	minute := prototime.DurationToProto(time.Minute)
	hour := prototime.DurationToProto(time.Hour)
	upstream.ConnectionConfig = &v1.ConnectionConfig{
		TcpKeepalive: &v1.ConnectionConfig_TcpKeepAlive{
			KeepaliveInterval: minute,
			KeepaliveTime:     hour,
			KeepaliveProbes:   3,
		},
		Http1ProtocolOptions: http1ProtocolOptions,
	}

	return upstream
}
