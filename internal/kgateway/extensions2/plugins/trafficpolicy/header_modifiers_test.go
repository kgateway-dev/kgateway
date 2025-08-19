package trafficpolicy

import (
	"testing"

	mutation_rulesv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	header_mutationv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/header_mutation/v3"
	"github.com/stretchr/testify/assert"
)

// Helper to create a simple header mutations filter for testing.
func testHeaderMutation(isAppend bool) *header_mutationv3.HeaderMutationPerRoute {
	appendAction := envoycorev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD
	if isAppend {
		appendAction = envoycorev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD
	}

	return &header_mutationv3.HeaderMutationPerRoute{
		Mutations: &header_mutationv3.Mutations{
			RequestMutations: []*mutation_rulesv3.HeaderMutation{{
				Action: &mutation_rulesv3.HeaderMutation_Append{
					Append: &envoycorev3.HeaderValueOption{
						Header: &envoycorev3.HeaderValue{
							Key:   "x-test-request",
							Value: "test-request",
						},
						AppendAction: appendAction,
					},
				},
			}},
			ResponseMutations: []*mutation_rulesv3.HeaderMutation{{
				Action: &mutation_rulesv3.HeaderMutation_Append{
					Append: &envoycorev3.HeaderValueOption{
						Header: &envoycorev3.HeaderValue{
							Key:   "x-test-response",
							Value: "test-response",
						},
						AppendAction: appendAction,
					},
				},
			}},
		},
	}
}

func TestHeaderModifiersIREquals(t *testing.T) {
	tests := []struct {
		name             string
		headerModifiers1 *headerModifiersIR
		headerModifiers2 *headerModifiersIR
		expected         bool
	}{
		{
			name:             "both nil are equal",
			headerModifiers1: nil,
			headerModifiers2: nil,
			expected:         true,
		},
		{
			name:             "nil vs non-nil are not equal",
			headerModifiers1: nil,
			headerModifiers2: &headerModifiersIR{policy: testHeaderMutation(false)},
			expected:         false,
		},
		{
			name:             "non-nil vs nil are not equal",
			headerModifiers1: &headerModifiersIR{policy: testHeaderMutation(false)},
			headerModifiers2: nil,
			expected:         false,
		},
		{
			name:             "identical instance is equal",
			headerModifiers1: &headerModifiersIR{policy: testHeaderMutation(false)},
			headerModifiers2: &headerModifiersIR{policy: testHeaderMutation(false)},
			expected:         true,
		},
		{
			name:             "different append settings are not equal",
			headerModifiers1: &headerModifiersIR{policy: testHeaderMutation(true)},
			headerModifiers2: &headerModifiersIR{policy: testHeaderMutation(false)},
			expected:         false,
		},
		{
			name:             "nil HeaderModifiers fields are equal",
			headerModifiers1: &headerModifiersIR{policy: nil},
			headerModifiers2: &headerModifiersIR{policy: nil},
			expected:         true,
		},
		{
			name:             "nil vs non-nil HeaderModifiers fields are not equal",
			headerModifiers1: &headerModifiersIR{policy: nil},
			headerModifiers2: &headerModifiersIR{policy: testHeaderMutation(false)},
			expected:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.headerModifiers1.Equals(tt.headerModifiers2)
			assert.Equal(t, tt.expected, result)

			reverseResult := tt.headerModifiers2.Equals(tt.headerModifiers1)
			assert.Equal(t, result, reverseResult, "Equals should be symmetric")
		})
	}
}

func TestHeaderModifiersIRValidate(t *testing.T) {
	tests := []struct {
		name            string
		headerModifiers *headerModifiersIR
		wantErr         bool
	}{
		{
			name:            "nil headerModifiers is valid",
			headerModifiers: nil,
			wantErr:         false,
		},
		{
			name:            "headerModifiers with nil config is valid",
			headerModifiers: &headerModifiersIR{policy: nil},
			wantErr:         false,
		},
		{
			name: "valid headerModifiers config passes validation",
			headerModifiers: &headerModifiersIR{
				policy: testHeaderMutation(false),
			},
			wantErr: false,
		},
		{
			name: "invalid headerModifiers config fails validation",
			headerModifiers: &headerModifiersIR{
				policy: &header_mutationv3.HeaderMutationPerRoute{
					Mutations: &header_mutationv3.Mutations{
						RequestMutations: []*mutation_rulesv3.HeaderMutation{{
							Action: &mutation_rulesv3.HeaderMutation_Append{
								Append: &envoycorev3.HeaderValueOption{},
							},
						}},
						ResponseMutations: []*mutation_rulesv3.HeaderMutation{},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.headerModifiers.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
