package regexutils

import (
	"regexp"
)

// CheckRegexString to make sure the string is a valid RE2 expression
func CheckRegexString(candidateRegex string) error {
	// https://github.com/envoyproxy/envoy/blob/v1.30.0/source/common/common/regex.cc#L19C8-L19C14
	// Envoy uses the RE2 library for regex matching in google's owned c++ impl.
	// go has https://pkg.go.dev/regexp which implements RE2 with a single caveat.
	_, err := regexp.Compile(candidateRegex)
	return err
}
