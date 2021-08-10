package utils

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
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

func EnvoyPathAsString(matcher *envoy_config_route_v3.RouteMatch) string {
	switch path := matcher.GetPathSpecifier().(type) {
	case *envoy_config_route_v3.RouteMatch_Prefix:
		return path.Prefix
	case *envoy_config_route_v3.RouteMatch_Path:
		return path.Path
	case *envoy_config_route_v3.RouteMatch_SafeRegex:
		return path.SafeRegex.GetRegex()
	}
	return ""
}
