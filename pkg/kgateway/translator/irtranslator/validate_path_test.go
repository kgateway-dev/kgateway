package irtranslator

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
)

// The regex and url.Parse implementations of ValidateRoutePath, ValidatePrefixRewrite and
// isValidPathSeparated, kept as oracles: the byte loops that replaced them must agree with
// them on every input, down to the error value and message.
var (
	oracleValidPathRegex = regexp.MustCompile(validPathRegexCharacters)
	// Envoy's own validation pattern for path_separated_prefix.
	oracleSeparatedPathRegex = regexp.MustCompile("^[^?#]+[^?#/]$")
)

func oracleValidateRoutePath(s string) error {
	if s == "" {
		return nil
	}
	if !oracleValidPathRegex.MatchString(s) {
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

func oracleValidatePrefixRewrite(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return err
	}
	return oracleValidateRoutePath(u.Path)
}

var pathValidationInputs = []string{
	"",
	"/",
	"/foo",
	"/foo/bar",
	"/api/v1/service-123/items",
	"/a-b_c.d~e",
	"/:@!$&'()*+,;=",
	"/foo:bar@baz",
	"/%20",
	"/foo%41bar",
	"/%2f",
	"/%2F",
	"/%25",
	"/%zz",
	"/%2",
	"/%",
	"%",
	"%41",
	"/foo%",
	"/foo%a",
	"/foo%%41",
	"/foo//bar",
	"//",
	"//foo",
	"//foo/bar",
	"///foo",
	"/./",
	"/../",
	"/foo/./bar",
	"/foo/../bar",
	"/foo/..",
	"/foo/.",
	"/.",
	"/..",
	"/foo#bar",
	"/foo#",
	"#",
	"#frag",
	"/foo?bar",
	"/foo?",
	"/foo?x?y",
	"/foo?a=b#c",
	"?",
	"?q",
	"/foo bar",
	"/é",
	"/é/",
	"/\xff",
	"/foo\n",
	"/\x00",
	"/\x7f",
	"/foo\t",
	"*",
	"foo",
	"foo/bar",
	"foo:bar",
	"a:b/c",
	":foo",
	"1abc:",
	"http://host/path",
	"http://host/%2f",
	"http:/path",
	"http:path",
	"HTTP://Host",
	"mailto:x",
	"a",
	"ab",
	"a/",
	"é",
	"éa",
	"aé",
	"\xff",
	"\xff\xfe",
	"\xe9/",
	"/x",
	"/x/",
	"/x?",
	"/x#",
}

// requireSameError fails unless got and want are both nil, or have the same type and
// message and agree on being ValidRoutePatternError.
func requireSameError(t *testing.T, fn, input string, got, want error) {
	t.Helper()
	if got == nil && want == nil {
		return
	}
	if got == nil || want == nil ||
		fmt.Sprintf("%T", got) != fmt.Sprintf("%T", want) ||
		got.Error() != want.Error() ||
		errors.Is(got, ValidRoutePatternError) != errors.Is(want, ValidRoutePatternError) {
		t.Fatalf("%s(%q) = %v, oracle returned %v", fn, input, got, want)
	}
}

func checkPathValidationAgainstOracles(t *testing.T, s string) {
	t.Helper()
	requireSameError(t, "ValidateRoutePath", s, ValidateRoutePath(s), oracleValidateRoutePath(s))
	requireSameError(t, "ValidatePrefixRewrite", s, ValidatePrefixRewrite(s), oracleValidatePrefixRewrite(s))
	if got, want := isValidPathSeparated(s), oracleSeparatedPathRegex.MatchString(s); got != want {
		t.Fatalf("isValidPathSeparated(%q) = %t, oracle returned %t", s, got, want)
	}
}

func TestPathValidationMatchesOracles(t *testing.T) {
	for _, s := range pathValidationInputs {
		checkPathValidationAgainstOracles(t, s)
	}
	// Every single byte and every two-byte string covers each character class, escape
	// boundary and the separated prefix's two-character minimum.
	for i := range 256 {
		checkPathValidationAgainstOracles(t, string([]byte{byte(i)}))
		for j := range 256 {
			checkPathValidationAgainstOracles(t, string([]byte{byte(i), byte(j)}))
		}
	}
}

func FuzzPathValidationMatchesOracles(f *testing.F) {
	for _, s := range pathValidationInputs {
		f.Add(s)
	}
	f.Fuzz(checkPathValidationAgainstOracles)
}

func BenchmarkValidateRoutePath(b *testing.B) {
	for _, tc := range []struct{ name, path string }{
		{"root", "/"},
		{"plain", "/api/v1/service-123/items"},
		{"escaped", "/api/%7Euser/items"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				_ = ValidateRoutePath(tc.path)
			}
		})
	}
}

func BenchmarkValidatePrefixRewrite(b *testing.B) {
	for _, tc := range []struct{ name, rewrite string }{
		{"empty", ""},
		{"root", "/"},
		{"plain", "/rewritten/prefix"},
		{"escaped", "/rewritten/%7Euser"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				_ = ValidatePrefixRewrite(tc.rewrite)
			}
		})
	}
}

func BenchmarkIsValidPathSeparated(b *testing.B) {
	for b.Loop() {
		_ = isValidPathSeparated("/api/v1/service-123")
	}
}

// BenchmarkValidateRoutePreEnvoy measures the static checks every translated route pays:
// a PathPrefix match with a prefix rewrite, as an HTTPRoute with a URLRewrite produces.
func BenchmarkValidateRoutePreEnvoy(b *testing.B) {
	route := &envoyroutev3.Route{
		Name: "r",
		Match: &envoyroutev3.RouteMatch{
			PathSpecifier: &envoyroutev3.RouteMatch_PathSeparatedPrefix{PathSeparatedPrefix: "/api/v1/service-123"},
		},
		Action: &envoyroutev3.Route_Route{
			Route: &envoyroutev3.RouteAction{
				ClusterSpecifier: &envoyroutev3.RouteAction_Cluster{Cluster: "c"},
				PrefixRewrite:    "/",
			},
		},
	}
	for b.Loop() {
		if err := validateRoutePreEnvoy(route, apisettings.ValidationStrict); err != nil {
			b.Fatal(err)
		}
	}
}
