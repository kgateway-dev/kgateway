package utils

import (
	route "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
)

func PathAsString(matcher *matchers.Matcher) string {
	switch path := matcher.GetPathSpecifier().(type) {
	case *matchers.Matcher_Prefix:
		return path.Prefix
	case *matchers.Matcher_Exact:
		return path.Exact
	case *matchers.Matcher_Regex:
		return path.Regex
	}
	return ""
}

func EnvoyPathAsString(matcher *route.RouteMatch) string {
	switch path := matcher.GetPathSpecifier().(type) {
	case *route.RouteMatch_Prefix:
		return path.Prefix
	case *route.RouteMatch_Path:
		return path.Path
	case *route.RouteMatch_Regex:
		return path.Regex
	}
	return ""
}
