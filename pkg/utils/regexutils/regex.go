package regexutils

import (
	"regexp"

	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	wrappers "google.golang.org/protobuf/types/known/wrapperspb"
)

// TODO: [gloo cleanup] keeping this around as it may be needed, but the previous function call chain relied on
// the gloov1.Settings type. Can be refactored (including tests) if needed
// NewCheckedRegex creates a new regex matcher with the given regex.
// It is tightly coupled to envoy's implementation of regex.
// func NewCheckedRegex(ctx context.Context, candidateRegex string) (*envoy_type_matcher_v3.RegexMatcher, error) {
// 	if err := CheckRegexString(candidateRegex); err != nil {
// 		return nil, err
// 	}
// 	return NewRegex(ctx, candidateRegex), nil
// }

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
func NewRegexWithProgramSize(candidateRegex string, programsize *uint32) *envoy_type_matcher_v3.RegexMatcher {
	var maxProgramSize *wrappers.UInt32Value
	if programsize != nil {
		maxProgramSize = &wrappers.UInt32Value{
			Value: *programsize,
		}
	}

	return &envoy_type_matcher_v3.RegexMatcher{
		EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
			GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{MaxProgramSize: maxProgramSize},
		},
		Regex: candidateRegex,
	}
}
