package utils

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode"
)

// Virtual host and virtual cluster names cannot contain dots, otherwise Envoy might incorrectly compute
// its statistics tree. Any occurrences will be replaced with underscores.
const (
	illegalChar     = "."
	replacementChar = "_"
)

func SanitizeForEnvoy(ctx context.Context, resourceName, resourceTypeName string) string {
	if strings.Contains(resourceName, illegalChar) {
		//nolint:sloglint // ignore formatting
		slog.Debug(fmt.Sprintf("illegal character(s) '%s' in %s name [%s] will be replaced by '%s'",
			illegalChar, resourceTypeName, resourceName, replacementChar))
		resourceName = strings.ReplaceAll(resourceName, illegalChar, replacementChar)
	}
	return resourceName
}

// SanitizeCookieName ensures the cookie name is valid per RFC 6265 and Gateway API conventions.
// - Only ASCII, no control chars, no separators, not empty, not too long (max 64 chars recommended)
// - Replaces illegal characters with '_'
func SanitizeCookieName(name string) string {
	if len(name) == 0 {
		return "cookie"
	}
	// RFC 2616 separators: ()<>@,;:\"/[]?={} and whitespace
	/*

		https://www.rfc-editor.org/rfc/rfc2616#section-2.2
		       token          = 1*<any CHAR except CTLs or separators>
		       separators     = "(" | ")" | "<" | ">" | "@"
		                      | "," | ";" | ":" | "\" | <">
		                      | "/" | "[" | "]" | "?" | "="
		                      | "{" | "}" | SP | HT

	*/
	separators := "()<>@,;:\\\"/[]?={} \t"

	return strings.Map(func(c rune) rune {
		if unicode.IsControl(c) || c > unicode.MaxASCII || strings.ContainsRune(separators, c) {
			return '_'
		}
		return c
	}, name)
}

// SanitizeHeaderName ensures the header name is valid per RFC 7230 and Gateway API conventions.
// - Only allowed header chars: ^[A-Za-z0-9!#$%&'*+\-.^_`|~]+$
// - Not empty, not too long (max 256 chars)
// - Replaces illegal characters with '_'
func SanitizeHeaderName(name string) string {
	if len(name) == 0 {
		return "header"
	}
	/*
		https://www.rfc-editor.org/rfc/rfc7230#section-3.2.6
		     token          = 1*tchar

		     tchar          = "!" / "#" / "$" / "%" / "&" / "'" / "*"
		                    / "+" / "-" / "." / "^" / "_" / "`" / "|" / "~"
		                    / DIGIT / ALPHA
		                    ; any VCHAR, except delimiters


	*/
	return strings.Map(func(c rune) rune {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			strings.ContainsRune("!#$%&'*+-.^_`|~", rune(c)) {
			return c
		}
		return '-'
	}, name)
}
