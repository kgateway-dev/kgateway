package static

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/hostrewrite"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/static"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/utils"
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
			UpstreamSpec: &v1.UpstreamSpec{
				UpstreamType: &v1.UpstreamSpec_Static{
					Static: upstreamSpec,
				},
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
		It("doesn't have ssl by default", func() {
			p.ProcessUpstream(params, upstream, out)
			Expect(out.TlsContext).To(BeNil())
		})

		It("should autodetect ssl", func() {
			upstreamSpec.Hosts[0].Port = 443
			p.ProcessUpstream(params, upstream, out)
			Expect(out.TlsContext).ToNot(BeNil())
		})

		It("should allow configuring ssl", func() {
			upstreamSpec.UseTls = true
			p.ProcessUpstream(params, upstream, out)
			Expect(out.TlsContext).ToNot(BeNil())
		})

		It("should not override existing tls config", func() {
			existing := &envoyauth.UpstreamTlsContext{}
			out.TlsContext = existing
			upstreamSpec.UseTls = true
			p.ProcessUpstream(params, upstream, out)
			Expect(out.TlsContext).To(BeIdenticalTo(existing))
		})
	})

	Context("host re-write", func() {
		var (
			outRouteAction    *envoyroute.RouteAction
			paramsRouteAction plugins.RouteActionParams
			inRoute           *v1.Route
			inRouteAction     *v1.RouteAction
		)
		BeforeEach(func() {
			outRouteAction = new(envoyroute.RouteAction)
			inRoute = &v1.Route{}
			paramsRouteAction = plugins.RouteActionParams{Route: inRoute}
			inRouteAction = &v1.RouteAction{
				Destination: &v1.RouteAction_Single{
					Single: &v1.Destination{
						DestinationType: &v1.Destination_Upstream{
							Upstream: utils.ResourceRefPtr(upstream.Metadata.Ref()),
						},
					},
				},
			}

		})
		It("host rewrites with an address", func() {
			p.ProcessUpstream(params, upstream, out)
			p.ProcessRouteAction(paramsRouteAction, inRouteAction, outRouteAction)
			Expect(outRouteAction.GetAutoHostRewrite().GetValue()).To(Equal(true))
		})
		It("host rewrites address but disabled", func() {
			upstreamSpec.AutoHostRewrite = &types.BoolValue{Value: false}
			p.ProcessUpstream(params, upstream, out)
			p.ProcessRouteAction(paramsRouteAction, inRouteAction, outRouteAction)
			Expect(outRouteAction.GetAutoHostRewrite().GetValue()).To(Equal(false))
		})
		It("host rewrites with an ip", func() {
			upstreamSpec.Hosts = []*v1static.Host{{
				Addr: "1.2.3.4",
				Port: 1234,
			}}
			p.ProcessUpstream(params, upstream, out)
			p.ProcessRouteAction(paramsRouteAction, inRouteAction, outRouteAction)
			Expect(outRouteAction.GetAutoHostRewrite().GetValue()).To(Equal(false))
		})
		It("host rewrites with an ip but enabled", func() {
			upstreamSpec.Hosts = []*v1static.Host{{
				Addr: "1.2.3.4",
				Port: 1234,
			}}
			upstreamSpec.AutoHostRewrite = &types.BoolValue{Value: true}
			p.ProcessUpstream(params, upstream, out)
			p.ProcessRouteAction(paramsRouteAction, inRouteAction, outRouteAction)
			Expect(outRouteAction.GetAutoHostRewrite().GetValue()).To(Equal(true))
		})
		It("host rewrites enabled but route has a rewrite already set", func() {
			inRoute.RoutePlugins = &v1.RoutePlugins{
				HostRewrite: &hostrewrite.HostRewrite{
					HostRewriteType: &hostrewrite.HostRewrite_HostRewrite{
						HostRewrite: "test",
					},
				},
			}
			upstreamSpec.AutoHostRewrite = &types.BoolValue{Value: true}
			p.ProcessUpstream(params, upstream, out)
			p.ProcessRouteAction(paramsRouteAction, inRouteAction, outRouteAction)
			Expect(outRouteAction.GetAutoHostRewrite().GetValue()).To(Equal(false))
		})
	})
})
