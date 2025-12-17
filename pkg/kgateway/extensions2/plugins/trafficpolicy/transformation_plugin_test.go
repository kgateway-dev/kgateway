package trafficpolicy

import (
	"testing"

	transformationpb "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	"github.com/stretchr/testify/assert"
)

func TestTransformationIREquals(t *testing.T) {
	createSimpleTransformation := func() *transformationpb.RouteTransformations {
		return &transformationpb.RouteTransformations{
			Transformations: []*transformationpb.RouteTransformations_RouteTransformation{
				{
					Match: &transformationpb.RouteTransformations_RouteTransformation_RequestMatch_{
						RequestMatch: &transformationpb.RouteTransformations_RouteTransformation_RequestMatch{
							RequestTransformation: &transformationpb.Transformation{
								TransformationType: &transformationpb.Transformation_TransformationTemplate{
									TransformationTemplate: &transformationpb.TransformationTemplate{
										Headers: map[string]*transformationpb.InjaTemplate{
											"x-test": {Text: "test-value"},
										},
									},
								},
							},
						},
					},
				},
			},
		}
	}

	tests := []struct {
		name     string
		trans1   *transformationIR
		trans2   *transformationIR
		expected bool
	}{
		{
			name:     "both nil are equal",
			trans1:   nil,
			trans2:   nil,
			expected: true,
		},
		{
			name:     "nil vs non-nil are not equal",
			trans1:   nil,
			trans2:   &transformationIR{config: createSimpleTransformation()},
			expected: false,
		},
		{
			name:     "non-nil vs nil are not equal",
			trans1:   &transformationIR{config: createSimpleTransformation()},
			trans2:   nil,
			expected: false,
		},
		{
			name:     "same instance is equal",
			trans1:   &transformationIR{config: createSimpleTransformation()},
			trans2:   &transformationIR{config: createSimpleTransformation()},
			expected: true,
		},
		{
			name:     "nil transformation fields are equal",
			trans1:   &transformationIR{config: nil},
			trans2:   &transformationIR{config: nil},
			expected: true,
		},
		{
			name:     "nil vs non-nil transformation fields are not equal",
			trans1:   &transformationIR{config: nil},
			trans2:   &transformationIR{config: createSimpleTransformation()},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.trans1.Equals(tt.trans2)
			assert.Equal(t, tt.expected, result)

			// Test symmetry: a.Equals(b) should equal b.Equals(a)
			reverseResult := tt.trans2.Equals(tt.trans1)
			assert.Equal(t, result, reverseResult, "Equals should be symmetric")
		})
	}

	// Test reflexivity: x.Equals(x) should always be true for non-nil values
	t.Run("reflexivity", func(t *testing.T) {
		transformation := &transformationIR{
			config: &transformationpb.RouteTransformations{
				Transformations: []*transformationpb.RouteTransformations_RouteTransformation{},
			},
		}
		assert.True(t, transformation.Equals(transformation), "transformation should equal itself")
	})

	// Test transitivity: if a.Equals(b) && b.Equals(c), then a.Equals(c)
	t.Run("transitivity", func(t *testing.T) {
		createSameTransformation := func() *transformationIR {
			return &transformationIR{
				config: createSimpleTransformation(),
			}
		}

		a := createSameTransformation()
		b := createSameTransformation()
		c := createSameTransformation()

		assert.True(t, a.Equals(b), "a should equal b")
		assert.True(t, b.Equals(c), "b should equal c")
		assert.True(t, a.Equals(c), "a should equal c (transitivity)")
	})
}
