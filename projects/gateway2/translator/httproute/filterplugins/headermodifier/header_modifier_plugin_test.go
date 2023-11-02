package headermodifier_test

import (
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/filtertests"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/headermodifier"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	"github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("HeaderModifierPlugin", func() {
	It("applies header modifier filter", func() {

		plugin := headermodifier.NewPlugin()
		filter := gwv1.HTTPRouteFilter{
			Type: gwv1.HTTPRouteFilterRequestHeaderModifier,
			RequestHeaderModifier: &gwv1.HTTPHeaderFilter{
				Add: []gwv1.HTTPHeader{
					{
						Name:  "foo",
						Value: "bar",
					},
				},
				Set: []gwv1.HTTPHeader{
					{
						Name:  "foo",
						Value: "bar",
					},
				},
				Remove: []string{"foo"},
			},
		}
		expectedRoute := &v1.Route{
			Options: &v1.RouteOptions{
				HeaderManipulation: &headers.HeaderManipulation{
					RequestHeadersToAdd: []*core.HeaderValueOption{
						{
							HeaderOption: &core.HeaderValueOption_Header{
								Header: &core.HeaderValue{
									Key:   "foo",
									Value: "bar",
								},
							},
							Append: &wrappers.BoolValue{Value: true},
						},
						{
							HeaderOption: &core.HeaderValueOption_Header{
								Header: &core.HeaderValue{
									Key:   "foo",
									Value: "bar",
								},
							},
							Append: &wrappers.BoolValue{Value: false},
						},
					},
					RequestHeadersToRemove: []string{"foo"},
				},
			},
		}
		filtertests.AssertExpectedRoute(
			plugin,
			filter,
			expectedRoute,
			false,
		)
	})
})
