package httproute

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func pathMatch(t gwv1.PathMatchType, v string) *gwv1.HTTPRouteMatch {
	return &gwv1.HTTPRouteMatch{
		Path: &gwv1.HTTPPathMatch{Type: new(t), Value: new(v)},
	}
}

func TestMergeParentChildRouteMatchPath(t *testing.T) {
	tests := []struct {
		name         string
		parent       *gwv1.HTTPRouteMatch
		child        *gwv1.HTTPRouteMatch
		expectedPath string
		expectedType gwv1.PathMatchType
		// regexMatches are paths the resulting matcher must match; regexRejects must not match.
		regexMatches []string
		regexRejects []string
		// unanchoredParent asserts the merged regex is not end-anchored, i.e. an
		// unanchored parent regex stays unanchored after merging.
		unanchoredParent bool
	}{
		{
			name:         "PathPrefix parent and PathPrefix child are joined",
			parent:       pathMatch(gwv1.PathMatchPathPrefix, "/api"),
			child:        pathMatch(gwv1.PathMatchPathPrefix, "/v1"),
			expectedPath: "/api/v1",
			expectedType: gwv1.PathMatchPathPrefix,
		},
		{
			name:         "PathPrefix parent and Exact child retain child type",
			parent:       pathMatch(gwv1.PathMatchPathPrefix, "/api"),
			child:        pathMatch(gwv1.PathMatchExact, "/users"),
			expectedPath: "/api/users",
			expectedType: gwv1.PathMatchExact,
		},
		{
			name:         "nil parent path defaults to root PathPrefix",
			parent:       &gwv1.HTTPRouteMatch{},
			child:        pathMatch(gwv1.PathMatchPathPrefix, "/api"),
			expectedPath: "/api",
			expectedType: gwv1.PathMatchPathPrefix,
		},
		{
			name: "nil parent path fields default to root PathPrefix",
			parent: &gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{},
			},
			child:        pathMatch(gwv1.PathMatchExact, "/api"),
			expectedPath: "/api",
			expectedType: gwv1.PathMatchExact,
		},
		{
			name:         "nil child path defaults to root PathPrefix",
			parent:       pathMatch(gwv1.PathMatchPathPrefix, "/api"),
			child:        &gwv1.HTTPRouteMatch{},
			expectedPath: "/api",
			expectedType: gwv1.PathMatchPathPrefix,
		},
		{
			name:   "nil child path fields default to root PathPrefix",
			parent: pathMatch(gwv1.PathMatchPathPrefix, "/api"),
			child: &gwv1.HTTPRouteMatch{
				Path: &gwv1.HTTPPathMatch{},
			},
			expectedPath: "/api",
			expectedType: gwv1.PathMatchPathPrefix,
		},
		{
			name:         "RegularExpression parent and PathPrefix child stay open-ended",
			parent:       pathMatch(gwv1.PathMatchRegularExpression, "^/teams/[^/]+(?:/.*)?$"),
			child:        pathMatch(gwv1.PathMatchPathPrefix, "/members"),
			expectedPath: "^/teams/[^/]+(?:/.*)?/members(?:/.*)?$",
			expectedType: gwv1.PathMatchRegularExpression,
			regexMatches: []string{"/teams/acme/members", "/teams/acme/members/123"},
			regexRejects: []string{"/teams/acme/owners"},
		},
		{
			name:         "RegularExpression parent and Exact child are anchored",
			parent:       pathMatch(gwv1.PathMatchRegularExpression, "^/teams/[^/]+$"),
			child:        pathMatch(gwv1.PathMatchExact, "/members"),
			expectedPath: "^/teams/[^/]+/members$",
			expectedType: gwv1.PathMatchRegularExpression,
			regexMatches: []string{"/teams/acme/members"},
			regexRejects: []string{"/teams/acme/members/extra"},
		},
		{
			name:         "RegularExpression parent and RegularExpression child drop child anchors",
			parent:       pathMatch(gwv1.PathMatchRegularExpression, "^/teams/[^/]+$"),
			child:        pathMatch(gwv1.PathMatchRegularExpression, "^/v[0-9]+$"),
			expectedPath: "^/teams/[^/]+/v[0-9]+$",
			expectedType: gwv1.PathMatchRegularExpression,
			regexMatches: []string{"/teams/acme/v2"},
			regexRejects: []string{"/teams/acme/vX"},
		},
		{
			name:         "special characters in child are escaped under regex parent",
			parent:       pathMatch(gwv1.PathMatchRegularExpression, "^/teams/[^/]+$"),
			child:        pathMatch(gwv1.PathMatchExact, "/a.b"),
			expectedPath: `^/teams/[^/]+/a\.b$`,
			expectedType: gwv1.PathMatchRegularExpression,
			regexMatches: []string{"/teams/acme/a.b"},
			regexRejects: []string{"/teams/acme/axb"},
		},
		{
			name:             "unanchored RegularExpression parent and PathPrefix child stay unanchored",
			parent:           pathMatch(gwv1.PathMatchRegularExpression, "/a/.*"),
			child:            pathMatch(gwv1.PathMatchPathPrefix, "/b"),
			expectedPath:     "/a/.*/b(?:/.*)?",
			expectedType:     gwv1.PathMatchRegularExpression,
			regexMatches:     []string{"/a/x/b", "/a/x/b/deeper"},
			regexRejects:     []string{"/a/x"},
			unanchoredParent: true,
		},
		{
			name:             "unanchored RegularExpression parent and Exact child stay unanchored",
			parent:           pathMatch(gwv1.PathMatchRegularExpression, "/a/.*"),
			child:            pathMatch(gwv1.PathMatchExact, "/b"),
			expectedPath:     "/a/.*/b",
			expectedType:     gwv1.PathMatchRegularExpression,
			regexMatches:     []string{"/a/x/b"},
			regexRejects:     []string{"/a/x"},
			unanchoredParent: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mergeParentChildRouteMatch(tc.parent, tc.child)
			assert.Equal(t, tc.expectedPath, *tc.child.Path.Value)
			assert.Equal(t, tc.expectedType, *tc.child.Path.Type)
			if tc.unanchoredParent {
				assert.False(t, strings.HasSuffix(*tc.child.Path.Value, "$"),
					"unanchored parent regex must stay unanchored after merge")
			}
			if tc.expectedType == gwv1.PathMatchRegularExpression {
				re, err := regexp.Compile(*tc.child.Path.Value)
				assert.NoError(t, err)
				for _, p := range tc.regexMatches {
					assert.True(t, re.MatchString(p), "expected %q to match", p)
				}
				for _, p := range tc.regexRejects {
					assert.False(t, re.MatchString(p), "expected %q not to match", p)
				}
			}
		})
	}
}
