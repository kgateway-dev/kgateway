package delegation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestChildRouteCanAttachToParentRef(t *testing.T) {
	testCases := []struct {
		name            string
		routeNamespace  string
		routeParentRefs []gwv1.ParentReference
		parentRef       types.NamespacedName
		expected        bool
	}{
		{
			name:           "no ParentRefs, should allow attachment",
			routeNamespace: "default",
			parentRef:      types.NamespacedName{Name: "parent", Namespace: "default"},
			expected:       true,
		},
		{
			name:           "ParentRefs match, should allow attachment",
			routeNamespace: "default",
			routeParentRefs: []gwv1.ParentReference{
				{
					Group:     new(gwv1.Group("gateway.networking.k8s.io")),
					Kind:      new(gwv1.Kind("HTTPRoute")),
					Name:      "parent",
					Namespace: new(gwv1.Namespace("default")),
				},
			},
			parentRef: types.NamespacedName{Name: "parent", Namespace: "default"},
			expected:  true,
		},
		{
			name:           "ParentRef doesn't match Name, should not allow attachment",
			routeNamespace: "default",
			routeParentRefs: []gwv1.ParentReference{
				{
					Group:     new(gwv1.Group("gateway.networking.k8s.io")),
					Kind:      new(gwv1.Kind("HTTPRoute")),
					Name:      "invalid",
					Namespace: new(gwv1.Namespace("default")),
				},
			},
			parentRef: types.NamespacedName{Name: "parent", Namespace: "default"},
			expected:  false,
		},
		{
			name:           "ParentRef doesn't match Namespace, should not allow attachment",
			routeNamespace: "default",
			routeParentRefs: []gwv1.ParentReference{
				{
					Group:     new(gwv1.Group("gateway.networking.k8s.io")),
					Kind:      new(gwv1.Kind("HTTPRoute")),
					Name:      "parent",
					Namespace: new(gwv1.Namespace("invalid")),
				},
			},
			parentRef: types.NamespacedName{Name: "parent", Namespace: "default"},
			expected:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := assert.New(t)
			result := ChildRouteCanAttachToParentRef(tc.routeNamespace, tc.routeParentRefs, tc.parentRef)
			a.Equal(tc.expected, result)
		})
	}
}

func TestIsDelegatedRouteMatch(t *testing.T) {
	testCases := []struct {
		name     string
		parent   gwv1.HTTPRouteMatch
		child    gwv1.HTTPRouteMatch
		expected bool
	}{
		{
			name: "child route without parentRef matches parent",
			parent: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  new(gwv1.PathMatchPathPrefix),
					Value: new("/foo"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  new(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
				},
				Method: new(gwv1.HTTPMethod("GET")),
			},
			child: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  new(gwv1.PathMatchPathPrefix),
					Value: new("/foo/baz"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header3"),
						Value: "val3",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  new(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query3"),
						Value: "val3.*foo",
					},
				},
				Method: new(gwv1.HTTPMethod("GET")),
			},
			expected: true,
		},
		{
			name: "child route without parentRef doesn't match parent path",
			parent: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  new(gwv1.PathMatchPathPrefix),
					Value: new("/foo"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  new(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
				},
			},
			child: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  new(gwv1.PathMatchPathPrefix),
					Value: new("/bar/baz"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header3"),
						Value: "val3",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  new(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
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
					Type:  new(gwv1.PathMatchPathPrefix),
					Value: new("/foo"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  new(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
				},
			},
			child: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  new(gwv1.PathMatchPathPrefix),
					Value: new("/foo/baz"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header3"),
						Value: "val3",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  new(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
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
					Type:  new(gwv1.PathMatchPathPrefix),
					Value: new("/foo"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  new(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
				},
			},
			child: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  new(gwv1.PathMatchPathPrefix),
					Value: new("/foo/baz"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header3"),
						Value: "val3",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
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
					Type:  new(gwv1.PathMatchPathPrefix),
					Value: new("/foo"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  new(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
				},
				Method: new(gwv1.HTTPMethod("GET")),
			},
			child: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  new(gwv1.PathMatchPathPrefix),
					Value: new("/foo/baz"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header3"),
						Value: "val3",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  new(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query3"),
						Value: "val3.*foo",
					},
				},
				Method: new(gwv1.HTTPMethod("PUT")),
			},
			expected: false,
		},
		{
			name: "child route with parentRef matches parent",
			parent: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  new(gwv1.PathMatchPathPrefix),
					Value: new("/foo"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  new(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
				},
			},
			child: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  new(gwv1.PathMatchPathPrefix),
					Value: new("/foo/baz"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header3"),
						Value: "val3",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  new(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query3"),
						Value: "val3.*foo",
					},
				},
			},
			expected: true,
		},
		{
			name: "child route with parentRef matches parent without parentRef.Namespace set",
			parent: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  new(gwv1.PathMatchPathPrefix),
					Value: new("/foo"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  new(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
				},
			},
			child: gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{
					Type:  new(gwv1.PathMatchPathPrefix),
					Value: new("/foo/baz"),
				},
				Headers: []gwv1.HTTPHeaderMatch{
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.HeaderMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("header2"),
						Value: "val2.*foo",
					},
					{
						Type:  new(gwv1.HeaderMatchExact),
						Name:  gwv1.HTTPHeaderName("header3"),
						Value: "val3",
					},
				},
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{
						Type:  new(gwv1.QueryParamMatchExact),
						Name:  gwv1.HTTPHeaderName("query1"),
						Value: "val1",
					},
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query2"),
						Value: "val2.*foo",
					},
					{
						Type:  new(gwv1.QueryParamMatchRegularExpression),
						Name:  gwv1.HTTPHeaderName("query3"),
						Value: "val3.*foo",
					},
				},
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a := assert.New(t)
			actual := IsDelegatedRouteMatch(tc.parent, tc.child)

			a.Equal(tc.expected, actual)
		})
	}
}

