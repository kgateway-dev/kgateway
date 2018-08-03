package rest_test

import (
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/gogo/protobuf/types"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/pkg/plugins/rest"
)

var _ = Describe("Transformation", func() {
	It("processes response transformations", func() {
		p := NewPlugin()
		out := &envoyroute.Route{}

		upstreamName := "nothing"
		params := &plugins.RoutePluginParams{}

		in := NewNonFunctionSingleDestRoute(upstreamName)
		in.Extensions = EncodeRouteExtension(RouteExtension{
			ResponseTransformation: &TransformationSpec{
				Body: strPtr("{{body}}"),
			},
		})
		err := p.ProcessRoute(params, in, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.Metadata).NotTo(BeNil())
		Expect(out.Metadata.FilterMetadata).To(HaveKey("io.solo.transformation"))
		Expect(out.Metadata.FilterMetadata["io.solo.transformation"].Fields).To(HaveKey("response-transformation"))
		filter, _ := p.HttpFilters(nil)
		Expect(filter).To(HaveLen(1))
		Expect(filter[0].Stage).To(Equal(plugins.PostInAuth))
		Expect(filter[0].HttpFilter.Config).NotTo(BeNil())
		Expect(filter[0].HttpFilter.Config.Fields).To(HaveKey("transformations"))
		Expect(filter[0].HttpFilter.Config.Fields["transformations"].Kind).To(BeAssignableToTypeOf(&types.Value_StructValue{}))
		transformations := filter[0].HttpFilter.Config.Fields["transformations"].Kind.(*types.Value_StructValue)
		Expect(transformations.StructValue.Fields).To(HaveLen(1))
		for k, v := range transformations.StructValue.Fields {
			Expect(v.Kind.(*types.Value_StructValue).StructValue.Fields["transformation_template"]).To(Equal(&types.Value{
				Kind: &types.Value_StructValue{
					StructValue: &types.Struct{
						Fields: map[string]*types.Value{
							"extractors": &types.Value{
								Kind: &types.Value_StructValue{
									StructValue: &types.Struct{
										Fields: map[string]*types.Value{},
									},
								},
							},
							"body": &types.Value{
								Kind: &types.Value_StructValue{
									StructValue: &types.Struct{
										Fields: map[string]*types.Value{
											"text": &types.Value{
												Kind: &types.Value_StringValue{
													StringValue: "{{body}}",
												},
											},
										},
									},
								},
							},
							"headers": &types.Value{
								Kind: &types.Value_StructValue{
									StructValue: &types.Struct{
										Fields: map[string]*types.Value{},
									},
								},
							},
						},
					},
				},
			}))
			// make sure the hashes match
			Expect(out.Metadata.FilterMetadata["io.solo.transformation"].Fields["response-transformation"].Kind.(*types.Value_StringValue).StringValue).To(Equal(k))
		}
	})
	It("process route for functional upstream", func() {
		upstreamName := "users-svc"
		funcName := "get_user"
		params := &plugins.RoutePluginParams{
			Upstreams: []*v1.Upstream{
				NewFunctionalUpstream(upstreamName, funcName),
			},
		}
		in := NewSingleDestRoute(upstreamName, funcName)
		p := NewPlugin()
		out := &envoyroute.Route{}
		err := p.ProcessRoute(params, in, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.Metadata).NotTo(BeNil())
		Expect(out.Metadata.FilterMetadata).To(HaveKey("io.solo.transformation"))
		Expect(out.Metadata.FilterMetadata["io.solo.transformation"].Fields).To(HaveKey("request-transformation"))
		filter, _ := p.HttpFilters(nil)
		Expect(filter).To(HaveLen(1))
		Expect(filter[0].Stage).To(Equal(plugins.PostInAuth))
		Expect(filter[0].HttpFilter.Config).NotTo(BeNil())
		Expect(filter[0].HttpFilter.Config.Fields).To(HaveKey("transformations"))
		Expect(filter[0].HttpFilter.Config.Fields["transformations"].Kind).To(BeAssignableToTypeOf(&types.Value_StructValue{}))
		// gives the free headers automatically
		for hash, trans := range filter[0].HttpFilter.Config.Fields["transformations"].Kind.(*types.Value_StructValue).StructValue.Fields {
			Expect(trans).To(Equal(&types.Value{
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
															"path": {
																Kind: &types.Value_StructValue{
																	StructValue: &types.Struct{
																		Fields: map[string]*types.Value{
																			"subgroup": {
																				Kind: &types.Value_NumberValue{
																					NumberValue: 1.000000,
																				},
																			},
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
																		},
																	},
																},
															},
															"type": {
																Kind: &types.Value_StructValue{
																	StructValue: &types.Struct{
																		Fields: map[string]*types.Value{
																			"header": {
																				Kind: &types.Value_StringValue{
																					StringValue: "content-type",
																				},
																			},
																			"regex": {
																				Kind: &types.Value_StringValue{
																					StringValue: `application/([\-._[:alnum:]]+)`,
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
															"account": {
																Kind: &types.Value_StructValue{
																	StructValue: &types.Struct{
																		Fields: map[string]*types.Value{
																			"subgroup": {
																				Kind: &types.Value_NumberValue{
																					NumberValue: 2.000000,
																				},
																			},
																			"header": {
																				Kind: &types.Value_StringValue{
																					StringValue: ":path",
																				},
																			},
																			"regex": {
																				Kind: &types.Value_StringValue{
																					StringValue: `/u\(se\)rs/([\-._[:alnum:]]+)/accounts/([\-._[:alnum:]]+)`,
																				},
																			},
																		},
																	},
																},
															},
															"foo.bar": {
																Kind: &types.Value_StructValue{
																	StructValue: &types.Struct{
																		Fields: map[string]*types.Value{
																			"header": {
																				Kind: &types.Value_StringValue{
																					StringValue: "x-foo-bar",
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
															"id": {
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
																					StringValue: `/u\(se\)rs/([\-._[:alnum:]]+)/accounts/([\-._[:alnum:]]+)`,
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
														},
													},
												},
											},
											"headers": {
												Kind: &types.Value_StructValue{
													StructValue: &types.Struct{
														Fields: map[string]*types.Value{
															":path": {
																Kind: &types.Value_StructValue{
																	StructValue: &types.Struct{
																		Fields: map[string]*types.Value{
																			"text": {
																				Kind: &types.Value_StringValue{
																					StringValue: "/{{id}}/why/{{id}}",
																				},
																			},
																		},
																	},
																},
															},
															"x-content-type": {
																Kind: &types.Value_StructValue{
																	StructValue: &types.Struct{
																		Fields: map[string]*types.Value{
																			"text": {
																				Kind: &types.Value_StringValue{
																					StringValue: "{{type}}",
																				},
																			},
																		},
																	},
																},
															},
															"x-user-id": {
																Kind: &types.Value_StructValue{
																	StructValue: &types.Struct{
																		Fields: map[string]*types.Value{
																			"text": {
																				Kind: &types.Value_StringValue{
																					StringValue: "{{id}}",
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
											"body": {
												Kind: &types.Value_StructValue{
													StructValue: &types.Struct{
														Fields: map[string]*types.Value{
															"text": {
																Kind: &types.Value_StringValue{
																	StringValue: "{\"id\":\"{{id}}\", \"type\":\"{{type}}\"}",
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
			Expect(out.Metadata.FilterMetadata["io.solo.transformation"].Fields["request-transformation"].Kind).
				To(Equal(&types.Value_StructValue{
					StructValue: &types.Struct{
						Fields: map[string]*types.Value{
							"users-svc": {
								Kind: &types.Value_StructValue{
									StructValue: &types.Struct{
										Fields: map[string]*types.Value{
											"get_user": {
												Kind: &types.Value_StringValue{
													StringValue: hash,
												},
											},
										},
									},
								},
							},
						},
					},
				}))
		}
	})
	It("errors when user provides invalid parameters", func() {
		upstreamName := "users-svc"
		funcName := "get_user"
		params := &plugins.RoutePluginParams{
			Upstreams: []*v1.Upstream{
				NewFunctionalUpstream(upstreamName, funcName),
			},
		}
		in := NewBadExtractorsRoute(upstreamName, funcName)
		p := NewPlugin()
		out := &envoyroute.Route{}
		err := p.ProcessRoute(params, in, out)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`f{foo/bar} is not valid syntax. {} braces must be closed and variable names must satisfy regex ([\-._[:alnum:]]+)`))
	})
})

func NonFunctionalUpstream(name string) *v1.Upstream {
	return &v1.Upstream{
		Name: name,
		Type: "test",
		ServiceInfo: &v1.ServiceInfo{
			Type: ServiceTypeREST,
		},
	}
}

func NewSingleDestRoute(upstreamName, functionName string) *v1.Route {
	return &v1.Route{
		Matcher: &v1.Route_RequestMatcher{
			RequestMatcher: &v1.RequestMatcher{
				Path: &v1.RequestMatcher_PathRegex{
					PathRegex: "/users/.*/accounts/.*",
				},
				Verbs: []string{"GET"},
			},
		},
		SingleDestination: &v1.Destination{
			DestinationType: &v1.Destination_Function{
				Function: &v1.FunctionDestination{
					FunctionName: functionName,
					UpstreamName: upstreamName,
				},
			},
		},
		Extensions: EncodeRouteExtension(RouteExtension{
			Parameters: &Parameters{
				Path: strPtr("/u(se)rs/{id}/accounts/{account}"),
				Headers: map[string]string{
					"content-type": "application/{type}",
					"x-foo-bar":    "{foo.bar}",
				},
			},
		}),
	}
}

func NewBadExtractorsRoute(upstreamName, functionName string) *v1.Route {
	return &v1.Route{
		Matcher: &v1.Route_RequestMatcher{
			RequestMatcher: &v1.RequestMatcher{
				Path: &v1.RequestMatcher_PathRegex{
					PathRegex: "/users/.*/accounts/.*",
				},
				Verbs: []string{"GET"},
			},
		},
		SingleDestination: &v1.Destination{
			DestinationType: &v1.Destination_Function{
				Function: &v1.FunctionDestination{
					FunctionName: functionName,
					UpstreamName: upstreamName,
				},
			},
		},
		Extensions: EncodeRouteExtension(RouteExtension{
			Parameters: &Parameters{
				Path: strPtr("/u(se)rs/{id}/accounts/{account}"),
				Headers: map[string]string{
					"bad-extractor": "f{foo/bar}",
				},
			},
		}),
	}
}

func NewNonFunctionSingleDestRoute(upstreamName string) *v1.Route {
	return &v1.Route{
		Matcher: &v1.Route_RequestMatcher{
			RequestMatcher: &v1.RequestMatcher{
				Path: &v1.RequestMatcher_PathRegex{
					PathRegex: "/users/.*/accounts/.*",
				},
				Verbs: []string{"GET"},
			},
		},
		SingleDestination: &v1.Destination{
			DestinationType: &v1.Destination_Upstream{
				Upstream: &v1.UpstreamDestination{
					Name: upstreamName,
				},
			},
		},
		Extensions: EncodeRouteExtension(RouteExtension{
			Parameters: &Parameters{
				Path:    strPtr("/u(se)rs/{id}/accounts/{account}"),
				Headers: map[string]string{"content-type": "application/{type}"},
			},
		}),
	}
}

func NewFunctionalUpstream(name, funcName string) *v1.Upstream {
	us := NonFunctionalUpstream(name)
	us.Functions = []*v1.Function{
		{
			Name: funcName,
			Spec: EncodeFunctionSpec(TransformationSpec{
				Path: "/{{id}}/why/{{id}}",
				Headers: map[string]string{
					"x-user-id":      "{{id}}",
					"x-content-type": "{{type}}",
				},
				Body: strPtr(`{"id":"{{id}}", "type":"{{type}}"}`),
			}),
		},
	}
	return us
}

func strPtr(s string) *types.StringValue {
	return &types.StringValue{Value:s}
}
