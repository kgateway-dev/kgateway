package upstreamconn_test

import (
	"time"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/gogoutils"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/upstreamconn"
)

var _ = Describe("Plugin", func() {

	var (
		params   plugins.Params
		plugin   *Plugin
		upstream *v1.Upstream
		out      *envoyapi.Cluster
	)
	BeforeEach(func() {
		out = new(envoyapi.Cluster)

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
		second := time.Second
		upstream.ConnectionConfig = &v1.ConnectionConfig{
			ConnectTimeout: &second,
		}

		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.GetConnectTimeout()).To(Equal(gogoutils.DurationStdToProto(&second)))
	})

	It("should set TcpKeepalive", func() {
		minute := time.Minute
		hour := time.Hour
		upstream.ConnectionConfig = &v1.ConnectionConfig{
			TcpKeepalive: &v1.ConnectionConfig_TcpKeepAlive{
				KeepaliveInterval: &minute,
				KeepaliveTime:     &hour,
				KeepaliveProbes:   3,
			},
		}

		err := plugin.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		outKeepAlive := out.GetUpstreamConnectionOptions().GetTcpKeepalive()
		expectedValue := envoycore.TcpKeepalive{
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
})
