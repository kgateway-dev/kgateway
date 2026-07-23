package regexutils

import (
	"regexp"

	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
)

// CheckRegexString to make sure the string is a valid RE2 expression
func CheckRegexString(candidateRegex string) error {
	// https://github.com/envoyproxy/envoy/blob/v1.30.0/source/common/common/regex.cc#L19C8-L19C14
	// Envoy uses the RE2 library for regex matching in google's owned c++ impl.
	// go has https://pkg.go.dev/regexp which implements RE2 with a single caveat.
	_, err := regexp.Compile(candidateRegex)
	return err
}

// NewRegexWithProgramSize creates a new regex matcher with the given program size.
// This means its tightly coupled to envoy's implementation of regex.
// NOTE: Call this after having checked regex with CheckRegexString.
// Deprecated programsize: The MaxProgramSize field inside GoogleRE2 is deprecated and ignored by Envoy.
// The programsize parameter is kept for API compatibility but has no effect.
func NewRegexWithProgramSize(candidateRegex string, _ *uint32) *envoy_type_matcher_v3.RegexMatcher {
	return &envoy_type_matcher_v3.RegexMatcher{
		Regex: candidateRegex,
	}
}
