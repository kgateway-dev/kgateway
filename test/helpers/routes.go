package helpers

import (
	gatwayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

const (
	ExactPath = iota
	PrefixPath
	RegexPath
)

func MakeMultiMatcherRoute(pathType1, length1, pathType2, length2 int) *v1.Route {
	return &v1.Route{Matchers: []*v1.Matcher{
		MakeMatcher(pathType1, length1),
		MakeMatcher(pathType2, length2),
	}}
}

func MakeRoute(pathType, length int) *v1.Route {
	return &v1.Route{Matchers: []*v1.Matcher{MakeMatcher(pathType, length)}}
}

func MakeGatewayRoute(pathType, length int) *gatwayv1.Route {
	return &gatwayv1.Route{Matchers: []*v1.Matcher{MakeMatcher(pathType, length)}}
}

func MakeMatcher(pathType, length int) *v1.Matcher {
	pathStr := "/"
	for i := 0; i < length; i++ {
		pathStr += "s/"
	}
	m := &v1.Matcher{}
	switch pathType {
	case ExactPath:
		m.PathSpecifier = &v1.Matcher_Exact{pathStr}
	case PrefixPath:
		m.PathSpecifier = &v1.Matcher_Prefix{pathStr}
	case RegexPath:
		m.PathSpecifier = &v1.Matcher_Regex{pathStr}
	default:
		panic("bad test")
	}
	return m
}
