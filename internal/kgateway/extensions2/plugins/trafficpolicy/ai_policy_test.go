package trafficpolicy

import (
	"os"
	"testing"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	envoytransformation "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

func TestProcessAITrafficPolicy(t *testing.T) {
	// extproc config from backend plugin
	backendExtprocSettings := &envoy_ext_proc_v3.ExtProcPerRoute{
		Override: &envoy_ext_proc_v3.ExtProcPerRoute_Overrides{
			Overrides: &envoy_ext_proc_v3.ExtProcOverrides{
				GrpcInitialMetadata: []*envoy_config_core_v3.HeaderValue{},
			},
		},
	}
	typedFilterConfig := ir.TypedFilterConfigMap(map[string]proto.Message{
		wellknown.AIExtProcFilterName: backendExtprocSettings,
	})

	t.Run("sets streaming header for chat streaming route", func(t *testing.T) {
		// Setup
		plugin := &trafficPolicyPluginGwPass{}
		chatStreamingType := v1alpha1.CHAT_STREAMING
		aiConfig := &v1alpha1.AIPolicy{
			RouteType: &chatStreamingType,
		}
		// extproc and transformation will be set by preProcessAITrafficPolicy
		aiSecret := &ir.Secret{}
		aiIR := &AIPolicyIR{
			AISecret: aiSecret,
		}

		// Execute
		err := preProcessAITrafficPolicy(aiConfig, aiIR)
		require.NoError(t, err)
		plugin.processAITrafficPolicy(&typedFilterConfig, aiIR)

		// Verify streaming header was added
		extprocSettingsPostPlugin := typedFilterConfig.GetTypedConfig(wellknown.AIExtProcFilterName).(*envoy_ext_proc_v3.ExtProcPerRoute)
		found := false
		for _, header := range extprocSettingsPostPlugin.GetOverrides().GrpcInitialMetadata {
			if header.Key == "x-chat-streaming" && header.Value == "true" {
				found = true
				break
			}
		}
		assert.True(t, found, "streaming header not found")

		// Verify transformation and extproc were added to context
		transformation, ok := typedFilterConfig.GetTypedConfig(wellknown.AIPolicyTransformationFilterName).(*envoytransformation.RouteTransformations)
		assert.True(t, ok)
		assert.NotNil(t, transformation)
	})

	t.Run("sets debug logging when environment variable is set", func(t *testing.T) {
		// Setup
		plugin := &trafficPolicyPluginGwPass{}
		aiConfig := &v1alpha1.AIPolicy{}
		// extproc and transformation will be set by preProcessAITrafficPolicy
		aiSecret := &ir.Secret{}
		aiIR := &AIPolicyIR{
			AISecret: aiSecret,
		}

		// Set env var
		oldEnv := os.Getenv(AiDebugTransformations)
		os.Setenv(AiDebugTransformations, "true")
		defer os.Setenv(AiDebugTransformations, oldEnv)

		// Execute
		err := preProcessAITrafficPolicy(aiConfig, aiIR)
		require.NoError(t, err)

		plugin.processAITrafficPolicy(&typedFilterConfig, aiIR)

		// Verify
		require.NoError(t, err)
		transformation, ok := typedFilterConfig.GetTypedConfig(wellknown.AIPolicyTransformationFilterName).(*envoytransformation.RouteTransformations)
		assert.True(t, ok)
		assert.True(t, len(transformation.Transformations) == 1)
		assert.True(t, transformation.Transformations[0].GetRequestMatch().GetRequestTransformation().GetLogRequestResponseInfo().GetValue())
	})

	t.Run("applies defaults and prompt enrichment", func(t *testing.T) {
		// Setup
		plugin := &trafficPolicyPluginGwPass{}
		aiConfig := &v1alpha1.AIPolicy{
			Defaults: []v1alpha1.FieldDefault{
				{
					Field: "model",
					Value: "gpt-4",
				},
			},
			PromptEnrichment: &v1alpha1.AIPromptEnrichment{
				Prepend: []v1alpha1.Message{
					{
						Role:    "system",
						Content: "You are a helpful assistant",
					},
				},
			},
		}
		// extproc and transformation will be set by preProcessAITrafficPolicy
		aiSecret := &ir.Secret{}
		aiIR := &AIPolicyIR{
			AISecret: aiSecret,
		}
		// Execute
		err := preProcessAITrafficPolicy(aiConfig, aiIR)
		require.NoError(t, err)

		plugin.processAITrafficPolicy(&typedFilterConfig, aiIR)

		routeTransformations, ok := typedFilterConfig.GetTypedConfig(wellknown.AIPolicyTransformationFilterName).(*envoytransformation.RouteTransformations)
		assert.True(t, ok)
		assert.True(t, len(routeTransformations.Transformations) == 1)
		transformation := routeTransformations.Transformations[0]

		// Check the model field was set in the transformation
		modelTemplate := transformation.GetRequestMatch().GetRequestTransformation().GetTransformationTemplate().GetMergeJsonKeys().GetJsonKeys()["model"]
		assert.NotNil(t, modelTemplate)
		assert.Contains(t, modelTemplate.GetTmpl().GetText(), "gpt-4")

		// Check the messages field contains the system message
		messagesTemplate := transformation.GetRequestMatch().GetRequestTransformation().GetTransformationTemplate().GetMergeJsonKeys().GetJsonKeys()["messages"]
		assert.NotNil(t, messagesTemplate)
		assert.Contains(t, messagesTemplate.GetTmpl().GetText(), "You are a helpful assistant")
		assert.Contains(t, messagesTemplate.GetTmpl().GetText(), "system")
	})

	t.Run("applies prompt guard configuration", func(t *testing.T) {
		// Setup
		plugin := &trafficPolicyPluginGwPass{}
		aiConfig := &v1alpha1.AIPolicy{
			PromptGuard: &v1alpha1.AIPromptGuard{
				Request: &v1alpha1.PromptguardRequest{
					Moderation: &v1alpha1.Moderation{
						OpenAIModeration: &v1alpha1.OpenAIConfig{
							AuthToken: v1alpha1.SingleAuthToken{
								Inline: ptr.To("test-token"),
							},
						},
					},
				},
				Response: &v1alpha1.PromptguardResponse{
					Regex: &v1alpha1.Regex{
						Builtins: []v1alpha1.BuiltIn{v1alpha1.SSN, v1alpha1.PHONE_NUMBER},
					},
				},
			},
		}
		// extproc and transformation will be set by preProcessAITrafficPolicy
		aiSecret := &ir.Secret{}
		aiIR := &AIPolicyIR{
			AISecret: aiSecret,
		}

		// Execute
		err := preProcessAITrafficPolicy(aiConfig, aiIR)
		require.NoError(t, err)

		plugin.processAITrafficPolicy(&typedFilterConfig, aiIR)

		// Check that the guardrails config headers were added
		foundReqConfig := false
		foundReqHash := false
		foundRespConfig := false
		foundRespHash := false

		// Check extproc
		outputExtprocProto := typedFilterConfig.GetTypedConfig(wellknown.AIExtProcFilterName)
		assert.NotNil(t, outputExtprocProto)
		outputExtproc := outputExtprocProto.(*envoy_ext_proc_v3.ExtProcPerRoute)
		for _, header := range outputExtproc.GetOverrides().GrpcInitialMetadata {
			switch header.Key {
			case "x-req-guardrails-config":
				foundReqConfig = true
				assert.Contains(t, header.Value, "openAIModeration")
			case "x-req-guardrails-config-hash":
				foundReqHash = true
			case "x-resp-guardrails-config":
				foundRespConfig = true
				assert.Contains(t, header.Value, "SSN")
				assert.Contains(t, header.Value, "PHONE_NUMBER")
			case "x-resp-guardrails-config-hash":
				foundRespHash = true
			}
		}

		assert.True(t, foundReqConfig, "request guardrails config not found")
		assert.True(t, foundReqHash, "request guardrails hash not found")
		assert.True(t, foundRespConfig, "response guardrails config not found")
		assert.True(t, foundRespHash, "response guardrails hash not found")

		// Check transformation
		outputTransformationProto := typedFilterConfig.GetTypedConfig(wellknown.AIPolicyTransformationFilterName)
		assert.NotNil(t, outputTransformationProto)
		outputTransformation := outputTransformationProto.(*envoytransformation.RouteTransformations)
		assert.Len(t, outputTransformation.Transformations, 1)
	})

	t.Run("handles error from prompt guard", func(t *testing.T) {
		// Setup
		aiConfig := &v1alpha1.AIPolicy{
			PromptGuard: &v1alpha1.AIPromptGuard{
				Request: &v1alpha1.PromptguardRequest{
					Moderation: &v1alpha1.Moderation{
						// missing config
					},
				},
			},
		}
		// extproc and transformation will be set by preProcessAITrafficPolicy
		aiSecret := &ir.Secret{}
		aiIR := &AIPolicyIR{
			AISecret: aiSecret,
		}

		// Execute
		err := preProcessAITrafficPolicy(aiConfig, aiIR)

		// Verify
		require.Error(t, err)
		assert.Contains(t, err.Error(), "OpenAI moderation config must be set")
	})
}

// Mock implementation of RouteBackendContext for testing
func (ir *RouteBackendContext) NewRouteBackendContext() *RouteBackendContext {
	return &RouteBackendContext{
		configs: make(map[string]interface{}),
	}
}

func (ir *RouteBackendContext) AddTypedConfig(name string, config interface{}) {
	ir.configs[name] = config
}

func (ir *RouteBackendContext) GetTypedConfig(name string) interface{} {
	return ir.configs[name]
}

type RouteBackendContext struct {
	configs map[string]interface{}
}
