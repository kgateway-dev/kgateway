package helpers

import (
	gatwayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/matchers"
)

const (
	ExactPath = iota
	PrefixPath
	RegexPath
)

func MakeMultiMatcherRoute(pathType1, length1, pathType2, length2 int) *v1.Route {
	return &v1.Route{Matchers: []*matchers.Matcher{
		MakeMatcher(pathType1, length1),
		MakeMatcher(pathType2, length2),
	}}
}

func MakeRoute(pathType, length int) *v1.Route {
	return &v1.Route{Matchers: []*matchers.Matcher{MakeMatcher(pathType, length)}}
}

func MakeGatewayRoute(pathType, length int) *gatwayv1.Route {
	return &gatwayv1.Route{Matchers: []*matchers.Matcher{MakeMatcher(pathType, length)}}
}

func MakeMatcher(pathType, length int) *matchers.Matcher {
	pathStr := "/"
	for i := 0; i < length; i++ {
		pathStr += "s/"
	}
	m := &matchers.Matcher{}
	switch pathType {
	case ExactPath:
		m.PathSpecifier = &matchers.Matcher_Exact{pathStr}
	case PrefixPath:
		m.PathSpecifier = &matchers.Matcher_Prefix{pathStr}
	case RegexPath:
		m.PathSpecifier = &matchers.Matcher_Regex{pathStr}
	default:
		panic("bad test")
	}
	return m
}
