package headermodifier_test

import (
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/filtertests"
	"github.com/solo-io/gloo/projects/gateway2/translator/httproute/filterplugins/headermodifier"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	"github.com/solo-io/solo-kit/pkg/api/external/envoy/api/v2/core"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = DescribeTable(
	"HeaderModifierPlugin",
	func(
		plugin filterplugins.FilterPlugin,
		filter gwv1.HTTPRouteFilter,
		expectedRoute *v1.Route,
	) {
		filtertests.AssertExpectedRoute(
			plugin,
			filter,
			expectedRoute,
			true,
		)
	},
	Entry(
		"applies request header modifier filter",
		headermodifier.NewPlugin(),
		gwv1.HTTPRouteFilter{
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
		},
		&v1.Route{
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
		},
	),
	Entry(
		"applies response header modifier filter",
		headermodifier.NewPlugin(),
		gwv1.HTTPRouteFilter{
			Type: gwv1.HTTPRouteFilterResponseHeaderModifier,
			ResponseHeaderModifier: &gwv1.HTTPHeaderFilter{
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
		},
		&v1.Route{
			Options: &v1.RouteOptions{
				HeaderManipulation: &headers.HeaderManipulation{
					ResponseHeadersToAdd: []*headers.HeaderValueOption{
						{
							Header: &headers.HeaderValue{
								Key:   "foo",
								Value: "bar",
							},
							Append: &wrappers.BoolValue{Value: true},
						},
						{
							Header: &headers.HeaderValue{
								Key:   "foo",
								Value: "bar",
							},
							Append: &wrappers.BoolValue{Value: false},
						},
					},
					ResponseHeadersToRemove: []string{"foo"},
				},
			},
		},
	),
)
