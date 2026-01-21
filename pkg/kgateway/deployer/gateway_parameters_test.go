package deployer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestGenerateHelmReleaseName(t *testing.T) {
	tests := []struct {
		name         string
		gatewayName  string
		namespace    string
		expectTrunc  bool
		expectValid  bool
	}{
		{
			name:        "short valid name",
			gatewayName: "my-gateway",
			namespace:   "default",
			expectTrunc: false,
			expectValid: true,
		},
		{
			name:        "exactly 53 chars",
			gatewayName: strings.Repeat("a", 53),
			namespace:   "default",
			expectTrunc: false,
			expectValid: true,
		},
		{
			name:        "long name requiring truncation",
			gatewayName: "looooooooooooooooooooooooooooooooooooooooooooooooooooong-name",
			namespace:   "default",
			expectTrunc: true,
			expectValid: true,
		},
		{
			name:        "very long name",
			gatewayName: strings.Repeat("very-long-gateway-name-", 10),
			namespace:   "production",
			expectTrunc: true,
			expectValid: true,
		},
		{
			name:        "name with uppercase (should be handled)",
			gatewayName: "My-Gateway-Name",
			namespace:   "default",
			expectTrunc: false,
			expectValid: true,
		},
		{
			name:        "name ending with hyphen",
			gatewayName: "gateway-name-",
			namespace:   "default",
			expectTrunc: false,
			expectValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateHelmReleaseName(tt.gatewayName, tt.namespace)

			// Validate the result
			assert.True(t, isValidHelmReleaseName(result), "Generated name should be valid: %s", result)
			assert.LessOrEqual(t, len(result), maxHelmReleaseNameLength, "Generated name should not exceed max length")

			if tt.expectTrunc {
				assert.NotEqual(t, tt.gatewayName, result, "Long name should be truncated")
				assert.Contains(t, result, "-", "Truncated name should contain hash separator")
			}

			// Test determinism - same input should produce same output
			result2 := generateHelmReleaseName(tt.gatewayName, tt.namespace)
			assert.Equal(t, result, result2, "Function should be deterministic")
		})
	}
}

func TestGenerateHelmReleaseNameUniqueness(t *testing.T) {
	// Test that different gateways with similar names produce different release names
	testCases := []struct {
		gatewayName string
		namespace   string
	}{
		{"looooooooooooooooooooooooooooooooooooooooooooooooooooong-name-1", "default"},
		{"looooooooooooooooooooooooooooooooooooooooooooooooooooong-name-2", "default"},
		{"looooooooooooooooooooooooooooooooooooooooooooooooooooong-name-1", "production"},
	}

	results := make(map[string]bool)
	for _, tc := range testCases {
		result := generateHelmReleaseName(tc.gatewayName, tc.namespace)
		assert.False(t, results[result], "Each gateway should produce a unique release name: %s", result)
		results[result] = true
	}
}

func TestIsValidHelmReleaseName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid simple name", "gateway", true},
		{"valid with hyphens", "my-gateway-name", true},
		{"valid with numbers", "gateway-123", true},
		{"valid with dots", "gateway.example.com", true},
		{"too long", strings.Repeat("a", 54), false},
		{"starts with hyphen", "-gateway", false},
		{"ends with hyphen", "gateway-", false},
		{"contains uppercase", "Gateway", false},
		{"contains underscore", "my_gateway", false},
		{"empty string", "", false},
		{"single character", "a", true},
		{"exactly max length", strings.Repeat("a", 53), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidHelmReleaseName(tt.input)
			assert.Equal(t, tt.expected, result, "Validation result for '%s'", tt.input)
		})
	}
}

func TestGatewayReleaseNameAndNamespace(t *testing.T) {
	// Test the main function that integrates with the Gateway object
	gateway := &gwv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "looooooooooooooooooooooooooooooooooooooooooooooooooooong-name",
			Namespace: "default",
		},
	}

	releaseName, namespace := GatewayReleaseNameAndNamespace(gateway)

	assert.Equal(t, "default", namespace, "Namespace should be preserved")
	assert.True(t, isValidHelmReleaseName(releaseName), "Release name should be valid")
	assert.LessOrEqual(t, len(releaseName), maxHelmReleaseNameLength, "Release name should not exceed max length")
}

func TestHelmReleaseNameRegex(t *testing.T) {
	// Test that our regex matches Helm's actual validation
	validNames := []string{
		"a",
		"gateway",
		"my-gateway",
		"gateway-123",
		"a.b.c",
		"gateway.example.com",
		strings.Repeat("a", 53),
	}

	invalidNames := []string{
		"",
		"-gateway",
		"gateway-",
		"Gateway",
		"my_gateway",
		"gateway..name",
		"gateway.-name",
		strings.Repeat("a", 54),
	}

	for _, name := range validNames {
		t.Run("valid_"+name, func(t *testing.T) {
			assert.True(t, helmReleaseNameRegex.MatchString(name), "Should match valid name: %s", name)
		})
	}

	for _, name := range invalidNames {
		t.Run("invalid_"+name, func(t *testing.T) {
			if len(name) <= maxHelmReleaseNameLength { // Only test regex for names within length limit
				assert.False(t, helmReleaseNameRegex.MatchString(name), "Should not match invalid name: %s", name)
			}
		})
	}
}

// Benchmark the name generation function
func BenchmarkGenerateHelmReleaseName(b *testing.B) {
	gatewayName := "looooooooooooooooooooooooooooooooooooooooooooooooooooong-name"
	namespace := "default"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		generateHelmReleaseName(gatewayName, namespace)
	}
}