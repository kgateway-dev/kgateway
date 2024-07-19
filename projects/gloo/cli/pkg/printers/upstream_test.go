package printers

import (
	"bytes"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/enterprise/options/gcp"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws/ec2"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/azure"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/consul"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/pipe"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/static"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/grpc_json"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var (
	testNamespace      = "gloo-system"
	service            = "helloworld.Greeter"
	protoDescriptorBin = []byte{10, 230, 1, 10, 16, 104, 101, 108, 108, 111, 119, 111, 114, 108, 100, 46, 112, 114, 111, 116, 111, 18, 10, 104, 101, 108, 108, 111, 119, 111, 114, 108, 100, 34, 28, 10, 12, 72, 101, 108, 108, 111, 82, 101, 113, 117, 101, 115, 116, 18, 12, 10, 4, 110, 97, 109, 101, 24, 1, 32, 1, 40, 9, 34, 29, 10, 10, 72, 101, 108, 108, 111, 82, 101, 112, 108, 121, 18, 15, 10, 7, 109, 101, 115, 115, 97, 103, 101, 24, 1, 32, 1, 40, 9, 50, 73, 10, 7, 71, 114, 101, 101, 116, 101, 114, 18, 62, 10, 8, 83, 97, 121, 72, 101, 108, 108, 111, 18, 24, 46, 104, 101, 108, 108, 111, 119, 111, 114, 108, 100, 46, 72, 101, 108, 108, 111, 82, 101, 113, 117, 101, 115, 116, 26, 22, 46, 104, 101, 108, 108, 111, 119, 111, 114, 108, 100, 46, 72, 101, 108, 108, 111, 82, 101, 112, 108, 121, 34, 0, 66, 54, 10, 27, 105, 111, 46, 103, 114, 112, 99, 46, 101, 120, 97, 109, 112, 108, 101, 115, 46, 104, 101, 108, 108, 111, 119, 111, 114, 108, 100, 66, 15, 72, 101, 108, 108, 111, 87, 111, 114, 108, 100, 80, 114, 111, 116, 111, 80, 1, 162, 2, 3, 72, 76, 87, 98, 6, 112, 114, 111, 116, 111, 51}
)

