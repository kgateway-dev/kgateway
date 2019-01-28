package grpc

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	pluginsv1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins"
	v1grpc "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/grpc"
	v1static "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/static"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
)

var _ = Describe("Plugin", func() {

	var (
		p            *plugin
		params       plugins.Params
		upstream     *v1.Upstream
		upstreamSpec *v1static.UpstreamSpec
		out          *envoyapi.Cluster
		grpcSepc     *pluginsv1.ServiceSpec_Grpc
	)

	BeforeEach(func() {
		p = new(plugin)
		out = new(envoyapi.Cluster)

		grpcSepc = &pluginsv1.ServiceSpec_Grpc{
			Grpc: &v1grpc.ServiceSpec{},
		}

		p.Init(plugins.InitParams{})
		upstreamSpec = &v1static.UpstreamSpec{
			ServiceSpec: &pluginsv1.ServiceSpec{
				PluginType: grpcSepc,
			},
			Hosts: []*v1static.Host{{
				Addr: "localhost",
				Port: 1234,
			}},
		}
		upstream = &v1.Upstream{
			Metadata: core.Metadata{
				Name:      "test",
				Namespace: "default",
			},
			UpstreamSpec: &v1.UpstreamSpec{
				UpstreamType: &v1.UpstreamSpec_Static{
					Static: upstreamSpec,
				},
			},
		}

	})

	It("should not mark none grpc upstreams as http2", func() {
		upstreamSpec.ServiceSpec.PluginType = nil
		err := p.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.Http2ProtocolOptions).To(BeNil())
	})

	It("should mark grpc upstreams as http2", func() {
		err := p.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.Http2ProtocolOptions).NotTo(BeNil())
	})

})
