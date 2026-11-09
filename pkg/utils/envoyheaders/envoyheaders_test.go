package envoyheaders

import (
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsMutable(t *testing.T) {
	for name, want := range map[string]bool{
		"host":             false,
		"Host":             false,
		"hOsT":             false,
		":authority":       false,
		":path":            false,
		":status":          false,
		"X-Forwarded-Host": true,
		"hosts":            true,
		"x-test":           true,
	} {
		assert.Equal(t, want, IsMutable(name), name)
	}
}

func TestValidateValue(t *testing.T) {
	valid := []string{
		"",
		"plain",
		"100%%",
		"%%%%",
		"%PROTOCOL%",
		"%REQ(:AUTHORITY)%",
		"%REQ(x-id):10%",
		"%START_TIME(%s.%3f)%",
		"prefix-%DOWNSTREAM_REMOTE_ADDRESS%-suffix",
		"%REQ(a)%%REQ(b)%",
		"%UPSTREAM_METADATA([\"ns\", \"key\"])%",
		"tab\tseparated",
		// Well-formed, but Envoy rejects them for command-specific reasons
		// (unknown command; REQ argument is not a header name). These depend
		// on the Envoy build and are left to strict-mode validation.
		"%NOT_A_COMMAND%",
		"%REQ(a)b)%",
	}
	for _, value := range valid {
		assert.NoError(t, ValidateValue(value), "%q", value)
	}

	invalid := map[string]string{
		"%REQ(Correlation-Id":           "position 0",
		"%RESPONSE(Invalid-Variable)":   "position 0",
		"100%":                          "position 3",
		"%":                             "position 0",
		"a%b":                           "position 1",
		"%lowercase%":                   "position 0",
		"%REQ(x):%":                     "position 0",
		"%REQ(x):99999999999999999999%": "out-of-range",
		"ok %PROTOCOL% then % bad":      "position 19",
		"line\nbreak":                   "position 4",
		"carriage\rreturn":              "position 8",
		"nul\x00byte":                   "position 3",
		"%REQ(a\n)%":                    "position 6",
	}
	for value, want := range invalid {
		err := ValidateValue(value)
		require.ErrorContains(t, err, want, "%q", value)
		if len(value) > 3 {
			assert.NotContains(t, err.Error(), value, "error must not echo the value, which may be a Secret")
		}
	}
}

func TestValidateLiteral(t *testing.T) {
	require.NoError(t, ValidateLiteral("100%"), "a literal is used verbatim, so % is fine")
	require.ErrorContains(t, ValidateLiteral("a\r\nb"), "position 1")
}

func TestValidateMutations(t *testing.T) {
	option := func(name, value string) *envoycorev3.HeaderValueOption {
		return &envoycorev3.HeaderValueOption{Header: &envoycorev3.HeaderValue{Key: name, Value: value}}
	}
	require.NoError(t, ValidateMutations(
		[]*envoycorev3.HeaderValueOption{option("x-a", "%REQ(x)%"), option("x-b", "b")},
		[]string{"x-c"},
	))

	err := ValidateMutations(
		[]*envoycorev3.HeaderValueOption{option("Host", "a"), option("x-bad", "%oops"), option(":path", "/")},
		[]string{":authority", "host", "x-ok"},
	)
	require.Error(t, err)
	for _, want := range []string{
		`cannot set header "Host"`,
		`invalid value for header "x-bad": no valid substitution command at position 0`,
		`cannot set header ":path"`,
		`cannot remove header ":authority"`,
		`cannot remove header "host"`,
	} {
		assert.ErrorContains(t, err, want)
	}
	assert.NotContains(t, err.Error(), "x-ok")
}
