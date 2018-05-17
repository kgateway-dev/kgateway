package grpc

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"

	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
)

var _ = Describe("Plugin", func() {
	It("returns a dependency with a file ref for each descriptor file "+
		"specified in the spec", func() {
		us := &v1.Upstream{
			ServiceInfo: &v1.ServiceInfo{
				Type: ServiceTypeGRPC,
				Properties: EncodeServiceProperties(ServiceProperties{
					DescriptorsFileRef: "file_1",
				}),
			},
		}
		plugin := &Plugin{}
		deps := plugin.GetDependencies(&v1.Config{Upstreams: []*v1.Upstream{us}})
		Expect(deps.FileRefs).To(HaveLen(1))
		Expect(deps.FileRefs[0]).To(Equal("file_1"))
	})
	Describe("ProcessUpstream", func() {
		It("Marks the cluster metadata with the transformation filter", func() {
			in := &v1.Upstream{
				Name: "myupstream",
				ServiceInfo: &v1.ServiceInfo{
					Type: ServiceTypeGRPC,
					Properties: EncodeServiceProperties(ServiceProperties{
						DescriptorsFileRef: "file_1",
						GRPCServiceNames:   []string{"Bookstore"},
					}),
				},
			}
			p := NewPlugin()
			b, err := ioutil.ReadFile("test/proto.pb")
			Expect(err).NotTo(HaveOccurred())
			params := &plugins.UpstreamPluginParams{
				Files: map[string]*dependencies.File{"file_1": {Ref: "file_1", Contents: b}},
			}
			out := &envoyapi.Cluster{}
			err = p.ProcessUpstream(params, in, out)
			Expect(err).To(BeNil())
			Expect(out.Metadata).NotTo(BeNil())
			Expect(out.Metadata.FilterMetadata).NotTo(BeNil())
			Expect(out.Metadata.FilterMetadata).To(HaveKey("io.solo.transformation"))
		})

		It("processes two upstreams with the same service", func() {
			in1 := &v1.Upstream{
				Name: "myupstream1",
				ServiceInfo: &v1.ServiceInfo{
					Type: ServiceTypeGRPC,
					Properties: EncodeServiceProperties(ServiceProperties{
						DescriptorsFileRef: "file_1",
						GRPCServiceNames:   []string{"Bookstore"},
					}),
				},
			}
			in2 := &v1.Upstream{
				Name: "myupstream2",
				ServiceInfo: &v1.ServiceInfo{
					Type: ServiceTypeGRPC,
					Properties: EncodeServiceProperties(ServiceProperties{
						DescriptorsFileRef: "file_1",
						GRPCServiceNames:   []string{"Bookstore"},
					}),
				},
			}
			p := NewPlugin()
			b, err := ioutil.ReadFile("test/proto.pb")
			Expect(err).NotTo(HaveOccurred())
			params := &plugins.UpstreamPluginParams{
				Files: map[string]*dependencies.File{"file_1": {Ref: "file_1", Contents: b}},
			}
			out := &envoyapi.Cluster{}
			err = p.ProcessUpstream(params, in1, out)
			Expect(err).NotTo(HaveOccurred())

			out = &envoyapi.Cluster{}
			err = p.ProcessUpstream(params, in2, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(p.upstreamServices).To(HaveLen(2))
		})

		It("Stores the descriptors proto in the plugin memory and adds to it http rules", func() {
			in := &v1.Upstream{
				Name: "myupstream",
				ServiceInfo: &v1.ServiceInfo{
					Type: ServiceTypeGRPC,
					Properties: EncodeServiceProperties(ServiceProperties{
						DescriptorsFileRef: "file_1",
						GRPCServiceNames:   []string{"Bookstore"},
					}),
				},
			}
			p := NewPlugin()
			b, err := ioutil.ReadFile("test/proto.pb")
			Expect(err).NotTo(HaveOccurred())
			params := &plugins.UpstreamPluginParams{
				Files: map[string]*dependencies.File{"file_1": {Ref: "file_1", Contents: b}},
			}
			out := &envoyapi.Cluster{}
			err = p.ProcessUpstream(params, in, out)
			Expect(err).To(BeNil())
			Expect(p.upstreamServices["myupstream"].ServiceNames).To(HaveLen(1))
			Expect(p.upstreamServices["myupstream"].PackageNames).To(HaveLen(1))
			Expect(p.upstreamServices["myupstream"].PackageNames[0]).To(Equal("bookstore"))
			Expect(p.upstreamServices["myupstream"].ServiceNames[0]).To(Equal("Bookstore"))
			Expect(p.upstreamServices["myupstream"].Descriptors).NotTo(BeNil())
			route := &v1.Route{
				Matcher: &v1.Route_RequestMatcher{
					RequestMatcher: &v1.RequestMatcher{
						Path: &v1.RequestMatcher_PathExact{PathExact: "/test"},
					},
				},
				SingleDestination: &v1.Destination{
					DestinationType: &v1.Destination_Function{
						Function: &v1.FunctionDestination{
							UpstreamName: "myupstream",
							FunctionName: "ListShelves",
						},
					},
				},
			}
			outRoute := &envoyroute.Route{}
			err = p.ProcessRoute(nil, route, outRoute)
			Expect(err).To(BeNil())
			filters := p.HttpFilters(nil)
			Expect(filters).To(HaveLen(2))
			Expect(filters[0].HttpFilter.Name).To(Equal("io.solo.transformation"))
			Expect(filters[0].HttpFilter.Config.Fields["transformations"].Kind.(*types.Value_StructValue).StructValue.Fields).
				To(HaveLen(1))
			for _, v := range filters[0].HttpFilter.Config.Fields["transformations"].Kind.(*types.Value_StructValue).StructValue.Fields {
				Expect(v).To(Equal(&types.Value{
					Kind: &types.Value_StructValue{
						StructValue: &types.Struct{
							Fields: map[string]*types.Value{
								"transformation_template": {
									Kind: &types.Value_StructValue{
										StructValue: &types.Struct{
											Fields: map[string]*types.Value{
												"extractors": {
													Kind: &types.Value_StructValue{
														StructValue: &types.Struct{
															Fields: map[string]*types.Value{
																"method": {
																	Kind: &types.Value_StructValue{
																		StructValue: &types.Struct{
																			Fields: map[string]*types.Value{
																				"header": {
																					Kind: &types.Value_StringValue{
																						StringValue: ":method",
																					},
																				},
																				"regex": {
																					Kind: &types.Value_StringValue{
																						StringValue: `([\-._[:alnum:]]+)`,
																					},
																				},
																				"subgroup": {
																					Kind: &types.Value_NumberValue{
																						NumberValue: 1.000000,
																					},
																				},
																			},
																		},
																	},
																},
																"path": {
																	Kind: &types.Value_StructValue{
																		StructValue: &types.Struct{
																			Fields: map[string]*types.Value{
																				"header": {
																					Kind: &types.Value_StringValue{
																						StringValue: ":path",
																					},
																				},
																				"regex": {
																					Kind: &types.Value_StringValue{
																						StringValue: `([\-._[:alnum:]]+)`,
																					},
																				},
																				"subgroup": {
																					Kind: &types.Value_NumberValue{
																						NumberValue: 1.000000,
																					},
																				},
																			},
																		},
																	},
																},
																"query_string": {
																	Kind: &types.Value_StructValue{
																		StructValue: &types.Struct{
																			Fields: map[string]*types.Value{
																				"header": {
																					Kind: &types.Value_StringValue{
																						StringValue: ":path",
																					},
																				},
																				"regex": {
																					Kind: &types.Value_StringValue{
																						StringValue: `/test\?([\-._[:alnum:]]+)`,
																					},
																				},
																				"subgroup": {
																					Kind: &types.Value_NumberValue{
																						NumberValue: 1.000000,
																					},
																				},
																			},
																		},
																	},
																},
																"scheme": {
																	Kind: &types.Value_StructValue{
																		StructValue: &types.Struct{
																			Fields: map[string]*types.Value{
																				"header": {
																					Kind: &types.Value_StringValue{
																						StringValue: ":scheme",
																					},
																				},
																				"regex": {
																					Kind: &types.Value_StringValue{
																						StringValue: `([\-._[:alnum:]]+)`,
																					},
																				},
																				"subgroup": {
																					Kind: &types.Value_NumberValue{
																						NumberValue: 1.000000,
																					},
																				},
																			},
																		},
																	},
																},
																"authority": {
																	Kind: &types.Value_StructValue{
																		StructValue: &types.Struct{
																			Fields: map[string]*types.Value{
																				"header": {
																					Kind: &types.Value_StringValue{
																						StringValue: ":authority",
																					},
																				},
																				"regex": {
																					Kind: &types.Value_StringValue{
																						StringValue: `([\-._[:alnum:]]+)`,
																					},
																				},
																				"subgroup": {
																					Kind: &types.Value_NumberValue{
																						NumberValue: 1.000000,
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
												"headers": {
													Kind: &types.Value_StructValue{
														StructValue: &types.Struct{
															Fields: map[string]*types.Value{
																":method": {
																	Kind: &types.Value_StructValue{
																		StructValue: &types.Struct{
																			Fields: map[string]*types.Value{
																				"text": {
																					Kind: &types.Value_StringValue{
																						StringValue: "POST",
																					},
																				},
																			},
																		},
																	},
																},
																":path": {
																	Kind: &types.Value_StructValue{
																		StructValue: &types.Struct{
																			Fields: map[string]*types.Value{
																				"text": {
																					Kind: &types.Value_StringValue{
																						StringValue: "/4f527d09/myupstream/bookstore.Bookstore/ListShelves?{{ default(query_string), \"\")}}",
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
												"merge_extractors_to_body": {
													Kind: &types.Value_StructValue{
														StructValue: &types.Struct{
															Fields: map[string]*types.Value{},
														},
													},
												},
											},
										},
									}}}}}}))
			}
			Expect(filters[1].HttpFilter.Name).To(Equal("envoy.grpc_json_transcoder"))
			Expect(filters[1].HttpFilter.Config.Fields["services"]).To(Equal(&types.Value{
				Kind: &types.Value_ListValue{
					ListValue: &types.ListValue{
						Values: []*types.Value{
							&types.Value{
								Kind: &types.Value_StringValue{
									StringValue: "bookstore.Bookstore",
								},
							},
						},
					},
				},
			}))
			Expect(filters[1].HttpFilter.Config.Fields["match_incoming_request_route"]).To(Equal(&types.Value{
				Kind: &types.Value_BoolValue{
					BoolValue: true,
				},
			}))
			// TODO: test the finished proto
		})
	})
})
