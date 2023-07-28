package translator_test

import (
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	gloov1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	glootransformation "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var _ = Describe("Route converter", func() {
	var (
		snapshot *gloov1snap.ApiSnapshot
	)
	gw := &v1.Gateway{
		Metadata: &core.Metadata{
			Name:      "gw1",
			Namespace: "gw-ns",
		},
	}
	DescribeTable("should detect bad config on a delegate route",
		func(route *v1.Route, expectedErr, expectedWarning error, rts translator.RouteTableSelector) {
			reports := reporter.ResourceReports{}
			vs := &v1.VirtualService{
				Metadata: &core.Metadata{
					Name:      "foo",
					Namespace: "bar",
				},
				VirtualHost: &v1.VirtualHost{
					Routes: []*v1.Route{route},
				},
			}

			rv := translator.NewRouteConverter(rts, nil)
			rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, reports)

			// One error on the VS, one on the RT
			Expect(reports).To(HaveLen(1))
			_, vsReport := reports.Find("*v1.VirtualService", vs.Metadata.Ref())
			if expectedErr != nil {
				Expect(vsReport.Errors).To(HaveOccurred())
				Expect(vsReport.Errors).To(MatchError(ContainSubstring(expectedErr.Error())))
			} else {
				Expect(vsReport.Errors).NotTo(HaveOccurred())
			}
			if expectedWarning != nil {
				Expect(vsReport.Warnings).To(HaveLen(1))
				Expect(vsReport.Warnings[0]).To(ContainSubstring(expectedWarning.Error()))
			} else {
				Expect(vsReport.Warnings).To(BeEmpty())
			}
		},

		Entry("route has a regex matcher",
			&v1.Route{
				Matchers: []*matchers.Matcher{{
					PathSpecifier: &matchers.Matcher_Regex{
						Regex: "/any",
					},
				}},
				Action: &v1.Route_DelegateAction{
					DelegateAction: &v1.DelegateAction{
						DelegationType: &v1.DelegateAction_Ref{
							Ref: &core.ResourceRef{
								Name:      "any",
								Namespace: "ns",
							},
						},
					},
				},
			},
			translator.MissingPrefixErr,
			nil,
			nil,
		),

		Entry("route has an exact matcher",
			&v1.Route{
				Matchers: []*matchers.Matcher{{
					PathSpecifier: &matchers.Matcher_Exact{
						Exact: "/any",
					},
				}},
				Action: &v1.Route_DelegateAction{
					DelegateAction: &v1.DelegateAction{
						DelegationType: &v1.DelegateAction_Ref{
							Ref: &core.ResourceRef{
								Name:      "any",
								Namespace: "ns",
							},
						},
					},
				},
			},
			translator.MissingPrefixErr,
			nil,
			nil,
		),

		Entry("route has an connect matcher",
			&v1.Route{
				Matchers: []*matchers.Matcher{{
					PathSpecifier: &matchers.Matcher_ConnectMatcher_{
						ConnectMatcher: &matchers.Matcher_ConnectMatcher{},
					},
				}},
				Action: &v1.Route_DelegateAction{
					DelegateAction: &v1.DelegateAction{
						DelegationType: &v1.DelegateAction_Ref{
							Ref: &core.ResourceRef{
								Name:      "any",
								Namespace: "ns",
							},
						},
					},
				},
			},
			translator.MissingPrefixErr,
			nil,
			nil,
		),

		Entry("route has multiple path prefix matchers",
			&v1.Route{
				Matchers: []*matchers.Matcher{
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/foo",
						},
					},
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/bar",
						},
					},
				},
				Action: &v1.Route_DelegateAction{
					DelegateAction: &v1.DelegateAction{
						DelegationType: &v1.DelegateAction_Ref{
							Ref: &core.ResourceRef{
								Name: "foo",
							},
						},
					},
				},
			},
			translator.MatcherCountErr,
			nil,
			nil,
		),

		Entry("no matching route",
			&v1.Route{
				Matchers: []*matchers.Matcher{
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/foo",
						},
					},
				},
				Action: &v1.Route_DelegateAction{
					DelegateAction: &v1.DelegateAction{
						DelegationType: &v1.DelegateAction_Ref{
							Ref: &core.ResourceRef{
								Name: "foo",
							},
						},
					},
				},
			},
			nil,
			translator.RouteTableMissingWarning(core.ResourceRef{Name: "foo"}),
			translator.NewRouteTableSelector(v1.RouteTableList{
				buildRouteTableWithSimpleAction("rt-1", "ns-1", "/foo/1", nil),
				buildRouteTableWithSimpleAction("rt-2", "ns-1", "/foo/2", map[string]string{"foo": "bar"}),
			}),
		),
	)

	When("valid config", func() {
		It("uses '/' prefix matcher as default if matchers are omitted", func() {
			ref := core.ResourceRef{
				Name:      "any",
				Namespace: "ns",
			}
			route := &v1.Route{
				Matchers: []*matchers.Matcher{{}}, // empty struct in list of size one should default to '/'
				Action: &v1.Route_DelegateAction{
					DelegateAction: &v1.DelegateAction{
						DelegationType: &v1.DelegateAction_Ref{
							Ref: &ref,
						},
					},
				},
			}
			rt := v1.RouteTable{
				Routes: []*v1.Route{{
					Matchers: []*matchers.Matcher{}, // empty list should default to '/'
					Action:   &v1.Route_DirectResponseAction{},
				}},
				Metadata: &core.Metadata{
					Name:      "any",
					Namespace: "ns",
				},
			}

			rpt := reporter.ResourceReports{}
			vs := &v1.VirtualService{
				VirtualHost: &v1.VirtualHost{
					Routes: []*v1.Route{route},
				},
			}

			rv := translator.NewRouteConverter(
				translator.NewRouteTableSelector(v1.RouteTableList{&rt}),
				translator.NewRouteTableIndexer(),
			)
			converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
			Expect(converted[0].Matchers[0]).To(Equal(defaults.DefaultMatcher()))
		})

		It("uses parent resource's namespace as default if namespace is omitted on routeAction with single upstream destination", func() {
			route := &v1.Route{
				Matchers: []*matchers.Matcher{{}}, // empty struct in list of size one should default to '/'
				Action: &v1.Route_RouteAction{
					RouteAction: &gloov1.RouteAction{
						Destination: &gloov1.RouteAction_Single{
							Single: &gloov1.Destination{
								DestinationType: &gloov1.Destination_Upstream{
									Upstream: &core.ResourceRef{
										Name: "my-upstream",
									},
								},
							},
						},
					},
				},
			}

			rpt := reporter.ResourceReports{}
			vs := &v1.VirtualService{
				VirtualHost: &v1.VirtualHost{
					Routes: []*v1.Route{route},
				},
				Metadata: &core.Metadata{
					Namespace: "vs-ns",
				},
			}

			rv := translator.NewRouteConverter(
				translator.NewRouteTableSelector(v1.RouteTableList{}),
				translator.NewRouteTableIndexer(),
			)
			converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
			Expect(converted[0].GetRouteAction().GetSingle().GetUpstream().GetNamespace()).To(Equal("vs-ns"))
		})

		It("uses parent resource's namespace as default if namespace is omitted on routeAction with multi upstream destination", func() {
			route := &v1.Route{
				Matchers: []*matchers.Matcher{{}}, // empty struct in list of size one should default to '/'
				Action: &v1.Route_RouteAction{
					RouteAction: &gloov1.RouteAction{
						Destination: &gloov1.RouteAction_Multi{
							Multi: &gloov1.MultiDestination{
								Destinations: []*gloov1.WeightedDestination{
									{
										Destination: &gloov1.Destination{
											DestinationType: &gloov1.Destination_Upstream{
												Upstream: &core.ResourceRef{
													Name: "my-upstream",
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

			rpt := reporter.ResourceReports{}
			vs := &v1.VirtualService{
				VirtualHost: &v1.VirtualHost{
					Routes: []*v1.Route{route},
				},
				Metadata: &core.Metadata{
					Namespace: "vs-ns",
				},
			}

			rv := translator.NewRouteConverter(
				translator.NewRouteTableSelector(v1.RouteTableList{}),
				translator.NewRouteTableIndexer(),
			)
			converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
			dest0 := converted[0].GetRouteAction().GetMulti().GetDestinations()[0]
			Expect(dest0.GetDestination().GetUpstream().GetNamespace()).To(Equal("vs-ns"))
		})

		It("builds correct route name when the parent route is named", func() {
			ref := core.ResourceRef{
				Name:      "any",
				Namespace: "ns",
			}
			route := &v1.Route{
				Name:     "route1",
				Matchers: []*matchers.Matcher{{}},
				Action: &v1.Route_DelegateAction{
					DelegateAction: &v1.DelegateAction{
						DelegationType: &v1.DelegateAction_Ref{
							Ref: &ref,
						},
					},
				},
			}
			rt := v1.RouteTable{
				Routes: []*v1.Route{{
					Name:     "",
					Matchers: []*matchers.Matcher{},
					Action:   &v1.Route_DirectResponseAction{},
				}, {
					Name:     "redirectAction",
					Matchers: []*matchers.Matcher{},
					Action:   &v1.Route_RedirectAction{},
				}, {
					Name:     "routeAction1",
					Matchers: []*matchers.Matcher{},
					Action:   &v1.Route_RouteAction{},
				}},
				Metadata: &core.Metadata{
					Name:      "any",
					Namespace: "ns",
				},
			}

			rpt := reporter.ResourceReports{}
			vs := &v1.VirtualService{
				Metadata: &core.Metadata{Name: "vs1", Namespace: "vs-ns"},
				VirtualHost: &v1.VirtualHost{
					Routes: []*v1.Route{route},
				},
			}

			rv := translator.NewRouteConverter(
				translator.NewRouteTableSelector(v1.RouteTableList{&rt}),
				translator.NewRouteTableIndexer(),
			)
			converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
			Expect(converted).To(HaveLen(3))
			Expect(converted[0].Name).To(Equal("vs:gw1_proxy1_vs-ns_vs1_route:route1_rt:ns_any_route:<unnamed-0>"))
			Expect(converted[1].Name).To(Equal("vs:gw1_proxy1_vs-ns_vs1_route:route1_rt:ns_any_route:redirectAction"))
			Expect(converted[2].Name).To(Equal("vs:gw1_proxy1_vs-ns_vs1_route:route1_rt:ns_any_route:routeAction1"))
		})

		It("builds correct route name when the parent route is unnamed", func() {
			ref := core.ResourceRef{
				Name:      "any",
				Namespace: "ns",
			}
			route := &v1.Route{
				Matchers: []*matchers.Matcher{{}},
				Action: &v1.Route_DelegateAction{
					DelegateAction: &v1.DelegateAction{
						DelegationType: &v1.DelegateAction_Ref{
							Ref: &ref,
						},
					},
				},
			}
			rt := v1.RouteTable{
				Routes: []*v1.Route{{
					Name:     "directResponseAction",
					Matchers: []*matchers.Matcher{},
					Action:   &v1.Route_DirectResponseAction{},
				}, {
					Name:     "",
					Matchers: []*matchers.Matcher{},
					Action:   &v1.Route_RedirectAction{},
				}, {
					Name:     "routeAction",
					Matchers: []*matchers.Matcher{},
					Action:   &v1.Route_RouteAction{},
				}},
				Metadata: &core.Metadata{
					Name:      "any",
					Namespace: "ns",
				},
			}

			rpt := reporter.ResourceReports{}
			vs := &v1.VirtualService{
				Metadata: &core.Metadata{Name: "vs1", Namespace: "vs-ns"},
				VirtualHost: &v1.VirtualHost{
					Routes: []*v1.Route{route},
				},
			}

			rv := translator.NewRouteConverter(
				translator.NewRouteTableSelector(v1.RouteTableList{&rt}),
				translator.NewRouteTableIndexer(),
			)
			converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
			Expect(converted).To(HaveLen(3))
			Expect(converted[0].Name).To(Equal("vs:gw1_proxy1_vs-ns_vs1_route:<unnamed-0>_rt:ns_any_route:directResponseAction"))
			Expect(converted[1].Name).To(Equal(""))
			Expect(converted[2].Name).To(Equal("vs:gw1_proxy1_vs-ns_vs1_route:<unnamed-0>_rt:ns_any_route:routeAction"))
		})

		Context("inheritance mode", func() {

			var (
				rtOnlyHeaders []*matchers.HeaderMatcher
				vsOnlyHeaders []*matchers.HeaderMatcher
				vs            *v1.VirtualService
				rt            *v1.RouteTable
				rv            translator.RouteConverter
			)

			BeforeEach(func() {
				rtOnlyHeaders = []*matchers.HeaderMatcher{
					{
						Name:        "headername",
						Value:       "headerval",
						Regex:       false,
						InvertMatch: false,
					},
				}
				vsOnlyHeaders = []*matchers.HeaderMatcher{
					{
						Name:        "mismatchedheadername",
						Value:       "mismatchedheaderval",
						Regex:       false,
						InvertMatch: false,
					},
				}

				rt = &v1.RouteTable{
					Metadata: &core.Metadata{
						Name:      "rt",
						Namespace: "default",
					},
					Routes: []*v1.Route{{
						Name: "route-1",
						Action: &v1.Route_DirectResponseAction{
							DirectResponseAction: &gloov1.DirectResponseAction{
								Status: 200,
								Body:   "foo",
							},
						},
					}},
				}

				vs = &v1.VirtualService{
					Metadata: &core.Metadata{
						Name:      "vs",
						Namespace: "default",
					},
					VirtualHost: &v1.VirtualHost{
						Routes: []*v1.Route{
							{
								Matchers: []*matchers.Matcher{{
									Headers: vsOnlyHeaders,
									PathSpecifier: &matchers.Matcher_Prefix{
										Prefix: "/foo",
									},
								}},
								InheritableMatchers: &wrappers.BoolValue{Value: true},
								Action: &v1.Route_DelegateAction{
									DelegateAction: &v1.DelegateAction{
										DelegationType: &v1.DelegateAction_Ref{
											Ref: &core.ResourceRef{
												Name:      "rt",
												Namespace: "default",
											},
										},
									},
								},
							},
						},
					},
				}

				rv = translator.NewRouteConverter(
					translator.NewRouteTableSelector(v1.RouteTableList{rt}),
					translator.NewRouteTableIndexer(),
				)
			})

			It("accepts the route table if its parent has different headers but inheritance is on", func() {

				rt.Routes[0].Matchers = []*matchers.Matcher{
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/foo/bar",
						},
						Headers: rtOnlyHeaders,
					},
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/foo/baz",
						},
						Headers: rtOnlyHeaders,
					},
				}

				expectedHeaders := append(rtOnlyHeaders, vsOnlyHeaders...)

				rpt := reporter.ResourceReports{}
				converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
				Expect(converted).To(HaveLen(1))
				Expect(rpt).To(HaveLen(0))

				Expect(converted[0].Matchers).To(HaveLen(2))
				Expect(converted[0].Matchers[0].Headers).To(ConsistOf(expectedHeaders))
				Expect(converted[0].Matchers[1].Headers).To(ConsistOf(expectedHeaders))

				// zero out headers since we asserted them above
				// ConsistOf doesn't handle the nested objects, so we need to assert the headers for
				// each matcher (above) separate from the matchers
				converted[0].Matchers[0].Headers = nil
				converted[0].Matchers[1].Headers = nil

				Expect(converted[0].Matchers).To(BeEquivalentTo(
					[]*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo/bar"},
							// asserted above
							//Headers:       expectedHeaders,
						},
						{
							PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo/baz"},
							// asserted above
							//Headers:       expectedHeaders,
						},
					},
				))
			})

			It("accepts the route table if its parent has different headers but inheritance is on -- nested route tables", func() {

				rt = buildRouteTableWithDelegateAction("rt", "default", "/foo/bar", nil,
					&v1.DelegateAction{
						DelegationType: &v1.DelegateAction_Ref{
							Ref: &core.ResourceRef{
								Name:      "rt-child",
								Namespace: "default",
							},
						},
					},
				)
				rt.Routes[0].Matchers[0].Headers = rtOnlyHeaders

				rt2 := buildRouteTableWithDelegateAction("rt-child", "default", "/foo/bar/baz", nil,
					&v1.DelegateAction{
						DelegationType: &v1.DelegateAction_Ref{
							Ref: &core.ResourceRef{
								Name:      "rt-grandchild",
								Namespace: "default",
							},
						},
					},
				)
				rt3 := buildRouteTableWithSimpleAction("rt-grandchild", "default", "/foo/bar/baz/quz", nil)

				rv = translator.NewRouteConverter(
					translator.NewRouteTableSelector(v1.RouteTableList{rt, rt2, rt3}),
					translator.NewRouteTableIndexer(),
				)

				expectedHeaders := append(rtOnlyHeaders, vsOnlyHeaders...)

				rpt := reporter.ResourceReports{}
				converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
				Expect(converted).To(HaveLen(1))
				Expect(rpt).To(HaveLen(0))

				Expect(converted[0].Matchers).To(HaveLen(1))
				Expect(converted[0].Matchers[0].Headers).To(ConsistOf(expectedHeaders))

				// zero out headers since we asserted them above
				// ConsistOf doesn't handle the nested objects, so we need to assert the headers for
				// each matcher (above) separate from the matchers
				converted[0].Matchers[0].Headers = nil

				Expect(converted[0].Matchers).To(ConsistOf(
					[]*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo/bar/baz/quz"},
							// asserted above
							// Headers:       expectedHeaders,
						},
					},
				))
			})

			It("inherits route config from parent", func() {

				rt = buildRouteTableWithSimpleAction("rt", "default", "/bar", nil)

				rv = translator.NewRouteConverter(
					translator.NewRouteTableSelector(v1.RouteTableList{rt}),
					translator.NewRouteTableIndexer(),
				)

				// parent has /foo matcher
				Expect(vs.VirtualHost.GetRoutes()).To(HaveLen(1))
				Expect(vs.VirtualHost.GetRoutes()[0].Matchers).To(HaveLen(1))
				Expect(vs.VirtualHost.GetRoutes()[0].Matchers[0].GetPrefix()).To(Equal("/foo"))

				// but child has /bar matcher
				Expect(rt.GetRoutes()).To(HaveLen(1))
				Expect(rt.GetRoutes()[0].Matchers).To(HaveLen(1))
				Expect(rt.GetRoutes()[0].Matchers[0].GetPrefix()).To(Equal("/bar"))

				// with inheritable path matchers, the parent matcher will trump any child config
				vs.VirtualHost.Routes[0].InheritablePathMatchers = &wrappers.BoolValue{Value: true}

				rpt := reporter.ResourceReports{}
				converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
				Expect(converted).To(HaveLen(1))
				Expect(rpt).To(HaveLen(0))

				Expect(converted[0].Matchers).To(HaveLen(1))
				Expect(converted[0].Matchers[0].GetPrefix()).To(Equal("/foo"))
			})

			It("child route inheritance config overrides parent route inheritance config", func() {

				vs.VirtualHost.Routes[0].InheritableMatchers = &wrappers.BoolValue{Value: false}

				rt = buildRouteTableWithSimpleAction("rt", "default", "/bar", nil)

				rv = translator.NewRouteConverter(
					translator.NewRouteTableSelector(v1.RouteTableList{rt}),
					translator.NewRouteTableIndexer(),
				)

				// parent has /foo matcher
				Expect(vs.VirtualHost.GetRoutes()).To(HaveLen(1))
				Expect(vs.VirtualHost.GetRoutes()[0].Matchers).To(HaveLen(1))
				Expect(vs.VirtualHost.GetRoutes()[0].Matchers[0].GetPrefix()).To(Equal("/foo"))

				// but child has /bar matcher
				Expect(rt.GetRoutes()).To(HaveLen(1))
				Expect(rt.GetRoutes()[0].Matchers).To(HaveLen(1))
				Expect(rt.GetRoutes()[0].Matchers[0].GetPrefix()).To(Equal("/bar"))

				// with inheritable matchers, the parent matcher will trump any child config
				rt.Routes[0].InheritablePathMatchers = &wrappers.BoolValue{Value: true}
				rt.Routes[0].InheritableMatchers = &wrappers.BoolValue{Value: true}

				rpt := reporter.ResourceReports{}
				converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
				Expect(converted).To(HaveLen(1))
				Expect(rpt).To(HaveLen(0))

				Expect(converted[0].Matchers).To(HaveLen(1))
				Expect(converted[0].Matchers[0].GetPrefix()).To(Equal("/foo"))
			})

		})

		Context("transformation inheritance mode", func() {

			var (
				routeOnlyTransformation *glootransformation.Transformation
				rtOnlyTransformation    *glootransformation.Transformation
				vsOnlyTransformation    *glootransformation.Transformation
				vs                      *v1.VirtualService
				rt                      *v1.RouteTable
				rv                      translator.RouteConverter
			)

			BeforeEach(func() {
				rtOnlyTransformation = &glootransformation.Transformation{
					TransformationType: &glootransformation.Transformation_TransformationTemplate{
						TransformationTemplate: &glootransformation.TransformationTemplate{
							Headers: map[string]*glootransformation.InjaTemplate{
								"route-table-header": {
									Text: "route table header",
								},
							},
						},
					},
				}
				routeOnlyTransformation = &glootransformation.Transformation{
					TransformationType: &glootransformation.Transformation_TransformationTemplate{
						TransformationTemplate: &glootransformation.TransformationTemplate{
							Headers: map[string]*glootransformation.InjaTemplate{
								"route-header": {
									Text: "route header",
								},
							},
						},
					},
				}
				vsOnlyTransformation = &glootransformation.Transformation{
					TransformationType: &glootransformation.Transformation_TransformationTemplate{
						TransformationTemplate: &glootransformation.TransformationTemplate{
							Headers: map[string]*glootransformation.InjaTemplate{
								"vs-header": {
									Text: "vs header",
								},
							},
						},
					},
				}

				rt = &v1.RouteTable{
					Metadata: &core.Metadata{
						Name:      "rt",
						Namespace: "default",
					},
					Routes: []*v1.Route{{
						Matchers: []*matchers.Matcher{{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/foo/1",
							},
						}},
						Options: &gloov1.RouteOptions{
							StagedTransformations: &glootransformation.TransformationStages{
								InheritTransformation: true,
								Regular: &glootransformation.RequestResponseTransformations{
									RequestTransforms: []*glootransformation.RequestMatch{
										{
											RequestTransformation: routeOnlyTransformation,
										},
									},
								},
							},
						},
						Name: "route-1",
						Action: &v1.Route_DirectResponseAction{
							DirectResponseAction: &gloov1.DirectResponseAction{
								Status: 200,
								Body:   "foo",
							},
						},
					}},
				}

				vs = &v1.VirtualService{
					Metadata: &core.Metadata{
						Name:      "vs",
						Namespace: "default",
					},
					VirtualHost: &v1.VirtualHost{
						Options: &gloov1.VirtualHostOptions{
							StagedTransformations: &glootransformation.TransformationStages{
								Regular: &glootransformation.RequestResponseTransformations{
									RequestTransforms: []*glootransformation.RequestMatch{
										{
											RequestTransformation: vsOnlyTransformation,
										},
									},
								},
							},
						},
						Routes: []*v1.Route{
							{
								Matchers: []*matchers.Matcher{{
									PathSpecifier: &matchers.Matcher_Prefix{
										Prefix: "/foo",
									},
								}},
								Action: &v1.Route_DelegateAction{
									DelegateAction: &v1.DelegateAction{
										DelegationType: &v1.DelegateAction_Ref{
											Ref: &core.ResourceRef{
												Name:      "rt",
												Namespace: "default",
											},
										},
									},
								},
								Options: &gloov1.RouteOptions{
									StagedTransformations: &glootransformation.TransformationStages{
										InheritTransformation: true,
										Regular: &glootransformation.RequestResponseTransformations{
											RequestTransforms: []*glootransformation.RequestMatch{
												{
													RequestTransformation: rtOnlyTransformation,
												},
											},
										},
									},
								},
							},
						},
					},
				}

				rv = translator.NewRouteConverter(
					translator.NewRouteTableSelector(v1.RouteTableList{rt}),
					translator.NewRouteTableIndexer(),
				)

			})

			It("assigns vhost transformation config to route level", func() {
				rpt := reporter.ResourceReports{}
				converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
				Expect(rpt).To(HaveLen(0))
				Expect(converted).To(HaveLen(1))
				transforms := converted[0].GetOptions().GetStagedTransformations().GetRegular().GetRequestTransforms()
				Expect(transforms).To(HaveLen(3))

				By("verify order of transformations, child first")
				Expect(transforms[0]).To(Equal(&glootransformation.RequestMatch{
					RequestTransformation: routeOnlyTransformation,
				}))
				Expect(transforms[1]).To(Equal(&glootransformation.RequestMatch{
					RequestTransformation: rtOnlyTransformation,
				}))
				Expect(transforms[2]).To(Equal(&glootransformation.RequestMatch{
					RequestTransformation: vsOnlyTransformation,
				}))

			})

			It("merges transformations from different stages to child routes", func() {
				vs.GetVirtualHost().GetOptions().GetStagedTransformations().Early =
					vs.GetVirtualHost().GetOptions().GetStagedTransformations().GetRegular()
				vs.GetVirtualHost().GetOptions().GetStagedTransformations().Regular = nil

				rpt := reporter.ResourceReports{}
				converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
				Expect(rpt).To(HaveLen(0))
				Expect(converted).To(HaveLen(1))
				regularStageTransforms := converted[0].GetOptions().GetStagedTransformations().GetRegular().GetRequestTransforms()
				earlyStageTransforms := converted[0].GetOptions().GetStagedTransformations().GetEarly().GetRequestTransforms()
				Expect(regularStageTransforms).To(HaveLen(2))
				Expect(earlyStageTransforms).To(HaveLen(1))

				Expect(regularStageTransforms[0]).To(Equal(&glootransformation.RequestMatch{
					RequestTransformation: routeOnlyTransformation,
				}))
				Expect(regularStageTransforms[1]).To(Equal(&glootransformation.RequestMatch{
					RequestTransformation: rtOnlyTransformation,
				}))
				// has transformation from early stage as well from vhost level
				Expect(earlyStageTransforms[0]).To(Equal(&glootransformation.RequestMatch{
					RequestTransformation: vsOnlyTransformation,
				}))
			})

			It("doesn't merge in parent transformations, if specified", func() {
				vs.GetVirtualHost().GetRoutes()[0].GetOptions().GetStagedTransformations().InheritTransformation = false

				rpt := reporter.ResourceReports{}
				converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
				Expect(rpt).To(HaveLen(0))
				Expect(converted).To(HaveLen(1))
				regularStageTransforms := converted[0].GetOptions().GetStagedTransformations().GetRegular().GetRequestTransforms()
				// Should only have 2 transformations because we didn't merge in parent transformations on the RT
				Expect(regularStageTransforms).To(HaveLen(2))

				By("verify order of transformations, child first")
				Expect(regularStageTransforms[0]).To(Equal(&glootransformation.RequestMatch{
					RequestTransformation: routeOnlyTransformation,
				}))
				Expect(regularStageTransforms[1]).To(Equal(&glootransformation.RequestMatch{
					RequestTransformation: rtOnlyTransformation,
				}))
			})
		})
	})

	When("bad route table config", func() {

		var (
			vs *v1.VirtualService
			rt *v1.RouteTable
			rv translator.RouteConverter
		)

		BeforeEach(func() {
			rt = &v1.RouteTable{
				Metadata: &core.Metadata{
					Name:      "rt",
					Namespace: "default",
				},
				Routes: []*v1.Route{{
					Name: "route-1",
					Action: &v1.Route_DirectResponseAction{
						DirectResponseAction: &gloov1.DirectResponseAction{
							Status: 200,
							Body:   "foo",
						},
					},
				}},
			}

			vs = &v1.VirtualService{
				Metadata: &core.Metadata{
					Name:      "vs",
					Namespace: "default",
				},
				VirtualHost: &v1.VirtualHost{
					Routes: []*v1.Route{
						{
							Matchers: []*matchers.Matcher{{
								PathSpecifier: &matchers.Matcher_Prefix{
									Prefix: "/foo",
								},
							}},
							Action: &v1.Route_DelegateAction{
								DelegateAction: &v1.DelegateAction{
									DelegationType: &v1.DelegateAction_Ref{
										Ref: &core.ResourceRef{
											Name:      "rt",
											Namespace: "default",
										},
									},
								},
							},
						},
					},
				},
			}

			rv = translator.NewRouteConverter(
				translator.NewRouteTableSelector(v1.RouteTableList{rt}),
				translator.NewRouteTableIndexer(),
			)
		})

		When("route table has a matcher that doesn't match the prefix of the parent route", func() {
			It("reports warning on the route table and on the virtual service", func() {
				rt.Routes[0].Matchers = []*matchers.Matcher{
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/foo/bar",
						},
					},
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/invalid",
						},
					},
				}

				rpt := reporter.ResourceReports{}
				converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
				Expect(converted).To(BeNil())
				Expect(rpt).To(HaveLen(2))

				expectedWarning := translator.InvalidRouteTableForDelegatePrefixWarning("/foo", "/invalid").Error()

				_, vsReport := rpt.Find("*v1.VirtualService", vs.Metadata.Ref())
				Expect(vsReport.Errors).NotTo(HaveOccurred())
				Expect(vsReport.Warnings).To(HaveLen(1))
				Expect(vsReport.Warnings[0]).To(ContainSubstring(expectedWarning))

				_, rtReport := rpt.Find("*v1.RouteTable", rt.Metadata.Ref())
				Expect(rtReport.Errors).NotTo(HaveOccurred())
				Expect(rtReport.Warnings).To(HaveLen(1))
				Expect(rtReport.Warnings[0]).To(ContainSubstring(expectedWarning))
			})
		})

		When("route table has a matcher case sensitivity that doesn't match the prefix case sensitivity of the parent route", func() {
			It("reports warning on the route table and on the virtual service", func() {
				rtCaseSensitivity := &wrappers.BoolValue{Value: false}
				rt.Routes[0].Matchers = []*matchers.Matcher{
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/foo/bar",
						},
						CaseSensitive: rtCaseSensitivity,
					},
				}

				rpt := reporter.ResourceReports{}
				converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
				Expect(converted).To(BeNil())
				Expect(rpt).To(HaveLen(2))

				expectedWarning := translator.InvalidRouteTableForDelegateCaseSensitivePathMatchWarning(rtCaseSensitivity, nil).Error()

				_, vsReport := rpt.Find("*v1.VirtualService", vs.Metadata.Ref())
				Expect(vsReport.Errors).NotTo(HaveOccurred())
				Expect(vsReport.Warnings).To(HaveLen(1))
				Expect(vsReport.Warnings[0]).To(ContainSubstring(expectedWarning))

				_, rtReport := rpt.Find("*v1.RouteTable", rt.Metadata.Ref())
				Expect(rtReport.Errors).NotTo(HaveOccurred())
				Expect(rtReport.Warnings).To(HaveLen(1))
				Expect(rtReport.Warnings[0]).To(ContainSubstring(expectedWarning))
			})
		})

		When("route table has headers that don't match the headers of the parent route", func() {

			var (
				headers []*matchers.HeaderMatcher
			)

			BeforeEach(func() {
				headers = []*matchers.HeaderMatcher{
					{
						Name:        "headername",
						Value:       "headerval",
						Regex:       false,
						InvertMatch: false,
					},
				}
			})

			It("accepts the route table if its parent has no headers", func() {
				rt.Routes[0].Matchers = []*matchers.Matcher{
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/foo/bar",
						},
						Headers: headers,
					},
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/foo/baz",
						},
						Headers: headers,
					},
				}

				rpt := reporter.ResourceReports{}
				converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
				Expect(converted).To(HaveLen(1))
				Expect(rpt).To(HaveLen(0))
				Expect(converted[0].Matchers).To(ConsistOf(
					[]*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo/bar"},
							Headers:       headers,
						},
						{
							PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo/baz"},
							Headers:       headers,
						},
					},
				))
			})

			Context("virtual service route has headers", func() {

				BeforeEach(func() {
					vs.VirtualHost.Routes[0].Matchers[0].Headers = headers
				})

				It("reports warning on the route table and on the virtual service if virtual service has headers that aren't on the route table", func() {

					rt.Routes[0].Matchers = []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/foo/bar",
							},
							Headers: headers,
						},
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/foo/baz",
							},
							// This matcher is missing headers that were specified on the parent route, thus should error
						},
					}

					rpt := reporter.ResourceReports{}
					converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
					Expect(converted).To(BeNil())
					Expect(rpt).To(HaveLen(2))

					expectedWarning := translator.InvalidRouteTableForDelegateHeadersWarning(headers, []*matchers.HeaderMatcher{}).Error()

					_, vsReport := rpt.Find("*v1.VirtualService", vs.Metadata.Ref())
					Expect(vsReport.Errors).NotTo(HaveOccurred())
					Expect(vsReport.Warnings).To(HaveLen(1))
					Expect(vsReport.Warnings[0]).To(ContainSubstring(expectedWarning))

					_, rtReport := rpt.Find("*v1.RouteTable", rt.Metadata.Ref())
					Expect(rtReport.Errors).NotTo(HaveOccurred())
					Expect(rtReport.Warnings).To(HaveLen(1))
					Expect(rtReport.Warnings[0]).To(ContainSubstring(expectedWarning))
				})

				It("reports warning on the route table and on the virtual service if virtual service has headers that aren't equal on the route table", func() {

					mismatchedHeader := proto.Clone(headers[0]).(*matchers.HeaderMatcher)
					mismatchedHeader.Value = mismatchedHeader.Value + "invalid"

					rt.Routes[0].Matchers = []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/foo/bar",
							},
							Headers: headers,
						},
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/foo/baz",
							},
							Headers: []*matchers.HeaderMatcher{mismatchedHeader},
						},
					}

					rpt := reporter.ResourceReports{}
					converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
					Expect(converted).To(BeNil())
					Expect(rpt).To(HaveLen(2))

					expectedWarning := translator.InvalidRouteTableForDelegateHeadersWarning(headers, []*matchers.HeaderMatcher{mismatchedHeader}).Error()

					_, vsReport := rpt.Find("*v1.VirtualService", vs.Metadata.Ref())
					Expect(vsReport.Errors).NotTo(HaveOccurred())
					Expect(vsReport.Warnings).To(HaveLen(1))
					Expect(vsReport.Warnings[0]).To(ContainSubstring(expectedWarning))

					_, rtReport := rpt.Find("*v1.RouteTable", rt.Metadata.Ref())
					Expect(rtReport.Errors).NotTo(HaveOccurred())
					Expect(rtReport.Warnings).To(HaveLen(1))
					Expect(rtReport.Warnings[0]).To(ContainSubstring(expectedWarning))
				})

			})

		})

		When("route table has query parameters that don't match the query parameters of the parent route", func() {

			var (
				queryParams []*matchers.QueryParameterMatcher
			)

			BeforeEach(func() {
				queryParams = []*matchers.QueryParameterMatcher{
					{
						Name:  "queryparamname",
						Value: "queryparamval",
						Regex: false,
					},
				}
			})

			It("accepts the route table if its parent has no query params", func() {
				rt.Routes[0].Matchers = []*matchers.Matcher{
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/foo/bar",
						},
						QueryParameters: queryParams,
					},
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/foo/baz",
						},
						QueryParameters: queryParams,
					},
				}

				rpt := reporter.ResourceReports{}
				converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
				Expect(converted).To(HaveLen(1))
				Expect(rpt).To(HaveLen(0))
				Expect(converted[0].Matchers).To(ConsistOf(
					[]*matchers.Matcher{
						{
							PathSpecifier:   &matchers.Matcher_Prefix{Prefix: "/foo/bar"},
							QueryParameters: queryParams,
						},
						{
							PathSpecifier:   &matchers.Matcher_Prefix{Prefix: "/foo/baz"},
							QueryParameters: queryParams,
						},
					},
				))
			})

			Context("virtual service route has query params", func() {

				BeforeEach(func() {
					vs.VirtualHost.Routes[0].Matchers[0].QueryParameters = queryParams
				})

				It("reports warning on the route table and on the virtual service if virtual service has query params that aren't on the route table", func() {

					rt.Routes[0].Matchers = []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/foo/bar",
							},
							QueryParameters: queryParams,
						},
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/foo/baz",
							},
							// This matcher is missing query params that were specified on the parent route, thus should error
						},
					}

					rpt := reporter.ResourceReports{}
					converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
					Expect(converted).To(BeNil())
					Expect(rpt).To(HaveLen(2))

					expectedWarning := translator.InvalidRouteTableForDelegateQueryParamsWarning(queryParams, []*matchers.QueryParameterMatcher{}).Error()

					_, vsReport := rpt.Find("*v1.VirtualService", vs.Metadata.Ref())
					Expect(vsReport.Errors).NotTo(HaveOccurred())
					Expect(vsReport.Warnings).To(HaveLen(1))
					Expect(vsReport.Warnings[0]).To(ContainSubstring(expectedWarning))

					_, rtReport := rpt.Find("*v1.RouteTable", rt.Metadata.Ref())
					Expect(rtReport.Errors).NotTo(HaveOccurred())
					Expect(rtReport.Warnings).To(HaveLen(1))
					Expect(rtReport.Warnings[0]).To(ContainSubstring(expectedWarning))
				})

				It("reports warning on the route table and on the virtual service if virtual service has query params that aren't equal on the route table", func() {

					mismatchedQueryParams := proto.Clone(queryParams[0]).(*matchers.QueryParameterMatcher)
					mismatchedQueryParams.Value = mismatchedQueryParams.Value + "invalid"

					rt.Routes[0].Matchers = []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/foo/bar",
							},
							QueryParameters: queryParams,
						},
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/foo/baz",
							},
							QueryParameters: []*matchers.QueryParameterMatcher{mismatchedQueryParams},
						},
					}

					rpt := reporter.ResourceReports{}
					converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
					Expect(converted).To(BeNil())
					Expect(rpt).To(HaveLen(2))

					expectedWarning := translator.InvalidRouteTableForDelegateQueryParamsWarning(queryParams, []*matchers.QueryParameterMatcher{mismatchedQueryParams}).Error()

					_, vsReport := rpt.Find("*v1.VirtualService", vs.Metadata.Ref())
					Expect(vsReport.Errors).NotTo(HaveOccurred())
					Expect(vsReport.Warnings).To(HaveLen(1))
					Expect(vsReport.Warnings[0]).To(ContainSubstring(expectedWarning))

					_, rtReport := rpt.Find("*v1.RouteTable", rt.Metadata.Ref())
					Expect(rtReport.Errors).NotTo(HaveOccurred())
					Expect(rtReport.Warnings).To(HaveLen(1))
					Expect(rtReport.Warnings[0]).To(ContainSubstring(expectedWarning))
				})
			})

		})

		When("route table has methods that don't match the methods of the parent route", func() {

			var (
				methods []string
			)

			BeforeEach(func() {
				methods = []string{"GET", "POST"}
			})

			It("accepts the route table if its parent has no methods", func() {
				rt.Routes[0].Matchers = []*matchers.Matcher{
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/foo/bar",
						},
						Methods: methods,
					},
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: "/foo/baz",
						},
						Methods: methods,
					},
				}

				rpt := reporter.ResourceReports{}
				converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
				Expect(converted).To(HaveLen(1))
				Expect(rpt).To(HaveLen(0))
				Expect(converted[0].Matchers).To(ConsistOf(
					[]*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo/bar"},
							Methods:       methods,
						},
						{
							PathSpecifier: &matchers.Matcher_Prefix{Prefix: "/foo/baz"},
							Methods:       methods,
						},
					},
				))
			})

			Context("virtual service route has methods", func() {

				BeforeEach(func() {
					vs.VirtualHost.Routes[0].Matchers[0].Methods = methods
				})

				It("reports warning on the route table and on the virtual service if virtual service has methods that aren't on the route table", func() {

					rt.Routes[0].Matchers = []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/foo/bar",
							},
							Methods: methods,
						},
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/foo/baz",
							},
							// This matcher is missing methods that were specified on the parent route, thus should error
						},
					}

					rpt := reporter.ResourceReports{}
					converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
					Expect(converted).To(BeNil())
					Expect(rpt).To(HaveLen(2))

					expectedWarning := translator.InvalidRouteTableForDelegateMethodsWarning(methods, []string{}).Error()

					_, vsReport := rpt.Find("*v1.VirtualService", vs.Metadata.Ref())
					Expect(vsReport.Errors).NotTo(HaveOccurred())
					Expect(vsReport.Warnings).To(HaveLen(1))
					Expect(vsReport.Warnings[0]).To(ContainSubstring(expectedWarning))

					_, rtReport := rpt.Find("*v1.RouteTable", rt.Metadata.Ref())
					Expect(rtReport.Errors).NotTo(HaveOccurred())
					Expect(rtReport.Warnings).To(HaveLen(1))
					Expect(rtReport.Warnings[0]).To(ContainSubstring(expectedWarning))
				})

				It("reports warning on the route table and on the virtual service if virtual service has methods that aren't equal on the route table", func() {

					rt.Routes[0].Matchers = []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/foo/bar",
							},
							Methods: methods,
						},
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/foo/baz",
							},
							// only get the first method, not a superset of parent methods
							Methods: []string{methods[0]},
						},
					}

					rpt := reporter.ResourceReports{}
					converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
					Expect(converted).To(BeNil())
					Expect(rpt).To(HaveLen(2))

					expectedWarning := translator.InvalidRouteTableForDelegateMethodsWarning(methods, []string{methods[0]}).Error()

					_, vsReport := rpt.Find("*v1.VirtualService", vs.Metadata.Ref())
					Expect(vsReport.Errors).NotTo(HaveOccurred())
					Expect(vsReport.Warnings).To(HaveLen(1))
					Expect(vsReport.Warnings[0]).To(ContainSubstring(expectedWarning))

					_, rtReport := rpt.Find("*v1.RouteTable", rt.Metadata.Ref())
					Expect(rtReport.Errors).NotTo(HaveOccurred())
					Expect(rtReport.Warnings).To(HaveLen(1))
					Expect(rtReport.Warnings[0]).To(ContainSubstring(expectedWarning))
				})
			})

		})

		When("route table has no matchers and the parent route matcher is not the default one", func() {
			It("reports warning on the route table and on the virtual service", func() {
				rpt := reporter.ResourceReports{}
				converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
				Expect(converted).To(BeNil())
				Expect(rpt).To(HaveLen(2))

				expectedWarning := translator.InvalidRouteTableForDelegatePrefixWarning("/foo", "/").Error()

				_, vsReport := rpt.Find("*v1.VirtualService", vs.Metadata.Ref())
				Expect(vsReport.Errors).NotTo(HaveOccurred())
				Expect(vsReport.Warnings).To(HaveLen(1))
				Expect(vsReport.Warnings[0]).To(ContainSubstring(expectedWarning))

				_, rtReport := rpt.Find("*v1.RouteTable", rt.Metadata.Ref())
				Expect(rtReport.Errors).NotTo(HaveOccurred())
				Expect(rtReport.Warnings).To(HaveLen(1))
				Expect(rtReport.Warnings[0]).To(ContainSubstring(expectedWarning))
			})
		})

		When("route table has no matchers but the parent route matcher is the default one", func() {
			It("reports no errors", func() {
				vs.VirtualHost.Routes[0].Matchers = []*matchers.Matcher{defaults.DefaultMatcher()}

				rpt := reporter.ResourceReports{}
				converted := rv.ConvertVirtualService(vs, gw, "proxy1", snapshot, rpt)
				Expect(converted).To(HaveLen(1))
				Expect(rpt).To(HaveLen(0))
			})
		})
	})

	Describe("route table selection", func() {

		var (
			allRouteTables v1.RouteTableList
			reports        reporter.ResourceReports
			vs             *v1.VirtualService
			visitor        translator.RouteConverter
		)

		buildVirtualService := func(rtSelector *v1.RouteTableSelector) *v1.VirtualService {
			return &v1.VirtualService{
				Metadata: &core.Metadata{
					Name:      "vs-1",
					Namespace: "ns-1",
				},
				VirtualHost: &v1.VirtualHost{
					Routes: []*v1.Route{
						{
							Matchers: []*matchers.Matcher{{
								PathSpecifier: &matchers.Matcher_Prefix{
									Prefix: "/foo",
								},
							}},
							Action: &v1.Route_DelegateAction{
								DelegateAction: &v1.DelegateAction{
									DelegationType: &v1.DelegateAction_Selector{
										Selector: rtSelector,
									},
								},
							},
						},
					},
				},
			}
		}

		buildVirtualServiceWithName := func(rtSelector *v1.RouteTableSelector, routeName string) *v1.VirtualService {
			vs := buildVirtualService(rtSelector)
			vs.VirtualHost.Routes[0].Name = routeName
			return vs
		}

		JustBeforeEach(func() {
			reports = reporter.ResourceReports{}
			visitor = translator.NewRouteConverter(
				translator.NewRouteTableSelector(allRouteTables),
				translator.NewRouteTableIndexer(),
			)
		})

		Describe("merged route ordering", func() {

			BeforeEach(func() {
				allRouteTables = v1.RouteTableList{
					buildRouteTableWithSimpleAction("rt-1", "ns-1", "/foo", nil),
					buildRouteTableWithSimpleAction("rt-2", "ns-1", "/foo/bars", nil),
					buildRouteTableWithSimpleAction("rt-3", "ns-1", "/foo/bar", nil),
					buildRouteTableWithSimpleAction("rt-4", "ns-1", "/foo/bar/baz", nil),
				}
			})

			It("merged routes are sorted by descending specificity", func() {
				vs = buildVirtualService(&v1.RouteTableSelector{
					Namespaces: []string{"ns-1"},
				})

				converted := visitor.ConvertVirtualService(vs, gw, "proxy1", snapshot, reports)
				Expect(converted).To(HaveLen(4))
				Expect(converted[0]).To(WithTransform(getFirstPrefixMatcher, Equal("/foo/bars")))
				Expect(converted[1]).To(WithTransform(getFirstPrefixMatcher, Equal("/foo/bar/baz")))
				Expect(converted[2]).To(WithTransform(getFirstPrefixMatcher, Equal("/foo/bar")))
				Expect(converted[3]).To(WithTransform(getFirstPrefixMatcher, Equal("/foo")))

				Expect(reports).NotTo(BeNil())
				_, vsReport := reports.Find("*v1.VirtualService", vs.Metadata.Ref())
				Expect(vsReport).NotTo(BeNil())
				Expect(vsReport.Errors).To(BeNil())
				Expect(vsReport.Warnings).To(BeNil())
			})
		})

		When("configuration is correct", func() {

			BeforeEach(func() {
				allRouteTables = v1.RouteTableList{
					buildRouteTableWithSimpleAction("rt-1", "ns-1", "/foo/1", nil),
					buildRouteTableWithSimpleAction("rt-2", "ns-1", "/foo/2", map[string]string{"foo": "bar", "team": "dev"}),
					buildRouteTableWithSimpleAction("rt-3", "ns-2", "/foo/3", map[string]string{"foo": "bar"}),
					buildRouteTableWithSimpleAction("rt-4", "ns-3", "/foo/4", map[string]string{"foo": "baz"}),
					buildRouteTableWithSelector("rt-5", "ns-4", "/foo", nil,
						&v1.RouteTableSelector{
							Labels:     map[string]string{"team": "dev"},
							Namespaces: []string{"ns-1", "ns-5"},
						}),
					buildRouteTableWithSimpleAction("rt-6", "ns-5", "/foo/6", map[string]string{"team": "dev"}),
				}
			})

			DescribeTable("selector works as expected",
				func(selector *v1.RouteTableSelector, expectedPrefixMatchers []string) {
					vs = buildVirtualService(selector)
					converted := visitor.ConvertVirtualService(vs, gw, "proxy1", snapshot, reports)
					Expect(converted).To(HaveLen(len(expectedPrefixMatchers)))
					for i, prefix := range expectedPrefixMatchers {
						Expect(converted[i]).To(WithTransform(getFirstPrefixMatcher, Equal(prefix)))
					}
				},

				Entry("when no labels nor namespaces are provided",
					&v1.RouteTableSelector{},
					[]string{"/foo/2", "/foo/1"},
				),

				Entry("when a label is specified in the selector (but no namespace)",
					&v1.RouteTableSelector{
						Labels: map[string]string{"foo": "bar"},
					},
					[]string{"/foo/2"},
				),

				Entry("when namespaces are specified in the selector (but no labels)",
					&v1.RouteTableSelector{
						Namespaces: []string{"ns-1", "ns-2"},
					},
					[]string{"/foo/3", "/foo/2", "/foo/1"},
				),

				Entry("when both namespaces and labels are specified in the selector",
					&v1.RouteTableSelector{
						Labels:     map[string]string{"foo": "bar"},
						Namespaces: []string{"ns-2"},
					},
					[]string{"/foo/3"},
				),

				Entry("when we have multiple levels of delegation",
					&v1.RouteTableSelector{
						Namespaces: []string{"ns-4"},
					},
					[]string{"/foo/6", "/foo/2"},
				),

				// This also covers the case where a route table is selected by multiple route tables.
				// rt-1 and rt-6 are selected both directly by the below selector and indirectly via rt-5.
				Entry("when selector contains 'all namespaces' wildcard selector (*)",
					&v1.RouteTableSelector{
						Namespaces: []string{"ns-1", "*"},
					},
					[]string{"/foo/6", "/foo/6", "/foo/4", "/foo/3", "/foo/2", "/foo/2", "/foo/1"},
				),
			)

			DescribeTable("route naming works as expected",
				func(selector *v1.RouteTableSelector, routeName string, expectedNames []string) {

					vs = buildVirtualServiceWithName(selector, routeName)
					converted := visitor.ConvertVirtualService(vs, gw, "proxy1", snapshot, reports)
					Expect(converted).To(HaveLen(len(expectedNames)))
					for i, name := range expectedNames {
						Expect(converted[i].Name).To(Equal(name))
					}
				},

				Entry("when one delegate action matches multiple route tables",
					&v1.RouteTableSelector{},
					"testRouteName",
					[]string{"vs:gw1_proxy1_ns-1_vs-1_route:testRouteName_rt:ns-1_rt-2_route:simpleRouteName",
						"vs:gw1_proxy1_ns-1_vs-1_route:testRouteName_rt:ns-1_rt-1_route:simpleRouteName"},
				),

				Entry("when we have multiple levels of delegation",
					&v1.RouteTableSelector{
						Namespaces: []string{"ns-4"},
					},
					"topLevelRoute",
					[]string{"vs:gw1_proxy1_ns-1_vs-1_route:topLevelRoute_rt:ns-4_rt-5_route:<unnamed-0>_rt:ns-5_rt-6_route:simpleRouteName",
						"vs:gw1_proxy1_ns-1_vs-1_route:topLevelRoute_rt:ns-4_rt-5_route:<unnamed-0>_rt:ns-1_rt-2_route:simpleRouteName"},
				),

				// rt-1 and rt-6 are selected both directly by the below selector and indirectly via rt-5.
				Entry("when a route table is selected by multiple route tables",
					&v1.RouteTableSelector{
						Namespaces: []string{"ns-1", "*"},
					},
					"topLevelRoute",
					[]string{"vs:gw1_proxy1_ns-1_vs-1_route:topLevelRoute_rt:ns-4_rt-5_route:<unnamed-0>_rt:ns-5_rt-6_route:simpleRouteName",
						"vs:gw1_proxy1_ns-1_vs-1_route:topLevelRoute_rt:ns-5_rt-6_route:simpleRouteName",
						"vs:gw1_proxy1_ns-1_vs-1_route:topLevelRoute_rt:ns-3_rt-4_route:simpleRouteName",
						"vs:gw1_proxy1_ns-1_vs-1_route:topLevelRoute_rt:ns-2_rt-3_route:simpleRouteName",
						"vs:gw1_proxy1_ns-1_vs-1_route:topLevelRoute_rt:ns-1_rt-2_route:simpleRouteName",
						"vs:gw1_proxy1_ns-1_vs-1_route:topLevelRoute_rt:ns-4_rt-5_route:<unnamed-0>_rt:ns-1_rt-2_route:simpleRouteName",
						"vs:gw1_proxy1_ns-1_vs-1_route:topLevelRoute_rt:ns-1_rt-1_route:simpleRouteName"},
				),
			)

		})

		When("there are circular references", func() {

			Context("using a route table selector", func() {
				BeforeEach(func() {
					allRouteTables = v1.RouteTableList{
						buildRouteTableWithSelector("rt-0", "self", "/foo", nil,
							&v1.RouteTableSelector{
								Namespaces: []string{"self"},
							}),

						buildRouteTableWithSelector("rt-1", "ns-1", "/foo", nil,
							&v1.RouteTableSelector{
								Namespaces: []string{"*"},
								Labels:     map[string]string{"foo": "bar"},
							}),
						buildRouteTableWithSelector("rt-2", "ns-2", "/foo/1", map[string]string{"foo": "bar"},
							&v1.RouteTableSelector{
								Namespaces: []string{"ns-3"},
							}),
						// This one points back to rt-1
						buildRouteTableWithSelector("rt-3", "ns-3", "/foo/1/2", nil,
							&v1.RouteTableSelector{
								Namespaces: []string{"ns-1"},
							}),
					}
				})

				DescribeTable("delegation cycles are detected",
					func(selector *v1.RouteTableSelector, expectedCycleInfoMessage string, offendingTable core.Metadata) {
						vs = buildVirtualService(selector)
						visitor.ConvertVirtualService(vs, gw, "proxy1", snapshot, reports)
						expectedErrStr := translator.DelegationCycleErr(expectedCycleInfoMessage).Error()

						// Verify that error is reported on Route Table and VS
						Expect(reports).To(HaveLen(2))
						_, rtReport := reports.Find("*v1.RouteTable", offendingTable.Ref())
						Expect(rtReport.Errors).To(HaveOccurred())
						Expect(rtReport.Errors).To(MatchError(ContainSubstring(expectedErrStr)))
						_, vsReport := reports.Find("*v1.VirtualService", vs.Metadata.Ref())
						Expect(vsReport.Errors).To(HaveOccurred())
						Expect(vsReport.Errors).To(MatchError(ContainSubstring(expectedErrStr)))
					},

					Entry("a route table selects itself",
						&v1.RouteTableSelector{
							Namespaces: []string{"self"},
						},
						"[self.rt-0] -> [self.rt-0]",
						core.Metadata{Name: "rt-0", Namespace: "self"},
					),

					Entry("multi route table cycle scenario",
						&v1.RouteTableSelector{
							Namespaces: []string{"ns-1"},
						},
						"[ns-1.rt-1] -> [ns-2.rt-2] -> [ns-3.rt-3] -> [ns-1.rt-1]",
						core.Metadata{Name: "rt-1", Namespace: "ns-1"},
					),
				)
			})

			Context("using a hard reference", func() {

				BeforeEach(func() {
					allRouteTables = v1.RouteTableList{
						buildRouteTableWithDelegateAction("rt-x", "x", "/foo/bar", nil,
							&v1.DelegateAction{
								DelegationType: &v1.DelegateAction_Ref{
									Ref: &core.ResourceRef{
										Name:      "rt-x",
										Namespace: "x",
									},
								},
							}),
						buildRouteTableWithDelegateAction("rt-y", "y", "/foo/baz", nil,
							&v1.DelegateAction{
								DelegationType: &v1.DelegateAction_Ref{
									Ref: &core.ResourceRef{
										Name:      "rt-y",
										Namespace: "y",
									},
								},
							}),
					}
				})

				DescribeTable("delegation cycles are detected",
					func(selector *v1.RouteTableSelector, expectedCycleInfoMessage string, offendingTable core.Metadata) {
						vs = buildVirtualService(selector)
						visitor.ConvertVirtualService(vs, gw, "proxy1", snapshot, reports)
						expectedErrStr := translator.DelegationCycleErr(expectedCycleInfoMessage).Error()

						// Verify that error is reported on Route Table and VS
						_, rtReport := reports.Find("*v1.RouteTable", offendingTable.Ref())
						Expect(rtReport.Errors).To(HaveOccurred())
						Expect(rtReport.Errors).To(MatchError(ContainSubstring(expectedErrStr)))
						_, vsReport := reports.Find("*v1.VirtualService", vs.Metadata.Ref())
						Expect(vsReport.Errors).To(HaveOccurred())
						Expect(vsReport.Errors).To(MatchError(ContainSubstring(expectedErrStr)))
					},

					Entry("using the new ref format",
						&v1.RouteTableSelector{
							Namespaces: []string{"x"},
						},
						"[x.rt-x] -> [x.rt-x]",
						core.Metadata{Name: "rt-x", Namespace: "x"},
					),

					Entry("using the deprecated ref format",
						&v1.RouteTableSelector{
							Namespaces: []string{"y"},
						},
						"[y.rt-y] -> [y.rt-y]",
						core.Metadata{Name: "rt-y", Namespace: "y"},
					),
				)
			})
		})

		Describe("route tables with weights", func() {

			var rt1, rt2, rt3, rt1a, rt1b, rt3a, rt3b, rt3c *v1.RouteTable

			BeforeEach(func() {

				// Matches rt1, rt2, rt3
				vs = buildVirtualService(&v1.RouteTableSelector{
					Namespaces: []string{"ns-1"},
				})

				// Matches rt1a, rt1b
				rt1 = buildRouteTableWithSelector("rt-1", "ns-1", "/foo/a", nil,
					&v1.RouteTableSelector{
						Namespaces: []string{"ns-2"},
					},
				)
				rt1.Weight = &wrappers.Int32Value{Value: 20}

				// Same weight as rt1
				rt2 = buildRouteTableWithSimpleAction("rt-2", "ns-1", "/foo/b", nil)
				rt2.Weight = &wrappers.Int32Value{Value: 20}

				// Matches rt3a, rt3b
				rt3 = buildRouteTableWithSelector("rt-3", "ns-1", "/foo/c", nil,
					&v1.RouteTableSelector{
						Namespaces: []string{"ns-3"},
					},
				)
				rt3.Weight = &wrappers.Int32Value{Value: -10}

				// No weight
				rt1a = buildRouteTableWithSimpleAction("rt-1-a", "ns-2", "/foo/a/1", nil)
				// No weight
				rt1b = buildRouteTableWithSimpleAction("rt-1-b", "ns-2", "/foo/a/1/2", nil)

				rt3a = buildRouteTableWithSimpleAction("rt-3-a", "ns-3", "/foo/c/1", nil)
				rt3a.Weight = &wrappers.Int32Value{Value: -20}

				// The following RTs have the same weight. We want to verify that only the routes from rt3b and rt3c
				// get re-sorted, but that we respect the -10 weight on rt3a.
				rt3b = buildRouteTableWithSimpleAction("rt-3-b", "ns-3", "/foo/c/1/short-circuited", nil)
				rt3b.Weight = &wrappers.Int32Value{Value: 0}
				rt3c = buildRouteTableWithSimpleAction("rt-3-c", "ns-3", "/foo/c/2", nil)
				rt3c.Weight = &wrappers.Int32Value{Value: 0}

				allRouteTables = v1.RouteTableList{rt1, rt2, rt3, rt1a, rt1b, rt3a, rt3b, rt3c}
			})

			It("works as expected", func() {

				converted := visitor.ConvertVirtualService(vs, gw, "proxy1", snapshot, reports)
				Expect(converted).To(HaveLen(6))

				Expect(converted[0]).To(WithTransform(getFirstPrefixMatcher, Equal("/foo/c/1")))
				Expect(converted[1]).To(WithTransform(getFirstPrefixMatcher, Equal("/foo/c/2")))
				Expect(converted[2]).To(WithTransform(getFirstPrefixMatcher, Equal("/foo/c/1/short-circuited")))
				Expect(converted[3]).To(WithTransform(getFirstPrefixMatcher, Equal("/foo/b")))
				Expect(converted[4]).To(WithTransform(getFirstPrefixMatcher, Equal("/foo/a/1/2")))
				Expect(converted[5]).To(WithTransform(getFirstPrefixMatcher, Equal("/foo/a/1")))

				By("virtual service contains all warnings about child route tables with the same weight", func() {
					_, vsReport := reports.Find("*v1.VirtualService", vs.Metadata.Ref())
					Expect(vsReport).NotTo(BeNil())
					Expect(vsReport.Warnings).To(BeNil())
					Expect(vsReport.Errors).To(BeNil())
				})
			})
		})
	})
})

