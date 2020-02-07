package static

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

var _ = Describe("Plugin", func() {

	var (
		p            *plugin
		params       plugins.Params
		upstream     *v1.Upstream
		upstreamSpec *v1static.UpstreamSpec
		out          *envoyapi.Cluster
	)

	BeforeEach(func() {
		p = new(plugin)
		out = new(envoyapi.Cluster)

		p.Init(plugins.InitParams{})
		upstreamSpec = &v1static.UpstreamSpec{
			Hosts: []*v1static.Host{{
				Addr: "localhost",
				Port: 1234,
			}},
		}
		upstream = &v1.Upstream{
			Metadata: core.Metadata{
				Name:      "extauth-server",
				Namespace: "default",
			},
			UpstreamType: &v1.Upstream_Static{
				Static: upstreamSpec,
			},
		}

	})

	Context("h2", func() {

		It("should not http 2 by default", func() {
			p.ProcessUpstream(params, upstream, out)
			Expect(out.Http2ProtocolOptions).To(BeNil())
		})
	})

	Context("cluster type", func() {

		It("use strict dns", func() {
			p.ProcessUpstream(params, upstream, out)
			Expect(out.GetType()).To(Equal(envoyapi.Cluster_STRICT_DNS))
		})

		It("use static if only has ips", func() {
			upstreamSpec.Hosts = []*v1static.Host{{
				Addr: "1.2.3.4",
				Port: 1234,
			}, {
				Addr: "2603:3005:b0b:1d00::b7aa",
				Port: 1234,
			}}

			p.ProcessUpstream(params, upstream, out)
			Expect(out.GetType()).To(Equal(envoyapi.Cluster_STATIC))
		})

		It("use dns if has mixed addresses", func() {
			upstreamSpec.Hosts = []*v1static.Host{{
				Addr: "test.solo.io",
				Port: 1234,
			}, {
				Addr: "1.2.3.4",
				Port: 1234,
			}}

			p.ProcessUpstream(params, upstream, out)
			Expect(out.GetType()).To(Equal(envoyapi.Cluster_STRICT_DNS))
		})
	})

	Context("ssl", func() {
		tlsContext := func() *envoyauth.UpstreamTlsContext {
			if out.TransportSocket == nil {
				return nil
			}
			return pluginutils.MustAnyToMessage(out.TransportSocket.GetTypedConfig()).(*envoyauth.UpstreamTlsContext)
		}
		It("doesn't have ssl by default", func() {
			p.ProcessUpstream(params, upstream, out)
			Expect(tlsContext()).To(BeNil())
		})

		It("should autodetect ssl", func() {
			upstreamSpec.Hosts[0].Port = 443
			p.ProcessUpstream(params, upstream, out)
			Expect(tlsContext()).ToNot(BeNil())
		})

		It("should allow configuring ssl", func() {
			upstreamSpec.UseTls = true
			p.ProcessUpstream(params, upstream, out)
			Expect(tlsContext()).ToNot(BeNil())
		})

		It("should not override existing tls config", func() {
			existing := &envoyauth.UpstreamTlsContext{}
			out.TransportSocket = &envoycore.TransportSocket{
				Name:       pluginutils.TlsTransportSocket,
				ConfigType: &envoycore.TransportSocket_TypedConfig{TypedConfig: pluginutils.MustMessageToAny(existing)},
			}
			upstreamSpec.UseTls = true
			p.ProcessUpstream(params, upstream, out)
			Expect(tlsContext()).To(Equal(existing))
		})
	})
})
