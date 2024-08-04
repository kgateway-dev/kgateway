package httproute

import (
	"testing"

	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestIsDelegatedRouteMatch(t *testing.T) {
	testCases := []struct {
		name       string
		parent     gwv1.HTTPRouteMatch
		parentRef  types.NamespacedName
		child      gwv1.HTTPRouteMatch
		childNs    string
		parentRefs []gwv1.ParentReference
		expected   bool
	}{
		{
			name: "child route without parentRef matches parent",
			parent: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  ptr.To(gwv1.PathMatchPathPrefix),
					Value: ptr.To("/foo"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  ptr.To(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
				},
				Method: ptr.To[gwv1.HTTPMethod]("GET"),
			},
			child: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  ptr.To(gwv1.PathMatchPathPrefix),
					Value: ptr.To("/foo/baz"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header3"),
						Value: "val3",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  ptr.To(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query3"),
						Value: "val3.*foo",
					},
				},
				Method: ptr.To[gwv1.HTTPMethod]("GET"),
			},
			expected: true,
		},
		{
			name: "child route without parentRef doesn't match parent path",
			parent: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  ptr.To(gwv1.PathMatchPathPrefix),
					Value: ptr.To("/foo"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  ptr.To(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
				},
			},
			child: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  ptr.To(gwv1.PathMatchPathPrefix),
					Value: ptr.To("/bar/baz"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header3"),
						Value: "val3",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  ptr.To(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query3"),
						Value: "val3.*foo",
					},
				},
			},
			expected: false,
		},
		{
			name: "child route without parentRef doesn't match parent headers",
			parent: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  ptr.To(gwv1.PathMatchPathPrefix),
					Value: ptr.To("/foo"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  ptr.To(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
				},
			},
			child: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  ptr.To(gwv1.PathMatchPathPrefix),
					Value: ptr.To("/foo/baz"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header3"),
						Value: "val3",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  ptr.To(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query3"),
						Value: "val3.*foo",
					},
				},
			},
			expected: false,
		},
		{
			name: "child route without parentRef doesn't parent query params",
			parent: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  ptr.To(gwv1.PathMatchPathPrefix),
					Value: ptr.To("/foo"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  ptr.To(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
				},
			},
			child: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  ptr.To(gwv1.PathMatchPathPrefix),
					Value: ptr.To("/foo/baz"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header3"),
						Value: "val3",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query3"),
						Value: "val3.*foo",
					},
				},
			},
			expected: false,
		},
		{
			name: "child route without parentRef doesn't match parent method",
			parent: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  ptr.To(gwv1.PathMatchPathPrefix),
					Value: ptr.To("/foo"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  ptr.To(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
				},
				Method: ptr.To[gwv1.HTTPMethod]("GET"),
			},
			child: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  ptr.To(gwv1.PathMatchPathPrefix),
					Value: ptr.To("/foo/baz"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header3"),
						Value: "val3",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  ptr.To(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query3"),
						Value: "val3.*foo",
					},
				},
				Method: ptr.To[gwv1.HTTPMethod]("PUT"),
			},
			expected: false,
		},
		{
			name: "child route with parentRef matches parent",
			parent: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  ptr.To(gwv1.PathMatchPathPrefix),
					Value: ptr.To("/foo"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  ptr.To(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
				},
			},
			child: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  ptr.To(gwv1.PathMatchPathPrefix),
					Value: ptr.To("/foo/baz"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header3"),
						Value: "val3",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  ptr.To(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query3"),
						Value: "val3.*foo",
					},
				},
			},
			parentRef: types.NamespacedName{Namespace: "default", Name: "parent"},
			childNs:   "child",
			parentRefs: []gwv1.ParentReference{
				{
					Group:     ptr.To[gwv1.Group](gwv1.Group(wellknown.GatewayGroup)),
					Kind:      ptr.To[gwv1.Kind](gwv1.Kind(wellknown.HTTPRouteKind)),
					Name:      "parent",
					Namespace: ptr.To[gwv1.Namespace](gwv1.Namespace("default")),
				},
			},
			expected: true,
		},
		{
			name: "child route with parentRef matches parent without parentRef.Namespace set",
			parent: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  ptr.To(gwv1.PathMatchPathPrefix),
					Value: ptr.To("/foo"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  ptr.To(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
				},
			},
			child: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  ptr.To(gwv1.PathMatchPathPrefix),
					Value: ptr.To("/foo/baz"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
					{
						Type:  ptr.To(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header3"),
						Value: "val3",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  ptr.To(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
					{
						Type:  ptr.To(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query3"),
						Value: "val3.*foo",
					},
				},
			},
			parentRef: types.NamespacedName{Namespace: "default", Name: "parent"},
			childNs:   "default",
			parentRefs: []gwv1.ParentReference{
				{
					Group: ptr.To[gwv1.Group](gwv1.Group(wellknown.GatewayGroup)),
					Kind:  ptr.To[gwv1.Kind](gwv1.Kind(wellknown.HTTPRouteKind)),
					Name:  "parent",
				},
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := assert.New(t)
			actual := isDelegatedRouteMatch(tc.parent, tc.parentRef, tc.child, tc.childNs, tc.parentRefs)

			a.Equal(tc.expected, actual)
		})
	}
}
