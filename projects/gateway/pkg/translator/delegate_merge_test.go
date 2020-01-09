package translator

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

var _ = Describe("route merge util", func() {
	Context("bad config on a delegate route", func() {
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
					expectedErr: missingPrefixErr,
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
					expectedErr: missingPrefixErr,
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
					expectedErr: hasHeaderMatcherErr,
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
					expectedErr: hasMethodMatcherErr,
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
					expectedErr: hasQueryMatcherErr,
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
					expectedErr: matcherCountErr,
				},
			} {
				rv := &routeVisitor{}
				_, err := rv.convertDelegateAction(&v1.VirtualService{}, badRoute.route, reporter.ResourceReports{})
				Expect(err).To(Equal(badRoute.expectedErr))
			}
		})
	})
	Context("missing ref on a delegate route", func() {
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
			rv := &routeVisitor{}
			converted, err := rv.convertDelegateAction(vs, route, rpt)
			Expect(err).NotTo(HaveOccurred())
			Expect(converted).To(HaveLen(0)) // nothing to return, no error

			// missing ref should result in a warning
			Expect(rpt).To(Equal(reporter.ResourceReports{vs: reporter.Report{
				Warnings: []string{routeTableMissingWarning(ref)},
			}}))
		})
	})

	Context("valid config", func() {
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
				}},
				Metadata: core.Metadata{
					Name: "any",
				},
			}

			rpt := reporter.ResourceReports{}
			vs := &v1.VirtualService{}
			rv := &routeVisitor{tables: v1.RouteTableList{&rt}}
			converted, err := rv.convertDelegateAction(vs, route, rpt)
			Expect(err).NotTo(HaveOccurred())
			Expect(converted[0].Matchers[0]).To(Equal(defaults.DefaultMatcher()))
		})
	})
	Context("bad route table config", func() {
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
			rv := &routeVisitor{tables: v1.RouteTableList{&rt}}
			converted, err := rv.convertDelegateAction(vs, route, rpt)
			Expect(err).NotTo(HaveOccurred())
			expectedErr := invalidRouteTableForDelegateErr("/foo", "/invalid").Error()
			Expect(rpt.Validate().Error()).To(ContainSubstring(expectedErr))
			Expect(converted).To(BeNil())
		})
	})
})
