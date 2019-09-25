package translator

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/reporter"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
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
