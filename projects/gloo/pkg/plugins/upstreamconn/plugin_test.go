package upstreamconn_test

import (
	"time"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/solo-io/solo-kit/pkg/utils/prototime"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/protocol"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/upstreamconn"
)

var _ = Describe("Plugin", func() {

	var (
		params   plugins.Params
		plugin   plugins.UpstreamPlugin
		upstream *v1.Upstream
		out      *envoy_config_cluster_v3.Cluster
	)
	BeforeEach(func() {
		out = new(envoy_config_cluster_v3.Cluster)

		params = plugins.Params{}
		upstream = &v1.Upstream{}
		plugin = NewPlugin()
	})

	It("should set max requests when provided", func() {
		upstream.ConnectionConfig = &v1.ConnectionConfig{
			MaxRequestsPerConnection: 5,
		}

		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.GetMaxRequestsPerConnection().Value).To(BeEquivalentTo(5))
	})

	It("should set connection timeout", func() {
		second := prototime.DurationToProto(time.Second)
		upstream.ConnectionConfig = &v1.ConnectionConfig{
			ConnectTimeout: second,
		}

		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.GetConnectTimeout()).To(Equal(second))
	})

	It("should set TcpKeepalive", func() {
		minute := prototime.DurationToProto(time.Minute)
		hour := prototime.DurationToProto(time.Hour)
		upstream.ConnectionConfig = &v1.ConnectionConfig{
			TcpKeepalive: &v1.ConnectionConfig_TcpKeepAlive{
				KeepaliveInterval: minute,
				KeepaliveTime:     hour,
				KeepaliveProbes:   3,
			},
		}

		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		outKeepAlive := out.GetUpstreamConnectionOptions().GetTcpKeepalive()
		expectedValue := envoy_config_core_v3.TcpKeepalive{
			KeepaliveInterval: &wrappers.UInt32Value{
				Value: 60,
			},
			KeepaliveTime: &wrappers.UInt32Value{
				Value: 3600,
			},
			KeepaliveProbes: &wrappers.UInt32Value{
				Value: 3,
			},
		}

		Expect(*outKeepAlive).To(Equal(expectedValue))
	})

	It("should set per connection buffer bytes when provided", func() {
		upstream.ConnectionConfig = &v1.ConnectionConfig{
			PerConnectionBufferLimitBytes: &wrappers.UInt32Value{
				Value: uint32(4096),
			},
		}

		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.GetPerConnectionBufferLimitBytes().Value).To(BeEquivalentTo(uint32(4096)))
	})

	It("should set CommonHttpProtocolOptions", func() {
		hour := prototime.DurationToProto(time.Hour)
		upstream.ConnectionConfig = &v1.ConnectionConfig{
			CommonHttpProtocolOptions: &protocol.HttpProtocolOptions{
				MaxHeadersCount:              3,
				MaxStreamDuration:            hour,
				HeadersWithUnderscoresAction: 1,
			},
		}

		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		outChpo := out.GetCommonHttpProtocolOptions()
		expectedValue := envoy_config_core_v3.HttpProtocolOptions{
			MaxHeadersCount:              &wrappers.UInt32Value{Value: 3},
			MaxStreamDuration:            &duration.Duration{Seconds: 60 * 60},
			HeadersWithUnderscoresAction: envoy_config_core_v3.HttpProtocolOptions_REJECT_REQUEST,
		}

		Expect(*outChpo).To(Equal(expectedValue))
	})

	It("Should set Http1ProtocolOptions", func() {
		upstream.ConnectionConfig = &v1.ConnectionConfig{
			Http1ProtocolOptions: &protocol.Http1ProtocolOptions{
				EnableTrailers:                          true,
				OverrideStreamErrorOnInvalidHttpMessage: &wrappers.BoolValue{Value: true},
			},
		}

		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		outHpo := out.GetHttpProtocolOptions()
		expectedValue := envoy_config_core_v3.Http1ProtocolOptions{
			EnableTrailers:                          true,
			OverrideStreamErrorOnInvalidHttpMessage: &wrappers.BoolValue{Value: true},
		}

		Expect(*outHpo).To(Equal(expectedValue))
	})

	It("Should set preserve_case_header_key_format", func() {
		upstream.ConnectionConfig = &v1.ConnectionConfig{
			Http1ProtocolOptions: &protocol.Http1ProtocolOptions{
				HeaderFormat: &protocol.Http1ProtocolOptions_PreserveCaseHeaderKeyFormat{
					PreserveCaseHeaderKeyFormat: true,
				},
			},
		}

		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())

		Expect(out.GetHttpProtocolOptions().GetHeaderKeyFormat().GetStatefulFormatter()).ToNot(BeNil()) // expect preserve_case_words to be set
		Expect(out.GetHttpProtocolOptions().GetHeaderKeyFormat().GetProperCaseWords()).To(BeNil())      // ...which makes proper_case_words nil
	})

	It("Should set proper_case_header_key_format", func() {
		upstream.ConnectionConfig = &v1.ConnectionConfig{
			Http1ProtocolOptions: &protocol.Http1ProtocolOptions{
				HeaderFormat: &protocol.Http1ProtocolOptions_ProperCaseHeaderKeyFormat{
					ProperCaseHeaderKeyFormat: true,
				},
			},
		}

		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())

		Expect(out.GetHttpProtocolOptions().GetHeaderKeyFormat().GetProperCaseWords()).ToNot(BeNil()) // expect proper_case_words to be set
		Expect(out.GetHttpProtocolOptions().GetHeaderKeyFormat().GetStatefulFormatter()).To(BeNil())  // ...which makes preserve_case_words nil
	})

	It("should error setting CommonHttpProtocolOptions when an invalid enum value is used", func() {
		upstream.ConnectionConfig = &v1.ConnectionConfig{
			CommonHttpProtocolOptions: &protocol.HttpProtocolOptions{
				HeadersWithUnderscoresAction: 4,
			},
		}

		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).To(HaveOccurred())
	})
})
