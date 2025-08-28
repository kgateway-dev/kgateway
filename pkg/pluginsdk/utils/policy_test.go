package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	apiannotations "github.com/kgateway-dev/kgateway/v2/api/annotations"
)

func TestParsePrecedenceWeightAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    int32
		expectError bool
	}{
		{
			name:        "No annotation",
			annotations: map[string]string{},
			expected:    0,
			expectError: false,
		},
		{
			name: "Valid positive weight",
			annotations: map[string]string{
				apiannotations.RoutePrecedenceWeight: "100",
			},
			expected:    100,
			expectError: false,
		},
		{
			name: "Valid negative weight",
			annotations: map[string]string{
				apiannotations.RoutePrecedenceWeight: "-50",
			},
			expected:    -50,
			expectError: false,
		},
		{
			name: "Valid zero weight",
			annotations: map[string]string{
				apiannotations.RoutePrecedenceWeight: "0",
			},
			expected:    0,
			expectError: false,
		},
		{
			name: "Invalid non-numeric value",
			annotations: map[string]string{
				apiannotations.RoutePrecedenceWeight: "invalid",
			},
			expected:    0,
			expectError: true,
		},
		{
			name: "Invalid decimal value",
			annotations: map[string]string{
				apiannotations.RoutePrecedenceWeight: "100.5",
			},
			expected:    0,
			expectError: true,
		},
		{
			name: "Empty string value",
			annotations: map[string]string{
				apiannotations.RoutePrecedenceWeight: "",
			},
			expected:    0,
			expectError: true,
		},
		{
			name: "Value too large for int32",
			annotations: map[string]string{
				apiannotations.RoutePrecedenceWeight: "2147483648", // int32 max + 1
			},
			expected:    0,
			expectError: true,
		},
		{
			name: "Value too small for int32",
			annotations: map[string]string{
				apiannotations.RoutePrecedenceWeight: "-2147483649", // int32 min - 1
			},
			expected:    0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)

			weight, err := ParsePrecedenceWeightAnnotation(tt.annotations, apiannotations.RoutePrecedenceWeight)

			if tt.expectError {
				a.Error(err)
				a.Equal(int32(0), weight)
				return
			}
			a.NoError(err)
			a.Equal(tt.expected, weight)
		})
	}
}
