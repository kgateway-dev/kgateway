package irtranslator

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	// invalidPathSequences are path sequences that should not be contained in a path
	invalidPathSequences = []string{"//", "/./", "/../", "%2f", "%2F", "#"}
	// invalidPathSuffixes are path suffixes that should not be at the end of a path
	invalidPathSuffixes = []string{"/..", "/."}
	// validCharacterRegex pattern based off RFC 3986 similar to kubernetes-sigs/gateway-api implementation
	// for finding "pchar" characters = unreserved / pct-encoded / sub-delims / ":" / "@"
	validPathRegexCharacters = "^(?:([A-Za-z0-9/:@._~!$&'()*+,:=;-]*|[%][0-9a-fA-F]{2}))*$"

	validPathRegex = regexp.MustCompile(validPathRegexCharacters)

	NoDestinationSpecifiedError       = errors.New("must specify at least one weighted destination for multi destination routes")
	ValidRoutePatternError            = fmt.Errorf("must only contain valid characters matching pattern %s", validPathRegexCharacters)
	PathContainsInvalidCharacterError = func(s, invalid string) error {
		return fmt.Errorf("path [%s] cannot contain [%s]", s, invalid)
	}
	PathEndsWithInvalidCharactersError = func(s, invalid string) error {
		return fmt.Errorf("path [%s] cannot end with [%s]", s, invalid)
	}
)

func validatePath(path string, errs *[]error) {
	if err := ValidateRoutePath(path); err != nil {
		*errs = append(*errs, fmt.Errorf("the \"%s\" path is invalid: %w", path, err))
	}
}

func validatePrefixRewrite(rewrite string, errs *[]error) {
	if err := ValidatePrefixRewrite(rewrite); err != nil {
		*errs = append(*errs, fmt.Errorf("the rewrite %s is invalid: %w", rewrite, err))
	}
}

// ValidatePrefixRewrite will validate the rewrite using url.Parse. Then it will evaluate the Path of the rewrite.
func ValidatePrefixRewrite(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return err
	}
	return ValidateRoutePath(u.Path)
}

// ValidateRoutePath will validate a string for all characters according to RFC 3986
// "pchar" characters = unreserved / pct-encoded / sub-delims / ":" / "@"
// https://www.rfc-editor.org/rfc/rfc3986/
func ValidateRoutePath(s string) error {
	if s == "" {
		return nil
	}

	if !validPathRegex.Match([]byte(s)) {
		return ValidRoutePatternError
	}
	for _, invalid := range invalidPathSequences {
		if strings.Contains(s, invalid) {
			return PathContainsInvalidCharacterError(s, invalid)
		}
	}
	for _, invalid := range invalidPathSuffixes {
		if strings.HasSuffix(s, invalid) {
			return PathEndsWithInvalidCharactersError(s, invalid)
		}
	}
	return nil
}
