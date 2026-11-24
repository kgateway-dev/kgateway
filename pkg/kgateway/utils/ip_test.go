package utils_test

import (
	"testing"

	. "github.com/onsi/gomega"

	. "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

func TestIsIpv4Address(t *testing.T) {
	tests := []struct {
		name         string
		bindAddress  string
		expectValid  bool
		expectStrict bool
		expectErr    bool
	}{
		{
			name:         "pure IPv4 loopback address",
			bindAddress:  "127.0.0.1",
			expectValid:  true,
			expectStrict: true,
			expectErr:    false,
		},
		{
			name:         "pure IPv4 wildcard address",
			bindAddress:  "0.0.0.0",
			expectValid:  true,
			expectStrict: true,
			expectErr:    false,
		},
		{
			name:         "IPv4-mapped IPv6 address is valid IPv4 but not strict",
			bindAddress:  "::ffff:192.0.2.128",
			expectValid:  true,
			expectStrict: false,
			expectErr:    false,
		},
		{
			name:         "pure IPv6 loopback address",
			bindAddress:  "::1",
			expectValid:  false,
			expectStrict: false,
			expectErr:    false,
		},
		{
			name:         "pure IPv6 address",
			bindAddress:  "2001:db8::1",
			expectValid:  false,
			expectStrict: false,
			expectErr:    false,
		},
		{
			name:         "invalid IP string returns error",
			bindAddress:  "not-an-ip",
			expectValid:  false,
			expectStrict: false,
			expectErr:    true,
		},
		{
			name:         "empty string returns error",
			bindAddress:  "",
			expectValid:  false,
			expectStrict: false,
			expectErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			validIpv4, strictIPv4, err := IsIpv4Address(tc.bindAddress)
			if tc.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(validIpv4).To(Equal(tc.expectValid))
			g.Expect(strictIPv4).To(Equal(tc.expectStrict))
		})
	}
}
