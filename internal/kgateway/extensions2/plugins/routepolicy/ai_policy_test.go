package routepolicy

import (
	"context"
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

func TestProcessAIRoutePolicy(t *testing.T) {
	extprocSettings := &envoy_ext_proc_v3.ExtProcPerRoute{
		Override: &envoy_ext_proc_v3.ExtProcPerRoute_Overrides{
			Overrides: &envoy_ext_proc_v3.ExtProcOverrides{
				GrpcInitialMetadata: []*envoy_config_core_v3.HeaderValue{},
			},
		},
	}
	pCtx := &ir.RouteBackendContext{
		TypedFilterConfig: &map[string]proto.Message{
			wellknown.AIExtProcFilterName: extprocSettings,
		},
	}

	t.Run("sets streaming header for chat streaming route", func(t *testing.T) {
		// Setup
		plugin := &routePolicyPluginGwPass{}
		ctx := context.Background()
		chatStreamingType := v1alpha1.CHAT_STREAMING
		aiConfig := &v1alpha1.AIRoutePolicy{
			RouteType: &chatStreamingType,
		}
		aiSecret := &ir.Secret{}

		// Execute
		err := plugin.processAIRoutePolicy(ctx, aiConfig, pCtx, extprocSettings, aiSecret)

		// Verify
		require.NoError(t, err)
		assert.True(t, plugin.setAIFilter)

		// Verify streaming header was added
		found := false
		for _, header := range extprocSettings.GetOverrides().GrpcInitialMetadata {
			if header.Key == "x-chat-streaming" && header.Value == "true" {
				found = true
				break
			}
		}
		assert.True(t, found, "streaming header not found")

		// Verify transformation and extproc were added to context
		transformation, ok := pCtx.GetTypedConfig(wellknown.AIPolicyTransformationFilterName).(*envoytransformation.RouteTransformations)
		assert.True(t, ok)
		assert.NotNil(t, transformation)

		extprocConfig, ok := pCtx.GetTypedConfig(wellknown.AIExtProcFilterName).(*envoy_ext_proc_v3.ExtProcPerRoute)
		assert.True(t, ok)
		assert.Equal(t, extprocSettings, extprocConfig)
	})

	t.Run("sets debug logging when environment variable is set", func(t *testing.T) {
		// Setup
		plugin := &routePolicyPluginGwPass{}
		ctx := context.Background()
		aiConfig := &v1alpha1.AIRoutePolicy{}
		extprocSettings := &envoy_ext_proc_v3.ExtProcPerRoute{
			Override: &envoy_ext_proc_v3.ExtProcPerRoute_Overrides{
				Overrides: &envoy_ext_proc_v3.ExtProcOverrides{
					GrpcInitialMetadata: []*envoy_config_core_v3.HeaderValue{},
				},
			},
		}
		aiSecret := &ir.Secret{}

		// Set env var
		oldEnv := os.Getenv("AI_PLUGIN_DEBUG_TRANSFORMATIONS")
		os.Setenv("AI_PLUGIN_DEBUG_TRANSFORMATIONS", "true")
		defer os.Setenv("AI_PLUGIN_DEBUG_TRANSFORMATIONS", oldEnv)

		// Execute
		err := plugin.processAIRoutePolicy(ctx, aiConfig, pCtx, extprocSettings, aiSecret)

		// Verify
		require.NoError(t, err)
		transformation, ok := pCtx.GetTypedConfig(wellknown.AIPolicyTransformationFilterName).(*envoytransformation.RouteTransformations)
		assert.True(t, ok)
		assert.True(t, len(transformation.Transformations) == 1)
		assert.True(t, transformation.Transformations[0].GetRequestMatch().GetRequestTransformation().GetLogRequestResponseInfo().GetValue())
	})

	t.Run("applies defaults and prompt enrichment", func(t *testing.T) {
		// Setup
		plugin := &routePolicyPluginGwPass{}
		ctx := context.Background()
		aiConfig := &v1alpha1.AIRoutePolicy{
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
		extprocSettings := &envoy_ext_proc_v3.ExtProcPerRoute{
			Override: &envoy_ext_proc_v3.ExtProcPerRoute_Overrides{
				Overrides: &envoy_ext_proc_v3.ExtProcOverrides{
					GrpcInitialMetadata: []*envoy_config_core_v3.HeaderValue{},
				},
			},
		}
		aiSecret := &ir.Secret{}

		// Execute
		err := plugin.processAIRoutePolicy(ctx, aiConfig, pCtx, extprocSettings, aiSecret)

		// Verify
		require.NoError(t, err)
		routeTransformations, ok := pCtx.GetTypedConfig(wellknown.AIPolicyTransformationFilterName).(*envoytransformation.RouteTransformations)
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
		plugin := &routePolicyPluginGwPass{}
		ctx := context.Background()
		aiConfig := &v1alpha1.AIRoutePolicy{
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
		aiSecret := &ir.Secret{}

		// Execute
		err := plugin.processAIRoutePolicy(ctx, aiConfig, pCtx, extprocSettings, aiSecret)

		// Verify
		require.NoError(t, err)

		// Check that the guardrails config headers were added
		foundReqConfig := false
		foundReqHash := false
		foundRespConfig := false
		foundRespHash := false

		for _, header := range extprocSettings.GetOverrides().GrpcInitialMetadata {
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
	})

	t.Run("handles error from prompt guard", func(t *testing.T) {
		// Setup
		plugin := &routePolicyPluginGwPass{}
		ctx := context.Background()
		aiConfig := &v1alpha1.AIRoutePolicy{
			PromptGuard: &v1alpha1.AIPromptGuard{
				Request: &v1alpha1.PromptguardRequest{
					Moderation: &v1alpha1.Moderation{
						// missing config
					},
				},
			},
		}
		aiSecret := &ir.Secret{}

		// Execute
		err := plugin.processAIRoutePolicy(ctx, aiConfig, pCtx, extprocSettings, aiSecret)

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
