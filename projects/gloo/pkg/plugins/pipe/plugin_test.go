package pipe_test

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1pipe "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/pipe"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/pipe"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Plugin", func() {

	var (
		p            plugins.UpstreamPlugin
		params       plugins.Params
		upstream     *v1.Upstream
		upstreamSpec *v1pipe.UpstreamSpec
		out          *envoy_config_cluster_v3.Cluster
	)

	BeforeEach(func() {
		p = NewPlugin()
		out = new(envoy_config_cluster_v3.Cluster)
		out.Name = "foo"

		p.Init(plugins.InitParams{})
		upstreamSpec = &v1pipe.UpstreamSpec{
			Path: "/foo",
		}
		upstream = &v1.Upstream{
			Metadata: &core.Metadata{
				Name:      "extauth-server",
				Namespace: "default",
			},
			UpstreamType: &v1.Upstream_Pipe{
				Pipe: upstreamSpec,
			},
		}
	})

	It("should translate upstream", func() {
		err := p.ProcessUpstream(params, upstream, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.GetType()).To(Equal(envoy_config_cluster_v3.Cluster_STATIC))
		Expect(out.GetLoadAssignment().GetClusterName()).To(Equal(out.Name))
		addr := out.GetLoadAssignment().GetEndpoints()[0].GetLbEndpoints()[0].GetEndpoint().GetAddress().GetPipe().GetPath()
		Expect(addr).To(Equal("/foo"))
	})

	It("should error with no path", func() {
		upstreamSpec.Path = ""
		err := p.ProcessUpstream(params, upstream, out)
		Expect(err).To(MatchError("no path provided"))
	})

})
