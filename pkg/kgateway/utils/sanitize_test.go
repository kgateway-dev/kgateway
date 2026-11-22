package utils_test

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	. "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

func TestSanitizeForEnvoy(t *testing.T) {
	tests := []struct {
		name         string
		resourceName string
		resourceType string
		expected     string
	}{
		{
			name:         "no dots remains unchanged",
			resourceName: "my-virtual-host",
			resourceType: "VirtualHost",
			expected:     "my-virtual-host",
		},
		{
			name:         "replaces dots with underscores",
			resourceName: "app.default.svc.cluster.local",
			resourceType: "VirtualHost",
			expected:     "app_default_svc_cluster_local",
		},
		{
			name:         "empty string returns empty",
			resourceName: "",
			resourceType: "VirtualCluster",
			expected:     "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			actual := SanitizeForEnvoy(context.Background(), tc.resourceName, tc.resourceType)
			g.Expect(actual).To(Equal(tc.expected))
		})
	}
}

func TestSanitizeCookieName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty input defaults to cookie",
			input:    "",
			expected: "cookie",
		},
		{
			name:     "valid RFC 6265 token remains unchanged",
			input:    "session.id-1_2",
			expected: "session.id-1_2",
		},
		{
			name:     "replaces RFC 2616 separators and whitespace with underscore",
			input:    "foo(bar)@baz;qux=1/2 name\tval",
			expected: "foo_bar__baz_qux_1_2_name_val",
		},
		{
			name:     "replaces control and non-ASCII characters with underscore",
			input:    "café\n\x00",
			expected: "caf___",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(SanitizeCookieName(tc.input)).To(Equal(tc.expected))
		})
	}
}

func TestSanitizeHeaderName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty input defaults to header",
			input:    "",
			expected: "header",
		},
		{
			name:     "valid RFC 7230 tchar header remains unchanged",
			input:    "X-Custom-Header_1.0~test!#$%&'*+^`|",
			expected: "X-Custom-Header_1.0~test!#$%&'*+^`|",
		},
		{
			name:     "replaces delimiters and spaces with hyphen",
			input:    "X Custom:Header/1@test(v2)",
			expected: "X-Custom-Header-1-test-v2-",
		},
		{
			name:     "replaces non-ASCII characters with hyphen",
			input:    "X-Café",
			expected: "X-Caf-",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(SanitizeHeaderName(tc.input)).To(Equal(tc.expected))
		})
	}
}
