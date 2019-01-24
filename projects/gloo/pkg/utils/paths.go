package utils

import (
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
