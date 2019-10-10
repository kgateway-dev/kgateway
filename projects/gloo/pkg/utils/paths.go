package utils

import (
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

func PathAsString(matcher *v1.Matcher) string {
	switch path := matcher.PathSpecifier.(type) {
	case *v1.Matcher_Prefix:
		return path.Prefix
	case *v1.Matcher_Exact:
		return path.Exact
	case *v1.Matcher_Regex:
		return path.Regex
	}
	panic("invalid matcher path type, must be one of: {Matcher_Regex, Matcher_Exact, Matcher_Prefix}")
}

func EnvoyPathAsString(matcher route.RouteMatch) string {
	switch path := matcher.PathSpecifier.(type) {
	case *route.RouteMatch_Prefix:
		return path.Prefix
	case *route.RouteMatch_Path:
		return path.Path
	case *route.RouteMatch_Regex:
		return path.Regex
	}
	panic("invalid matcher path type, must be one of: {RouteMatch_Prefix, RouteMatch_Path, RouteMatch_Regex}")
}
