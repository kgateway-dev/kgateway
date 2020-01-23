package translator_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var _ = Describe("route merge util", func() {

	When("bad config on a delegate route", func() {
		It("returns the correct error", func() {
			type routeErr struct {
				route       *v1.Route
				expectedErr error
			}
			for _, badRoute := range []routeErr{
				{
					route: &v1.Route{
						Matchers: []*matchers.Matcher{{
							PathSpecifier: &matchers.Matcher_Regex{
								Regex: "/any",
							},
						}},
						Action: &v1.Route_DelegateAction{
							DelegateAction: &v1.DelegateAction{
								DelegationType: &v1.DelegateAction_Ref{
									Ref: &core.ResourceRef{
										Name: "any",
									},
								},
							},
						},
					},
					expectedErr: translator.MissingPrefixErr,
				},
				{
					route: &v1.Route{
						Matchers: []*matchers.Matcher{{
							PathSpecifier: &matchers.Matcher_Exact{
								Exact: "/any",
							},
						}},
						Action: &v1.Route_DelegateAction{
							DelegateAction: &v1.DelegateAction{
								DelegationType: &v1.DelegateAction_Ref{
									Ref: &core.ResourceRef{
										Name: "any",
									},
								},
							},
						},
					},
					expectedErr: translator.MissingPrefixErr,
				},
				{
					route: &v1.Route{
						Matchers: []*matchers.Matcher{{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/any",
							},
							Headers: []*matchers.HeaderMatcher{{}},
						}},
						Action: &v1.Route_DelegateAction{
							DelegateAction: &v1.DelegateAction{
								DelegationType: &v1.DelegateAction_Ref{
									Ref: &core.ResourceRef{
										Name: "any",
									},
								},
							},
						},
					},
					expectedErr: translator.HasHeaderMatcherErr,
				},
				{
					route: &v1.Route{
						Matchers: []*matchers.Matcher{{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/any",
							},
							Methods: []string{"any"},
						}},
						Action: &v1.Route_DelegateAction{
							DelegateAction: &v1.DelegateAction{
								DelegationType: &v1.DelegateAction_Ref{
									Ref: &core.ResourceRef{
										Name: "any",
									},
								},
							},
						},
					},
					expectedErr: translator.HasMethodMatcherErr,
				},
				{
					route: &v1.Route{
						Matchers: []*matchers.Matcher{{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/any",
							},
							QueryParameters: []*matchers.QueryParameterMatcher{{}},
						}},
						Action: &v1.Route_DelegateAction{
							DelegateAction: &v1.DelegateAction{
								DelegationType: &v1.DelegateAction_Ref{
									Ref: &core.ResourceRef{
										Name: "any",
									},
								},
							},
						},
					},
					expectedErr: translator.HasQueryMatcherErr,
				},
				{
					route: &v1.Route{
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
					expectedErr: translator.MatcherCountErr,
				},
			} {
				rv := translator.NewRouteConverter(&v1.VirtualService{}, nil, reporter.ResourceReports{})
				_, err := rv.ConvertRoute(badRoute.route)
				Expect(err).To(Equal(badRoute.expectedErr))
			}
		})
	})

	When("missing ref on a delegate route", func() {
		It("returns nil and adds a warning", func() {
			ref := core.ResourceRef{
				Name: "any",
			}
			route := &v1.Route{
				Matchers: []*matchers.Matcher{{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/any",
					},
				}},
				Action: &v1.Route_DelegateAction{
					DelegateAction: &v1.DelegateAction{
						DelegationType: &v1.DelegateAction_Ref{
							Ref: &ref,
						},
					},
				},
			}

			rpt := reporter.ResourceReports{}
			vs := &v1.VirtualService{}

			converted, err := translator.NewRouteConverter(vs, nil, rpt).ConvertRoute(route)

			Expect(err).NotTo(HaveOccurred())
			Expect(converted).To(HaveLen(0)) // nothing to return, no error

			// missing ref should result in a warning
			Expect(rpt).To(Equal(reporter.ResourceReports{vs: reporter.Report{
				Warnings: []string{translator.RouteTableMissingWarning(ref)},
			}}))
		})
	})

	When("valid config", func() {
		It("uses '/' prefix matcher as default if matchers are omitted", func() {
			ref := core.ResourceRef{
				Name: "any",
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
				Metadata: core.Metadata{
					Name: "any",
				},
			}

			rpt := reporter.ResourceReports{}
			vs := &v1.VirtualService{}

			converted, err := translator.NewRouteConverter(vs, v1.RouteTableList{&rt}, rpt).ConvertRoute(route)

			Expect(err).NotTo(HaveOccurred())
			Expect(converted[0].Matchers[0]).To(Equal(defaults.DefaultMatcher()))
		})

		It("builds corrects route name", func() {
			ref := core.ResourceRef{
				Name: "any",
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
				}},
				Metadata: core.Metadata{
					Name: "any",
				},
			}

			rpt := reporter.ResourceReports{}
			vs := &v1.VirtualService{Metadata: core.Metadata{Name: "vs1"}}

			converted, err := translator.NewRouteConverter(vs, v1.RouteTableList{&rt}, rpt).ConvertRoute(route)

			Expect(err).NotTo(HaveOccurred())
			Expect(converted[0].Name).To(Equal("vs:vs1_route:route1_rtb:any_route:N/A"))
		})
	})

	When("bad route table config", func() {
		It("returns error if route table has a matcher that doesn't have the delegate prefix", func() {
			ref := core.ResourceRef{
				Name: "rt",
			}
			route := &v1.Route{
				Matchers: []*matchers.Matcher{{
					PathSpecifier: &matchers.Matcher_Prefix{
						Prefix: "/foo",
					},
				}},
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
					Matchers: []*matchers.Matcher{
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/foo/bar",
							},
						},
						{
							PathSpecifier: &matchers.Matcher_Prefix{
								Prefix: "/invalid",
							},
						}},
				}},
				Metadata: core.Metadata{
					Name: "rt",
				},
			}

			rpt := reporter.ResourceReports{}
			vs := &v1.VirtualService{}

			converted, err := translator.NewRouteConverter(vs, v1.RouteTableList{&rt}, rpt).ConvertRoute(route)

			Expect(err).NotTo(HaveOccurred())
			expectedErr := translator.InvalidRouteTableForDelegateErr("/foo", "/invalid").Error()
			Expect(rpt.Validate().Error()).To(ContainSubstring(expectedErr))
			Expect(converted).To(BeNil())
		})
	})

	Describe("route table selection", func() {

		var (
			allRouteTables v1.RouteTableList
			reports        reporter.ResourceReports
			vs             *v1.VirtualService
			visitor        translator.RouteConverter
		)

		buildRouteTableWithSimpleAction := func(name, namespace, prefix string, labels map[string]string) *v1.RouteTable {
			return &v1.RouteTable{
				Metadata: core.Metadata{
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
						Action: &v1.Route_DirectResponseAction{
							DirectResponseAction: &gloov1.DirectResponseAction{Status: 200}},
					},
				},
			}
		}

		buildRouteTableWithDelegateAction := func(name, namespace, prefix string, labels map[string]string, selector *v1.RouteTableSelector) *v1.RouteTable {
			return &v1.RouteTable{
				Metadata: core.Metadata{
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
							DelegateAction: &v1.DelegateAction{
								DelegationType: &v1.DelegateAction_Selector{
									Selector: selector,
								},
							},
						},
					},
				},
			}
		}

		buildRoute := func(rtSelector *v1.RouteTableSelector) *v1.Route {
			return &v1.Route{
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
			}
		}

		getFirstPrefixMatcher := func(route *gloov1.Route) string {
			return route.GetMatchers()[0].GetPrefix()
		}

		JustBeforeEach(func() {
			reports = reporter.ResourceReports{}
			vs = &v1.VirtualService{
				Metadata: core.Metadata{
					Name:      "vs-1",
					Namespace: "ns-1",
				},
			}

			visitor = translator.NewRouteConverter(vs, allRouteTables, reports)
		})

		When("selector has no matches", func() {

			BeforeEach(func() {
				allRouteTables = v1.RouteTableList{
					buildRouteTableWithSimpleAction("rt-1", "ns-1", "/foo/1", nil),
					buildRouteTableWithSimpleAction("rt-2", "ns-1", "/foo/2", map[string]string{"foo": "bar"}),
				}
			})

			It("returns the appropriate warning", func() {
				converted, err := visitor.ConvertRoute(buildRoute(&v1.RouteTableSelector{
					Labels:     map[string]string{"team": "dev", "foo": "bar"},
					Namespaces: []string{"*"},
				}))

				Expect(err).NotTo(HaveOccurred())
				Expect(converted).To(HaveLen(0))
				Expect(reports).To(Equal(reporter.ResourceReports{vs: reporter.Report{
					Warnings: []string{translator.NoMatchingRouteTablesWarning},
				}}))
			})
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
				converted, err := visitor.ConvertRoute(buildRoute(&v1.RouteTableSelector{
					Namespaces: []string{"ns-1"},
				}))

				Expect(err).NotTo(HaveOccurred())
				Expect(converted).To(HaveLen(4))
				Expect(converted[0]).To(WithTransform(getFirstPrefixMatcher, Equal("/foo/bars")))
				Expect(converted[1]).To(WithTransform(getFirstPrefixMatcher, Equal("/foo/bar/baz")))
				Expect(converted[2]).To(WithTransform(getFirstPrefixMatcher, Equal("/foo/bar")))
				Expect(converted[3]).To(WithTransform(getFirstPrefixMatcher, Equal("/foo")))
			})
		})

		When("configuration is correct", func() {

			BeforeEach(func() {
				allRouteTables = v1.RouteTableList{
					buildRouteTableWithSimpleAction("rt-1", "ns-1", "/foo/1", nil),
					buildRouteTableWithSimpleAction("rt-2", "ns-1", "/foo/2", map[string]string{"foo": "bar", "team": "dev"}),
					buildRouteTableWithSimpleAction("rt-3", "ns-2", "/foo/3", map[string]string{"foo": "bar"}),
					buildRouteTableWithSimpleAction("rt-4", "ns-3", "/foo/4", map[string]string{"foo": "baz"}),
					buildRouteTableWithDelegateAction("rt-5", "ns-4", "/foo", nil,
						&v1.RouteTableSelector{
							Labels:     map[string]string{"team": "dev"},
							Namespaces: []string{"ns-1", "ns-5"},
						}),
					buildRouteTableWithSimpleAction("rt-6", "ns-5", "/foo/6", map[string]string{"team": "dev"}),
				}
			})

			DescribeTable("selector works as expected",
				func(selector *v1.RouteTableSelector, expectedPrefixMatchers []string) {
					converted, err := visitor.ConvertRoute(buildRoute(selector))
					Expect(err).NotTo(HaveOccurred())
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
		})

		When("there are circular references", func() {

			BeforeEach(func() {
				allRouteTables = v1.RouteTableList{
					buildRouteTableWithDelegateAction("rt-0", "self", "/foo", nil,
						&v1.RouteTableSelector{
							Namespaces: []string{"self"},
						}),

					buildRouteTableWithDelegateAction("rt-1", "ns-1", "/foo", nil,
						&v1.RouteTableSelector{
							Namespaces: []string{"*"},
							Labels:     map[string]string{"foo": "bar"},
						}),
					buildRouteTableWithDelegateAction("rt-2", "ns-2", "/foo/1", map[string]string{"foo": "bar"},
						&v1.RouteTableSelector{
							Namespaces: []string{"ns-3"},
						}),
					// This one points back to rt-1
					buildRouteTableWithDelegateAction("rt-3", "ns-3", "/foo/1/2", nil,
						&v1.RouteTableSelector{
							Namespaces: []string{"ns-1"},
						}),
				}
			})

			DescribeTable("delegation cycles are detected",
				func(selector *v1.RouteTableSelector, expectedCycleInfoMessage string) {
					_, err := visitor.ConvertRoute(buildRoute(selector))
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(translator.DelegationCycleErr(expectedCycleInfoMessage)))
				},

				Entry("a route table selects itself",
					&v1.RouteTableSelector{
						Namespaces: []string{"self"},
					},
					"[self.rt-0] -> [self.rt-0]",
				),

				Entry("multi route table cycle scenario",
					&v1.RouteTableSelector{
						Namespaces: []string{"ns-1"},
					},
					"[ns-1.rt-1] -> [ns-2.rt-2] -> [ns-3.rt-3] -> [ns-1.rt-1]",
				),
			)
		})
	})
})
