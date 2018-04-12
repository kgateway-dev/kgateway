package aws_test

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/pkg/plugins/aws"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
)

var _ = Describe("Plugin", func() {
	Context("With a list of upstreams", func() {
		It("should return a list of secret refs for dependencies", func() {
			cfg := &v1.Config{
				Upstreams: []*v1.Upstream{
					&v1.Upstream{},
					upstream("aws1", "us-east-1", "aws-secret1"),
					upstream("aws2", "us-east-1", "aws-secret2"),
					upstream("aws3", "us-east-1", "aws-secret1"),
					upstream("aws4", "us-east-1", ""),             // invalid one
					upstream("aws5", "non-region", "aws-secret5"), // invalid one
				},
			}
			p := Plugin{}
			dependencies := p.GetDependencies(cfg)
			Expect(dependencies.SecretRefs).To(HaveLen(3))
			Expect(dependencies.SecretRefs[0]).To(Equal("aws-secret1"))
			Expect(dependencies.SecretRefs).ToNot(ContainElement("aws-secret5"))
		})
	})

	Describe("Processing upstream", func() {
		Context("With non-AWS upstream", func() {
			It("should not error and return nothing", func() {
				upstreams := []*v1.Upstream{&v1.Upstream{}, &v1.Upstream{Type: "some-upstream"}}
				p := Plugin{}
				for _, u := range upstreams {
					err := p.ProcessUpstream(nil, u, nil)
					Expect(err).NotTo(HaveOccurred())
				}
			})
		})

		Context("When secret referenced by AWS upstream is missing", func() {
			It("should error", func() {
				upstream := &v1.Upstream{
					Type: UpstreamTypeAws,
					Spec: upstreamSpec("us-east-1", "aws-secret"),
				}
				out := &envoyapi.Cluster{}
				params := &plugins.UpstreamPluginParams{}
				p := Plugin{}
				err := p.ProcessUpstream(params, upstream, out)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("When referenced secret is invalid", func() {
			It("should error", func() {
				secrets := []map[string]string{
					{},
					{AwsAccessKey: "", AwsSecretKey: "ball"},
					{AwsAccessKey: "apple", AwsSecretKey: ""},
					{AwsAccessKey: string([]byte{0xff, 1}), AwsSecretKey: "ball"},
					{AwsAccessKey: "apple", AwsSecretKey: string([]byte{0xff, 1})},
				}
				p := Plugin{}
				upstream := &v1.Upstream{
					Type: UpstreamTypeAws,
					Spec: upstreamSpec("us-east-1", "aws-secret"),
				}
				out := &envoyapi.Cluster{}
				for _, s := range secrets {
					params := &plugins.UpstreamPluginParams{Secrets: map[string]*dependencies.Secret{
						"aws-secret": {Ref: "aws-secret", Data: s},
					}}
					err := p.ProcessUpstream(params, upstream, out)
					Expect(err).To(HaveOccurred())
				}
			})
		})

		Context("With valid upstream spec", func() {
			var (
				err error
				p   Plugin
				out *envoyapi.Cluster
			)
			BeforeEach(func() {
				p = Plugin{}
				upstream := &v1.Upstream{
					Type: UpstreamTypeAws,
					Spec: upstreamSpec("us-east-1", "aws-secret"),
				}
				out = &envoyapi.Cluster{}
				params := &plugins.UpstreamPluginParams{Secrets: map[string]*dependencies.Secret{
					"aws-secret": {Ref: "aws-secret", Data: map[string]string{AwsAccessKey: "apple", AwsSecretKey: "ball"}},
				}}
				err = p.ProcessUpstream(params, upstream, out)
			})

			It("should not error", func() {
				Expect(err).NotTo(HaveOccurred())
			})

			It("should have a host in output", func() {
				Expect(out.Hosts).Should(HaveLen(1))
				Expect(out.Hosts[0].GetSocketAddress()).NotTo(BeNil())
			})

			It("should have region in the output host", func() {
				Expect(out.Hosts[0].GetSocketAddress()).To(ContainSubstring("us-east-1"))
			})
		})
	})

	Describe("Processing function", func() {
		Context("with non AWS upstream", func() {
			It("should return nil and not error", func() {
				p := Plugin{}
				nonAWS := &plugins.FunctionPluginParams{}
				out, err := p.ParseFunctionSpec(nonAWS, funcSpec("func1", "v1"))
				Expect(err).NotTo(HaveOccurred())
				Expect(out).To(BeNil())
			})
		})

		Context("with valid function spec", func() {
			It("Should return name and qualifier", func() {
				p := Plugin{}
				param := &plugins.FunctionPluginParams{UpstreamType: UpstreamTypeAws}
				out, err := p.ParseFunctionSpec(param, funcSpec("func1", "v1"))
				Expect(err).NotTo(HaveOccurred())
				Expect(get(out, "name")).To(Equal("func1"))
				Expect(get(out, "qualifier")).To(Equal("v1"))
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

func upstream(name, region, secretRef string) *v1.Upstream {
	return &v1.Upstream{
		Name: name,
		Type: UpstreamTypeAws,
		Spec: upstreamSpec(region, secretRef),
	}
}
