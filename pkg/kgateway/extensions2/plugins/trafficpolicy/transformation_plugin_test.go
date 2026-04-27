package trafficpolicy

import (
	"encoding/json"
	"testing"

	extensiondynamicmodulev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/dynamic_modules/v3"
	dynamicmodulesv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/dynamic_modules/v3"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
)

func TestRustformationIREquals(t *testing.T) {
	stringConf := `{"request":{"set":[{"name":"x-test","value":"text-value"}]}}`
	filterCfg := utils.MustMessageToAny(&wrapperspb.StringValue{
		Value: stringConf,
	})
	createSimpleTransformation := func() *dynamicmodulesv3.DynamicModuleFilterPerRoute {
		return &dynamicmodulesv3.DynamicModuleFilterPerRoute{
			DynamicModuleConfig: &extensiondynamicmodulev3.DynamicModuleConfig{
				Name: RustformationModuleName,
			},
			PerRouteConfigName: RustformationFilterName,
			FilterConfig:       filterCfg,
		}
	}

	tests := []struct {
		name     string
		trans1   *rustformationIR
		trans2   *rustformationIR
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
			trans2:   &rustformationIR{config: createSimpleTransformation()},
			expected: false,
		},
		{
			name:     "non-nil vs nil are not equal",
			trans1:   &rustformationIR{config: createSimpleTransformation()},
			trans2:   nil,
			expected: false,
		},
		{
			name:     "same instance is equal",
			trans1:   &rustformationIR{config: createSimpleTransformation()},
			trans2:   &rustformationIR{config: createSimpleTransformation()},
			expected: true,
		},
		{
			name:     "nil transformation fields are equal",
			trans1:   &rustformationIR{config: nil},
			trans2:   &rustformationIR{config: nil},
			expected: true,
		},
		{
			name:     "nil vs non-nil transformation fields are not equal",
			trans1:   &rustformationIR{config: nil},
			trans2:   &rustformationIR{config: createSimpleTransformation()},
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
		transformation := &rustformationIR{
			config: &dynamicmodulesv3.DynamicModuleFilterPerRoute{
				DynamicModuleConfig: &extensiondynamicmodulev3.DynamicModuleConfig{
					Name: RustformationModuleName,
				},
				PerRouteConfigName: RustformationFilterName,
			},
		}
		assert.True(t, transformation.Equals(transformation), "transformation should equal itself")
	})

	// Test transitivity: if a.Equals(b) && b.Equals(c), then a.Equals(c)
	t.Run("transitivity", func(t *testing.T) {
		createSameTransformation := func() *rustformationIR {
			return &rustformationIR{
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

func TestGenerateDynamicMetadataStableOrderingAndValues(t *testing.T) {
	config := generateDynamicMetadata("test-ns", map[string]kgateway.InjaTemplate{
		"z-key": "z-value",
		"a-key": "a-value",
		"m-key": "m-value",
	})

	var filterConfig wrapperspb.StringValue
	assert.NoError(t, config.FilterConfig.UnmarshalTo(&filterConfig))

	var policy kgateway.TransformationPolicy
	assert.NoError(t, json.Unmarshal([]byte(filterConfig.Value), &policy))

	expectedKeys := []string{"a-key", "m-key", "z-key"}
	expectedValues := []kgateway.InjaTemplate{"a-value", "m-value", "z-value"}

	assert.Len(t, policy.Request.DynamicMetadata, len(expectedKeys))

	for i, expectedKey := range expectedKeys {
		requestMetadata := policy.Request.DynamicMetadata[i]

		assert.Equal(t, expectedKey, requestMetadata.Key)
		assert.Equal(t, expectedValues[i], *requestMetadata.Value.StringValue)
	}
}
