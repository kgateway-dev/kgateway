package utils

import (
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
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
	case *route.RouteMatch_SafeRegex:
		return path.SafeRegex.Regex
	//case *route.RouteMatch_ConnectMatcher_: CONNECT request- doesn't have a path
	case *route.RouteMatch_HiddenEnvoyDeprecatedRegex:
		return path.HiddenEnvoyDeprecatedRegex
	}
	return ""
}
