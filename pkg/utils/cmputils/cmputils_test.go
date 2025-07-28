package cmputils

import (
	"testing"
)

type foo struct {
	bar int
}

func TestOnlyOneNil(t *testing.T) {
	f1 := &foo{bar: 1}
	f2 := &foo{bar: 2}

	tests := []struct {
		name     string
		a        *foo
		b        *foo
		expected bool
	}{
		{
			name:     "both nil",
			a:        nil,
			b:        nil,
			expected: false,
		},
		{
			name:     "both not nil",
			a:        f1,
			b:        f2,
			expected: false,
		},
		{
			name:     "first nil, second not nil",
			a:        nil,
			b:        f1,
			expected: true,
		},
		{
			name:     "first not nil, second nil",
			a:        f1,
			b:        nil,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := OnlyOneNil(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("OnlyOneNil(%v, %v) = %v, want %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

// Common test cases for comparison functions
var comparisonTestCases = []struct {
	name     string
	a        *foo
	b        *foo
	expected bool
}{
	{
		name:     "both nil",
		a:        nil,
		b:        nil,
		expected: true,
	},
	{
		name:     "first nil, second not nil",
		a:        nil,
		b:        &foo{bar: 1},
		expected: false,
	},
	{
		name:     "first not nil, second nil",
		a:        &foo{bar: 1},
		b:        nil,
		expected: false,
	},
	{
		name:     "both not nil, equal values",
		a:        &foo{bar: 1},
		b:        &foo{bar: 1},
		expected: true,
	},
	{
		name:     "both not nil, different values",
		a:        &foo{bar: 1},
		b:        &foo{bar: 2},
		expected: false,
	},
}

func TestCompareWithNils(t *testing.T) {
	// Comparison function for foo structs
	fooCompare := func(a, b *foo) bool {
		return a.bar == b.bar
	}

	for _, tt := range comparisonTestCases {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareWithNils(tt.a, tt.b, fooCompare)
			if result != tt.expected {
				t.Errorf("CompareWithNils(%v, %v, fooCompare) = %v, want %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestPointerValsEqualWithNils(t *testing.T) {
	for _, tt := range comparisonTestCases {
		t.Run(tt.name, func(t *testing.T) {
			result := PointerValsEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("PointerValsEqual(%v, %v) = %v, want %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}
