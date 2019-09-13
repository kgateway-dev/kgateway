package translator

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/reporter"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("route merge util", func() {

	Context("delegating with path prefix /", func() {
		It("prepends nothing", func() {
			routeTables := v1.RouteTableList{
				{
					Metadata: core.Metadata{Name: "leaf"},
					Routes: []*v1.Route{
						{
							Matcher: &gloov1.Matcher{
								PathSpecifier: &gloov1.Matcher_Exact{
									Exact: "/exact",
								},
							},
							Action: &v1.Route_DirectResponseAction{DirectResponseAction: &gloov1.DirectResponseAction{}},
						},
					},
				},
			}
			// create a chain of route tables
			for i := 0; i < 5; i++ {
				ref := routeTables[i].Metadata.Ref()
				routeTables = append(routeTables, &v1.RouteTable{
					Metadata: core.Metadata{Name: fmt.Sprintf("node-%v", i)},
					Routes: []*v1.Route{{
						Matcher: &gloov1.Matcher{
							PathSpecifier: &gloov1.Matcher_Prefix{
								Prefix: "/",
							},
						},
						Action: &v1.Route_DelegateAction{
							DelegateAction: &ref,
						}},
					}})
			}
			ref := routeTables[len(routeTables)-1].Metadata.Ref()

			// create a vs to point to the last route table
			vs := defaults.DefaultVirtualService("a", "b")
			vs.VirtualHost.Routes = []*v1.Route{{
				Matcher: &gloov1.Matcher{
					PathSpecifier: &gloov1.Matcher_Prefix{
						Prefix: "/",
					},
				},
				Action: &v1.Route_DelegateAction{
					DelegateAction: &ref,
				},
			}}

			resourceErrs := reporter.ResourceErrors{}

			routes, err := convertRoutes(vs, routeTables, resourceErrs)
			Expect(err).NotTo(HaveOccurred())
			Expect(resourceErrs.Validate()).NotTo(HaveOccurred())
			Expect(routes).To(HaveLen(1))
			Expect(routes[0].Matcher).To(Equal(&gloov1.Matcher{
				PathSpecifier: &gloov1.Matcher_Exact{
					Exact: "/exact",
				},
			}))
		})
	})

	Context("bad config on a delegate route", func() {
		It("returns the correct error", func() {
			type routeErr struct {
				route       *v1.Route
				expectedErr error
			}
			for _, badRoute := range []routeErr{
				{
					route: &v1.Route{
						Matcher: &gloov1.Matcher{
							PathSpecifier: &gloov1.Matcher_Regex{
								Regex: "/any",
							},
						},
						Action: &v1.Route_DelegateAction{
							DelegateAction: &core.ResourceRef{
								Name: "any",
							},
						},
					},
					expectedErr: missingPrefixErr,
				},
				{
					route: &v1.Route{
						Matcher: &gloov1.Matcher{
							PathSpecifier: &gloov1.Matcher_Exact{
								Exact: "/any",
							},
						},
						Action: &v1.Route_DelegateAction{
							DelegateAction: &core.ResourceRef{
								Name: "any",
							},
						},
					},
					expectedErr: missingPrefixErr,
				},
				{
					route: &v1.Route{
						Matcher: &gloov1.Matcher{
							PathSpecifier: &gloov1.Matcher_Prefix{
								Prefix: "/any",
							},
							Headers: []*gloov1.HeaderMatcher{{}},
						},
						Action: &v1.Route_DelegateAction{
							DelegateAction: &core.ResourceRef{
								Name: "any",
							},
						},
					},
					expectedErr: hasHeaderMatcherErr,
				},
				{
					route: &v1.Route{
						Matcher: &gloov1.Matcher{
							PathSpecifier: &gloov1.Matcher_Prefix{
								Prefix: "/any",
							},
							Methods: []string{"any"},
						},
						Action: &v1.Route_DelegateAction{
							DelegateAction: &core.ResourceRef{
								Name: "any",
							},
						},
					},
					expectedErr: hasMethodMatcherErr,
				},
				{
					route: &v1.Route{
						Matcher: &gloov1.Matcher{
							PathSpecifier: &gloov1.Matcher_Prefix{
								Prefix: "/any",
							},
							QueryParameters: []*gloov1.QueryParameterMatcher{{}},
						},
						Action: &v1.Route_DelegateAction{
							DelegateAction: &core.ResourceRef{
								Name: "any",
							},
						},
					},
					expectedErr: hasQueryMatcherErr,
				},
			} {
				rv := &routeVisitor{}
				_, err := rv.convertDelegateAction(&v1.VirtualService{}, badRoute.route, reporter.ResourceErrors{})
				Expect(err).To(Equal(badRoute.expectedErr))
			}
		})
	})
})
