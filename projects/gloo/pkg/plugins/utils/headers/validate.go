package headers

import (
	"fmt"
	"regexp"
)

var (
	// Regex to check that header names consists of only valid ASCII characters
	// https://github.com/envoyproxy/envoy/blob/b0f4332867267913d9aa80c5c0befda14a00d826/source/common/http/character_set_validation.h#L24-L35
	validHeaderNameRegex = regexp.MustCompile("^([a-zA-Z0-9!#$%&'*+.^_`|~-])+$")
)

// ValidateHeaderKey checks whether a header is valid based on the RFC and envoy's regex to accept a header key
func ValidateHeaderKey(key string) error {
	if len(key) == 0 {
		return fmt.Errorf("empty HTTP header names are not allowed")
	}
	if !validHeaderNameRegex.MatchString(key) {
		return fmt.Errorf("'%s' is an invalid HTTP header key", key)
	}
	return nil
}
