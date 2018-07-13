package google

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins"
)

var _ = Describe("Plugin", func() {
	Describe("Processing upstream", func() {
		Context("With non-Google upstream", func() {
			It("should not error and return nothing", func() {
				upstreams := []*v1.Upstream{&v1.Upstream{}, &v1.Upstream{Type: "some-upstream"}}
				p := Plugin{}
				for _, u := range upstreams {
					err := p.ProcessUpstream(nil, u, nil)
					Expect(err).NotTo(HaveOccurred())
				}
			})
		})

		Context("with valid upstream spec", func() {
			var (
				err error
				out *envoyapi.Cluster
			)

			BeforeEach(func() {
				upstream := &v1.Upstream{
					Type: UpstreamTypeGoogle,
					Spec: upstreamSpec("us-east1", "project-x"),
				}
				out = &envoyapi.Cluster{}
				p := Plugin{}
				err = p.ProcessUpstream(nil, upstream, out)
			})

			It("should not error", func() {
				Expect(err).NotTo(HaveOccurred())
			})

			It("should have a host in output", func() {
				Expect(out.Hosts).Should(HaveLen(1))
				Expect(out.Hosts[0].GetSocketAddress()).NotTo(BeNil())
			})

			It("should have region and project in the output host", func() {
				Expect(out.Hosts[0].GetSocketAddress().Address).To(Equal("us-east1-project-x.cloudfunctions.net"))
			})
		})
	})

	Describe("Processing function", func() {
		Context("with non Google upstream", func() {
			It("should return nil and not error", func() {
				p := Plugin{}
				nonGoogle := &plugins.FunctionPluginParams{}
				out, err := p.ParseFunctionSpec(nonGoogle, funcSpec("http://solo.io/gloo"))
				Expect(err).NotTo(HaveOccurred())
				Expect(out).To(BeNil())
			})
		})

		Context("with valid function spec", func() {
			It("Should return name and qualifier", func() {
				p := Plugin{}
				param := &plugins.FunctionPluginParams{UpstreamType: UpstreamTypeGoogle}
				out, err := p.ParseFunctionSpec(param, funcSpec("https://host.io/func"))
				Expect(err).NotTo(HaveOccurred())
				Expect(get(out, "host")).To(Equal("host.io"))
				Expect(get(out, "path")).To(Equal("/func"))
			})
		})
	})
})

func get(s *types.Struct, key string) string {
	v, ok := s.Fields[key]
	if !ok {
		return ""
	}
	sv, ok := v.Kind.(*types.Value_StringValue)
	if !ok {
		return ""
	}
	return sv.StringValue
}
