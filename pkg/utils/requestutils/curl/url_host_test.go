package curl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestURLHost(t *testing.T) {
	for name, tc := range map[string]struct {
		host string
		want string
	}{
		"ipv4 literal":      {host: "10.0.0.1", want: "10.0.0.1"},
		"hostname":          {host: "example.com", want: "example.com"},
		"ipv6 literal":      {host: "fc00:f853:ccd:e793::1", want: "[fc00:f853:ccd:e793::1]"},
		"ipv6 loopback":     {host: "::1", want: "[::1]"},
		"already bracketed": {host: "[::1]", want: "[::1]"},
		// A v4-mapped v6 address is written with colons but parses as IPv4; curl
		// takes it either way, and leaving it alone keeps the output stable.
		"ipv4 mapped": {host: "::ffff:10.0.0.1", want: "::ffff:10.0.0.1"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, urlHost(tc.host))
		})
	}
}

// An unbracketed IPv6 literal produces a URL curl reads as host "fc00" with a
// garbage port, so the gateway address in an IPv6 cluster has to be bracketed.
func TestBuildArgsBracketsIPv6Host(t *testing.T) {
	args := BuildArgs(WithHost("fc00:f853:ccd:e793::1"), WithPort(8080), WithPath("/status"))
	assert.Contains(t, args, "http://[fc00:f853:ccd:e793::1]:8080/status")
}

func TestBuildArgsBracketsIPv6ConnectTo(t *testing.T) {
	args := BuildArgs(WithHost("fc00:f853:ccd:e793::1"), WithPort(8443), WithSni("example.com"))
	assert.Contains(t, args, "example.com:8443:[fc00:f853:ccd:e793::1]:8443")
}

// Splitting on ":" gives more than two parts for an IPv6 host:port, which used to
// silently leave the request pointed at host "unset" on port 0.
func TestWithHostPort(t *testing.T) {
	for name, tc := range map[string]struct {
		hostPort string
		wantHost string
		wantPort int
	}{
		"ipv4":           {hostPort: "10.0.0.1:8080", wantHost: "10.0.0.1", wantPort: 8080},
		"hostname":       {hostPort: "example.com:8080", wantHost: "example.com", wantPort: 8080},
		"bracketed ipv6": {hostPort: "[fc00:f853:ccd:e793::1]:8080", wantHost: "fc00:f853:ccd:e793::1", wantPort: 8080},
		"missing port":   {hostPort: "10.0.0.1", wantHost: "unset", wantPort: 0},
		"unbracketed v6": {hostPort: "fc00:f853:ccd:e793::1", wantHost: "unset", wantPort: 0},
	} {
		t.Run(name, func(t *testing.T) {
			config := &requestConfig{}
			WithHostPort(tc.hostPort)(config)
			assert.Equal(t, tc.wantHost, config.host)
			assert.Equal(t, tc.wantPort, config.port)
		})
	}
}
