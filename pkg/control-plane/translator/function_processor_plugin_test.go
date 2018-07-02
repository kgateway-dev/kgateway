package translator

import (
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/aws"
	// . "github.com/solo-io/gloo/test/helpers"
	// . "github.com/solo-io/gloo/pkg/translator"

	"github.com/k0kubun/pp"
	"github.com/solo-io/gloo/pkg/plugins"

	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("FinalizerPlugin", func() {
	It("should set the function metadata for functional clusters", func() {
		funcs := []plugins.FunctionPlugin{&aws.Plugin{}}
		finalizerPlugin := newFunctionalPluginProcessor(funcs)

		out := envoyapi.Cluster{Metadata: new(envoycore.Metadata)}

		in := &v1.Upstream{
			Name: "something",
			Type: aws.UpstreamTypeAws,
			Spec: aws.EncodeUpstreamSpec(aws.UpstreamSpec{
				Region:    "us-east-1",
				SecretRef: "my-aws-creds",
			}),
			Functions: []*v1.Function{
				{
					Name: "func1",
					Spec: aws.EncodeFunctionSpec(aws.FunctionSpec{
						FunctionName: "func1",
						Qualifier:    "qualifier1",
					}),
				},
				{
					Name: "func2",
					Spec: aws.EncodeFunctionSpec(aws.FunctionSpec{
						FunctionName: "func2",
						Qualifier:    "qualifier2",
					}),
				},
			},
		}

		params := &plugins.UpstreamPluginParams{}
		pp.Fprintln(GinkgoWriter, in)

		err := finalizerPlugin.ProcessUpstream(params, in, &out)
		pp.Fprintln(GinkgoWriter, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.Metadata).To(Equal(&envoycore.Metadata{
			FilterMetadata: map[string]*types.Struct{
				"io.solo.function_router": {
					Fields: map[string]*types.Value{
						"functions": {
							Kind: &types.Value_StructValue{
								StructValue: &types.Struct{
									Fields: map[string]*types.Value{
										"func1": {
											Kind: &types.Value_StructValue{
												StructValue: &types.Struct{
													Fields: map[string]*types.Value{
														"name": {
															Kind: &types.Value_StringValue{
																StringValue: "func1",
															},
														},
														"qualifier": {
															Kind: &types.Value_StringValue{
																StringValue: "qualifier1",
															},
														},
													},
												},
											},
										},
										"func2": {
											Kind: &types.Value_StructValue{
												StructValue: &types.Struct{
													Fields: map[string]*types.Value{
														"name": {
															Kind: &types.Value_StringValue{
																StringValue: "func2",
															},
														},
														"qualifier": {
															Kind: &types.Value_StringValue{
																StringValue: "qualifier2",
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}))
	})
})