func pathMatch(t gwv1.PathMatchType, value string) *gwv1.HTTPPathMatch {
	return &gwv1.HTTPPathMatch{Type: new(t), Value: new(value)}
}

func TestFilterDelegatedRuleMatches(t *testing.T) {
	parentMatch := gwv1.HTTPRouteMatch{
		Path:   pathMatch(gwv1.PathMatchPathPrefix, "/api"),
		Method: new(gwv1.HTTPMethodGet),
		Headers: []gwv1.HTTPHeaderMatch{
			{Name: "x-parent", Value: "p"},
		},
	}

	testCases := []struct {
		name           string
		parentMatch    gwv1.HTTPRouteMatch
		ruleMatches    []gwv1.HTTPRouteMatch
		inheritMatcher bool
		expected       []gwv1.HTTPRouteMatch
	}{
		{
			name:        "non-inherit keeps matching child matches verbatim and drops the rest",
			parentMatch: parentMatch,
			ruleMatches: []gwv1.HTTPRouteMatch{
				{
					Path:    pathMatch(gwv1.PathMatchPathPrefix, "/api/users"),
					Method:  new(gwv1.HTTPMethodGet),
					Headers: []gwv1.HTTPHeaderMatch{{Name: "x-parent", Value: "p"}},
				},
				{
					// not under the parent prefix
					Path:    pathMatch(gwv1.PathMatchPathPrefix, "/other"),
					Method:  new(gwv1.HTTPMethodGet),
					Headers: []gwv1.HTTPHeaderMatch{{Name: "x-parent", Value: "p"}},
				},
			},
			expected: []gwv1.HTTPRouteMatch{
				{
					Path:    pathMatch(gwv1.PathMatchPathPrefix, "/api/users"),
					Method:  new(gwv1.HTTPMethodGet),
					Headers: []gwv1.HTTPHeaderMatch{{Name: "x-parent", Value: "p"}},
				},
			},
		},
		{
			name:        "non-inherit with no declared matches yields nothing",
			parentMatch: parentMatch,
			ruleMatches: nil,
			expected:    nil,
		},
		{
			name:           "inherit with no declared matches inherits the parent match verbatim",
			parentMatch:    parentMatch,
			ruleMatches:    nil,
			inheritMatcher: true,
			expected:       []gwv1.HTTPRouteMatch{parentMatch},
		},
		{
			name: "inherit merges each child match onto the parent match",
			parentMatch: gwv1.HTTPRouteMatch{
				Path:        pathMatch(gwv1.PathMatchPathPrefix, "/api"),
				Method:      new(gwv1.HTTPMethodGet),
				Headers:     []gwv1.HTTPHeaderMatch{{Name: "x-shared", Value: "parent"}},
				QueryParams: []gwv1.HTTPQueryParamMatch{{Name: "version", Value: "v1"}},
			},
			ruleMatches: []gwv1.HTTPRouteMatch{
				{
					Path:   pathMatch(gwv1.PathMatchPathPrefix, "/users"),
					Method: new(gwv1.HTTPMethodDelete),
					Headers: []gwv1.HTTPHeaderMatch{
						{Name: "x-shared", Value: "child"},
						{Name: "x-child", Value: "c"},
					},
				},
				{
					Path: pathMatch(gwv1.PathMatchExact, "/pets"),
				},
			},
			inheritMatcher: true,
			expected: []gwv1.HTTPRouteMatch{
				{
					Path: pathMatch(gwv1.PathMatchPathPrefix, "/api/users"),
					// parent method overwrites the child's
					Method: new(gwv1.HTTPMethodGet),
					// parent wins the name conflict; union is name-sorted
					Headers: []gwv1.HTTPHeaderMatch{
						{Name: "x-child", Value: "c"},
						{Name: "x-shared", Value: "parent"},
					},
					QueryParams: []gwv1.HTTPQueryParamMatch{{Name: "version", Value: "v1"}},
				},
				{
					// child keeps its own path match type
					Path:        pathMatch(gwv1.PathMatchExact, "/api/pets"),
					Method:      new(gwv1.HTTPMethodGet),
					Headers:     []gwv1.HTTPHeaderMatch{{Name: "x-shared", Value: "parent"}},
					QueryParams: []gwv1.HTTPQueryParamMatch{{Name: "version", Value: "v1"}},
				},
			},
		},
		{
			name: "inherit defaults a nil child path before joining",
			parentMatch: gwv1.HTTPRouteMatch{
				Path: pathMatch(gwv1.PathMatchPathPrefix, "/api"),
			},
			ruleMatches: []gwv1.HTTPRouteMatch{
				{Method: new(gwv1.HTTPMethodPost)},
			},
			inheritMatcher: true,
			expected: []gwv1.HTTPRouteMatch{
				{
					Path:   pathMatch(gwv1.PathMatchPathPrefix, "/api"),
					Method: new(gwv1.HTTPMethodPost),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parentBefore := *tc.parentMatch.DeepCopy()
			var rulesBefore []gwv1.HTTPRouteMatch
			for _, m := range tc.ruleMatches {
				rulesBefore = append(rulesBefore, *m.DeepCopy())
			}

			result := FilterDelegatedRuleMatches(tc.parentMatch, tc.ruleMatches, tc.inheritMatcher)
			assert.Equal(t, tc.expected, result)

			// Neither input is mutated.
			assert.Equal(t, parentBefore, tc.parentMatch)
			assert.Equal(t, rulesBefore, tc.ruleMatches)
		})
	}
}

func TestMergeParentChildRouteMatch(t *testing.T) {
	testCases := []struct {
		name     string
		parent   gwv1.HTTPRouteMatch
		child    gwv1.HTTPRouteMatch
		expected gwv1.HTTPRouteMatch
	}{
		{
			name:     "joins parent and child paths",
			parent:   gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchPathPrefix, "/api")},
			child:    gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchPathPrefix, "/users")},
			expected: gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchPathPrefix, "/api/users")},
		},
		{
			name:     "normalizes redundant slashes in the joined path",
			parent:   gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchPathPrefix, "/api/")},
			child:    gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchPathPrefix, "/users/")},
			expected: gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchPathPrefix, "/api/users")},
		},
		{
			name:     "child keeps its own path match type",
			parent:   gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchPathPrefix, "/api")},
			child:    gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchExact, "/users")},
			expected: gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchExact, "/api/users")},
		},
		{
			name:   "a nil child path defaults to the parent path",
			parent: gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchPathPrefix, "/api")},
			child:  gwv1.HTTPRouteMatch{Method: new(gwv1.HTTPMethodPost)},
			expected: gwv1.HTTPRouteMatch{
				Path:   pathMatch(gwv1.PathMatchPathPrefix, "/api"),
				Method: new(gwv1.HTTPMethodPost),
			},
		},
		{
			name:     "a nil parent path leaves the child path alone",
			parent:   gwv1.HTTPRouteMatch{Method: new(gwv1.HTTPMethodGet)},
			child:    gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchExact, "/users")},
			expected: gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchExact, "/users"), Method: new(gwv1.HTTPMethodGet)},
		},
		{
			name:     "a nil parent path value leaves the child path alone",
			parent:   gwv1.HTTPRouteMatch{Path: &gwv1.HTTPPathMatch{Type: new(gwv1.PathMatchPathPrefix)}},
			child:    gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchPathPrefix, "/users")},
			expected: gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchPathPrefix, "/users")},
		},
		{
			name:     "a valueless child path is treated as unset and inherits the parent path as a prefix match",
			parent:   gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchPathPrefix, "/api")},
			child:    gwv1.HTTPRouteMatch{Path: &gwv1.HTTPPathMatch{Type: new(gwv1.PathMatchExact)}},
			expected: gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchPathPrefix, "/api")},
		},
		{
			name:     "paths absent on both sides resolve to the root prefix",
			parent:   gwv1.HTTPRouteMatch{Method: new(gwv1.HTTPMethodGet)},
			child:    gwv1.HTTPRouteMatch{},
			expected: gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchPathPrefix, "/"), Method: new(gwv1.HTTPMethodGet)},
		},
		{
			name: "parent header wins a name conflict and the union is name-sorted",
			parent: gwv1.HTTPRouteMatch{
				Path: pathMatch(gwv1.PathMatchPathPrefix, "/api"),
				Headers: []gwv1.HTTPHeaderMatch{
					{Name: "x-shared", Value: "parent"},
					{Name: "x-parent", Value: "p"},
				},
			},
			child: gwv1.HTTPRouteMatch{
				Path: pathMatch(gwv1.PathMatchPathPrefix, "/users"),
				Headers: []gwv1.HTTPHeaderMatch{
					{Name: "x-shared", Value: "child"},
					{Name: "x-child", Value: "c"},
				},
			},
			expected: gwv1.HTTPRouteMatch{
				Path: pathMatch(gwv1.PathMatchPathPrefix, "/api/users"),
				Headers: []gwv1.HTTPHeaderMatch{
					{Name: "x-child", Value: "c"},
					{Name: "x-parent", Value: "p"},
					{Name: "x-shared", Value: "parent"},
				},
			},
		},
		{
			name: "parent query param wins a name conflict and the union is name-sorted",
			parent: gwv1.HTTPRouteMatch{
				Path: pathMatch(gwv1.PathMatchPathPrefix, "/api"),
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{Name: "version", Value: "v1"},
					{Name: "zone", Value: "us"},
				},
			},
			child: gwv1.HTTPRouteMatch{
				Path: pathMatch(gwv1.PathMatchPathPrefix, "/users"),
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{Name: "version", Value: "v2"},
					{Name: "page", Value: "1"},
				},
			},
			expected: gwv1.HTTPRouteMatch{
				Path: pathMatch(gwv1.PathMatchPathPrefix, "/api/users"),
				QueryParams: []gwv1.HTTPQueryParamMatch{
					{Name: "page", Value: "1"},
					{Name: "version", Value: "v1"},
					{Name: "zone", Value: "us"},
				},
			},
		},
		{
			name: "parent method overwrites the child method",
			parent: gwv1.HTTPRouteMatch{
				Path:   pathMatch(gwv1.PathMatchPathPrefix, "/api"),
				Method: new(gwv1.HTTPMethodGet),
			},
			child: gwv1.HTTPRouteMatch{
				Path:   pathMatch(gwv1.PathMatchPathPrefix, "/users"),
				Method: new(gwv1.HTTPMethodDelete),
			},
			expected: gwv1.HTTPRouteMatch{
				Path:   pathMatch(gwv1.PathMatchPathPrefix, "/api/users"),
				Method: new(gwv1.HTTPMethodGet),
			},
		},
		{
			name:   "child keeps its method when the parent has none",
			parent: gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchPathPrefix, "/api")},
			child: gwv1.HTTPRouteMatch{
				Path:   pathMatch(gwv1.PathMatchPathPrefix, "/users"),
				Method: new(gwv1.HTTPMethodDelete),
			},
			expected: gwv1.HTTPRouteMatch{
				Path:   pathMatch(gwv1.PathMatchPathPrefix, "/api/users"),
				Method: new(gwv1.HTTPMethodDelete),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parentBefore := *tc.parent.DeepCopy()

			MergeParentChildRouteMatch(&tc.parent, &tc.child)

			assert.Equal(t, tc.expected, tc.child)
			// The merge writes only to the child.
			assert.Equal(t, parentBefore, tc.parent)
		})
	}
}

func TestMergeParentChildRouteMatchNilSafety(t *testing.T) {
	child := gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchPathPrefix, "/users")}
	childBefore := *child.DeepCopy()
	MergeParentChildRouteMatch(nil, &child)
	assert.Equal(t, childBefore, child)

	// A nil child is a no-op rather than a panic.
	parent := gwv1.HTTPRouteMatch{Path: pathMatch(gwv1.PathMatchPathPrefix, "/api")}
	MergeParentChildRouteMatch(&parent, nil)
}