func getFirstPrefixMatcher(route *gloov1.Route) string {
	return route.GetMatchers()[0].GetPrefix()
}

func buildRouteTableWithSimpleAction(name, namespace, prefix string, labels map[string]string) *v1.RouteTable {
	return &v1.RouteTable{
		Metadata: &core.Metadata{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Routes: []*v1.Route{
			{
				Name: "simpleRouteName",
				Matchers: []*matchers.Matcher{
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: prefix,
						},
					},
				},
				Action: &v1.Route_DirectResponseAction{
					DirectResponseAction: &gloov1.DirectResponseAction{Status: 200}},
			},
		},
	}
}

func buildRouteTableWithSelector(name, namespace, prefix string, labels map[string]string, selector *v1.RouteTableSelector) *v1.RouteTable {
	return buildRouteTableWithDelegateAction(name, namespace, prefix, labels, &v1.DelegateAction{
		DelegationType: &v1.DelegateAction_Selector{
			Selector: selector,
		},
	})
}

func buildRouteTableWithDelegateAction(name, namespace, prefix string, labels map[string]string, action *v1.DelegateAction) *v1.RouteTable {
	return &v1.RouteTable{
		Metadata: &core.Metadata{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Routes: []*v1.Route{
			{
				Matchers: []*matchers.Matcher{
					{
						PathSpecifier: &matchers.Matcher_Prefix{
							Prefix: prefix,
						},
					},
				},
				Action: &v1.Route_DelegateAction{
					DelegateAction: action,
				},
			},
		},
	}
}
