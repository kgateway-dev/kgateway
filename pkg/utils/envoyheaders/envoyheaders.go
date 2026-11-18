// Package envoyheaders mirrors the checks Envoy's HeaderParser applies to
// header mutations (route, virtual host, route configuration, local reply and
// tunneling headers_to_add / headers_to_remove).
//
// Envoy rejects the whole resource carrying a mutation it cannot parse: one
// bad route header NACKs the RouteConfiguration, one bad local-reply header
// NACKs the listener. Checking here lets the translator contain the failure to
// the object that caused it.
//
// The checks never reject a mutation Envoy accepts. They do not catch
// everything Envoy rejects: command-specific errors in a well-formed
// substitution, such as an unknown %NOT_A_COMMAND% or a %REQ(a)b)% whose
// argument is not a header name, are caught only by strict-mode validation,
// because they depend on the Envoy build.
package envoyheaders

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
)

// commandRegex is Envoy's SubstitutionFormatParser command regex:
// %COMMAND(SUBCOMMAND):LENGTH%, where (SUBCOMMAND) and :LENGTH are optional.
var commandRegex = regexp.MustCompile(`^%((?:[A-Z]|[0-9]|_)+)(?:\((.*?)\))?(?::([0-9]+))?%`)

// IsMutable reports whether Envoy lets a HeaderParser add, set or remove the
// header. It mirrors Envoy's HeaderUtility::isModifiableHeader and
// isRemovableHeader, which reject host and every :-prefixed pseudo-header.
func IsMutable(name string) bool {
	return !strings.HasPrefix(name, ":") && !strings.EqualFold(name, "host")
}

// ValidateLiteral checks a value Envoy uses verbatim, such as
// host_rewrite_literal, against the HTTP_HEADER_VALUE rule in Envoy's proto
// validation (strict: false), which forbids NUL, CR and LF.
func ValidateLiteral(value string) error {
	if i := strings.IndexAny(value, "\x00\r\n"); i >= 0 {
		return fmt.Errorf("contains a NUL, CR or LF character at position %d", i)
	}
	return nil
}

// ValidateValue checks a header value that Envoy parses as a substitution
// format string. Besides the literal rule, every % must either be escaped as
// %% or start a %COMMAND%, %COMMAND(ARG)% or %COMMAND(ARG):LENGTH% command.
// Errors never include the value, which may come from a Secret.
func ValidateValue(value string) error {
	if err := ValidateLiteral(value); err != nil {
		return err
	}
	for pos := 0; pos < len(value); {
		if value[pos] != '%' {
			pos++
			continue
		}
		if pos+1 < len(value) && value[pos+1] == '%' {
			pos += 2
			continue
		}
		m := commandRegex.FindStringSubmatch(value[pos:])
		if m == nil {
			return fmt.Errorf("no valid substitution command at position %d; use %%%% for a literal %%", pos)
		}
		if m[3] != "" {
			if _, err := strconv.ParseUint(m[3], 10, 64); err != nil {
				return fmt.Errorf("out-of-range substitution length at position %d", pos)
			}
		}
		pos += len(m[0])
	}
	return nil
}

// ValidateMutations checks headers to add and remove as Envoy's HeaderParser
// does, returning one error per offending header.
func ValidateMutations(add []*envoycorev3.HeaderValueOption, remove []string) error {
	var errs []error
	for _, option := range add {
		name := option.GetHeader().GetKey()
		if !IsMutable(name) {
			errs = append(errs, fmt.Errorf("cannot set header %q: Envoy does not allow modifying host or :-prefixed pseudo-headers", name))
			continue
		}
		if err := ValidateValue(option.GetHeader().GetValue()); err != nil {
			errs = append(errs, fmt.Errorf("invalid value for header %q: %w", name, err))
		}
	}
	for _, name := range remove {
		if !IsMutable(name) {
			errs = append(errs, fmt.Errorf("cannot remove header %q: Envoy does not allow removing host or :-prefixed pseudo-headers", name))
		}
	}
	return errors.Join(errs...)
}
