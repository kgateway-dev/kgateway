package main

import "testing"

func TestHealthCheckAddress(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want string
	}{
		{name: "unset defaults to loopback:8234", env: "", want: "127.0.0.1:8234"},
		{name: "loopback bind dialed as-is", env: "127.0.0.1:8234", want: "127.0.0.1:8234"},
		{name: "wildcard bind dialed over loopback", env: "0.0.0.0:8234", want: "127.0.0.1:8234"},
		{name: "custom port preserved", env: "0.0.0.0:9999", want: "127.0.0.1:9999"},
		{name: "ipv6 wildcard bind", env: "[::]:8234", want: "127.0.0.1:8234"},
		{name: "malformed falls back to default port", env: "not-an-address", want: "127.0.0.1:8234"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := healthCheckAddress(tc.env); got != tc.want {
				t.Fatalf("healthCheckAddress(%q) = %q, want %q", tc.env, got, tc.want)
			}
		})
	}
}