var _ = Describe("Upstream", func() {

	Describe("Table", func() {
		It("handles malformed upstream (nil spec)", func() {
			Expect(func() {
				us := &v1.Upstream{}
				UpstreamTable(nil, []*v1.Upstream{us}, GinkgoWriter)
			}).NotTo(Panic())
		})

		It("prints grpc upstream function names", func() {
			us := &v1.Upstream{
				Metadata: &core.Metadata{
					Name: "test-us",
				},
				UpstreamType: &v1.Upstream_Kube{
					Kube: &kubernetes.UpstreamSpec{
						ServiceName:      "test",
						ServiceNamespace: testNamespace,
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_GrpcJsonTranscoder{
								GrpcJsonTranscoder: &grpc_json.GrpcJsonTranscoder{
									DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptorBin{
										ProtoDescriptorBin: protoDescriptorBin,
									},
									Services: []string{service},
								},
							},
						},
					},
				},
			}

			var out bytes.Buffer
			UpstreamTable(nil, []*v1.Upstream{us}, &out)
			// The `SayHello` method exists in the ProtoDescriptorBin. This should be printed when listing upstreams.
			// Since there is only one service, it is safe to assume that this method belongs to it
			Expect(out.String()).To(ContainSubstring("- SayHello"))
		})
	})

	Describe("addFunctionsFromGrpcTranscoder", func() {
		var (
			initialStatuses = func() *core.NamespacedStatuses {
				return &core.NamespacedStatuses{
					Statuses: map[string]*core.Status{
						testNamespace: {},
					},
				}
			}

			expDetails = &structpb.Struct{
				Fields: map[string]*structpb.Value{
					functionNamesKey: {
						Kind: &structpb.Value_StructValue{
							StructValue: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									service: {
										Kind: &structpb.Value_ListValue{
											ListValue: &structpb.ListValue{
												Values: []*structpb.Value{
													{
														Kind: &structpb.Value_StringValue{
															StringValue: "SayHello",
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
			}
		)

		DescribeTable("adds function names to namespaced status when there is a grpcJsonTranscoder with descriptors", func(us *v1.Upstream) {
			addFunctionsFromGrpcTranscoder(testNamespace)(us)
			Expect(us.NamespacedStatuses.GetStatuses()[testNamespace].GetDetails()).To(BeEquivalentTo(expDetails))
		},
			Entry("Kube with populated grpcJsonTranscoder", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Kube{
					Kube: &kubernetes.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_GrpcJsonTranscoder{
								GrpcJsonTranscoder: &grpc_json.GrpcJsonTranscoder{
									Services: []string{service},
									DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptorBin{
										ProtoDescriptorBin: protoDescriptorBin,
									},
								},
							},
						},
					},
				}}),
			Entry("Static with populated grpcJsonTranscoder", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Static{
					Static: &static.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_GrpcJsonTranscoder{
								GrpcJsonTranscoder: &grpc_json.GrpcJsonTranscoder{
									Services: []string{service},
									DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptorBin{
										ProtoDescriptorBin: protoDescriptorBin,
									},
								},
							},
						},
					},
				}}),
			Entry("Consul with populated grpcJsonTranscoder", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Consul{
					Consul: &consul.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_GrpcJsonTranscoder{
								GrpcJsonTranscoder: &grpc_json.GrpcJsonTranscoder{
									Services: []string{service},
									DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptorBin{
										ProtoDescriptorBin: protoDescriptorBin,
									},
								},
							},
						},
					},
				}}),
		)

		DescribeTable("does not modify Upstream when there is no grpcJsonTranscoder with selected descriptors", func(us *v1.Upstream) {
			addFunctionsFromGrpcTranscoder(testNamespace)(us)
			Expect(us.NamespacedStatuses.GetStatuses()[testNamespace].GetDetails()).To(BeNil())
		},
			Entry("Pipe", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Pipe{
					Pipe: &pipe.UpstreamSpec{},
				}}),
			Entry("AWS", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Aws{
					Aws: &aws.UpstreamSpec{},
				}}),
			Entry("Azure", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Azure{
					Azure: &azure.UpstreamSpec{},
				}}),
			Entry("AwsEc2", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_AwsEc2{
					AwsEc2: &ec2.UpstreamSpec{},
				}}),
			Entry("Gcp", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Gcp{
					Gcp: &gcp.UpstreamSpec{},
				}}),
			Entry("Kube without ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Kube{
					Kube: &kubernetes.UpstreamSpec{},
				}}),
			Entry("Kube with Rest ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Kube{
					Kube: &kubernetes.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_Rest{},
						},
					},
				}}),
			Entry("Kube with Grpc ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Kube{
					Kube: &kubernetes.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_Grpc{},
						},
					},
				}}),
			Entry("Kube with Graphql ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Kube{
					Kube: &kubernetes.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_Graphql{},
						},
					},
				}}),
			Entry("Kube with empty GrpcJsonTranscoder ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Kube{
					Kube: &kubernetes.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_GrpcJsonTranscoder{},
						},
					},
				}}),
			Entry("Kube with ProtoDescriptor GrpcJsonTranscoder ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Kube{
					Kube: &kubernetes.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_GrpcJsonTranscoder{
								GrpcJsonTranscoder: &grpc_json.GrpcJsonTranscoder{
									DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptor{},
								},
							},
						},
					},
				}}),
			Entry("Kube with ProtoDescriptorConfigMap GrpcJsonTranscoder ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Kube{
					Kube: &kubernetes.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_GrpcJsonTranscoder{
								GrpcJsonTranscoder: &grpc_json.GrpcJsonTranscoder{
									DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptorConfigMap{},
								},
							},
						},
					},
				}}),
			Entry("Kube with empty ProtoDescriptorBin GrpcJsonTranscoder ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Kube{
					Kube: &kubernetes.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_GrpcJsonTranscoder{
								GrpcJsonTranscoder: &grpc_json.GrpcJsonTranscoder{
									DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptorBin{},
								},
							},
						},
					},
				}}),

			Entry("Static without ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Static{
					Static: &static.UpstreamSpec{},
				}}),
			Entry("Static with Rest ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Static{
					Static: &static.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_Rest{},
						},
					},
				}}),
			Entry("Static with Grpc ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Static{
					Static: &static.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_Grpc{},
						},
					},
				}}),
			Entry("Static with Graphql ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Static{
					Static: &static.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_Graphql{},
						},
					},
				}}),
			Entry("Static with empty GrpcJsonTranscoder ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Static{
					Static: &static.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_GrpcJsonTranscoder{},
						},
					},
				}}),
			Entry("Static with ProtoDescriptor GrpcJsonTranscoder ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Static{
					Static: &static.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_GrpcJsonTranscoder{
								GrpcJsonTranscoder: &grpc_json.GrpcJsonTranscoder{
									DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptor{},
								},
							},
						},
					},
				}}),
			Entry("Static with ProtoDescriptorConfigMap GrpcJsonTranscoder ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Static{
					Static: &static.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_GrpcJsonTranscoder{
								GrpcJsonTranscoder: &grpc_json.GrpcJsonTranscoder{
									DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptorConfigMap{},
								},
							},
						},
					},
				}}),
			Entry("Static with empty ProtoDescriptorBin GrpcJsonTranscoder ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Static{
					Static: &static.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_GrpcJsonTranscoder{
								GrpcJsonTranscoder: &grpc_json.GrpcJsonTranscoder{
									DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptorBin{},
								},
							},
						},
					},
				}}),

			Entry("Consul without ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Consul{
					Consul: &consul.UpstreamSpec{},
				}}),
			Entry("Consul with Rest ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Consul{
					Consul: &consul.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_Rest{},
						},
					},
				}}),
			Entry("Consul with Grpc ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Consul{
					Consul: &consul.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_Grpc{},
						},
					},
				}}),
			Entry("Consul with Graphql ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Consul{
					Consul: &consul.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_Graphql{},
						},
					},
				}}),
			Entry("Consul with empty GrpcJsonTranscoder ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Consul{
					Consul: &consul.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_GrpcJsonTranscoder{},
						},
					},
				}}),
			Entry("Consul with ProtoDescriptor GrpcJsonTranscoder ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Consul{
					Consul: &consul.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_GrpcJsonTranscoder{
								GrpcJsonTranscoder: &grpc_json.GrpcJsonTranscoder{
									DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptor{},
								},
							},
						},
					},
				}}),
			Entry("Consul with ProtoDescriptorConfigMap GrpcJsonTranscoder ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Consul{
					Consul: &consul.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_GrpcJsonTranscoder{
								GrpcJsonTranscoder: &grpc_json.GrpcJsonTranscoder{
									DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptorConfigMap{},
								},
							},
						},
					},
				}}),
			Entry("Consul with empty ProtoDescriptorBin GrpcJsonTranscoder ServiceSpec", &v1.Upstream{
				NamespacedStatuses: initialStatuses(),
				UpstreamType: &v1.Upstream_Consul{
					Consul: &consul.UpstreamSpec{
						ServiceSpec: &options.ServiceSpec{
							PluginType: &options.ServiceSpec_GrpcJsonTranscoder{
								GrpcJsonTranscoder: &grpc_json.GrpcJsonTranscoder{
									DescriptorSet: &grpc_json.GrpcJsonTranscoder_ProtoDescriptorBin{},
								},
							},
						},
					},
				}}),
		)
	})
})
