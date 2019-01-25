package helpers

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

const (
	ExactPath = iota
	PrefixPath
	RegexPath
)

func MakeRoute(pathType int, length int) *v1.Route {
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
	return &v1.Route{Matcher: m}
}
