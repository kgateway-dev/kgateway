package ai

import (
	"context"
	"strings"
	"testing"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_ext_proc_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	envoytransformation "github.com/solo-io/envoy-gloo/go/config/filter/http/transformation/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"k8s.io/utils/ptr"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

func TestApplyAIBackend(t *testing.T) {
	customPath := "/api/v1/chat/completions"
	customHeader := "custom-header "
	customPrefix := "custom-prefix "

	outRoute := &envoy_config_route_v3.Route{}

	tests := []struct {
		name                string
		aiBackend           *v1alpha1.AIBackend
		pCtx                *ir.RouteBackendContext
		in                  ir.HttpBackend
		out                 *envoy_config_route_v3.Route
		expectedError       string
		expectedTypedConfig *map[string]proto.Message
	}{
		{
			name: "Single LLM provider",
			aiBackend: &v1alpha1.AIBackend{
				LLM: &v1alpha1.LLMProvider{
					Provider: v1alpha1.SupportedLLMProvider{
						OpenAI: &v1alpha1.OpenAIConfig{
							Model: ptr.To("gpt-3"),
							AuthToken: v1alpha1.SingleAuthToken{
								Kind:   v1alpha1.SingleAuthTokenKind("Inline"),
								Inline: ptr.To("test1"),
							},
						},
					},
				},
			},
			out:           outRoute,
			expectedError: "",
			expectedTypedConfig: &map[string]proto.Message{
				wellknown.AIExtProcFilterName: &envoy_ext_proc_v3.ExtProcPerRoute{
					Override: &envoy_ext_proc_v3.ExtProcPerRoute_Overrides{
						Overrides: &envoy_ext_proc_v3.ExtProcOverrides{
							GrpcInitialMetadata: []*envoy_config_core_v3.HeaderValue{
								{
									Key:   "x-llm-provider",
									Value: "openai",
								},
								{
									Key:   "x-llm-model",
									Value: "gpt-3",
								},
								{
									Key:   "x-request-id",
									Value: "%REQ(X-REQUEST-ID)%",
								},
							},
						},
					},
				},
				wellknown.AIBackendTransformationFilterName: &envoytransformation.RouteTransformations{
					Transformations: []*envoytransformation.RouteTransformations_RouteTransformation{
						{
							Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
								RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{
									RequestTransformation: &envoytransformation.Transformation{
										LogRequestResponseInfo: &wrapperspb.BoolValue{},
										TransformationType: &envoytransformation.Transformation_TransformationTemplate{
											TransformationTemplate: &envoytransformation.TransformationTemplate{
												Headers: map[string]*envoytransformation.InjaTemplate{
													":path": {
														Text: "/v1/chat/completions",
													},
													"Authorization": {
														Text: `Bearer {% if host_metadata("auth_token") != "" %}{{host_metadata("auth_token")}}{% else %}{{dynamic_metadata("auth_token","ai.kgateway.io")}}{% endif %}`,
													},
												},
												BodyTransformation: &envoytransformation.TransformationTemplate_MergeJsonKeys{
													MergeJsonKeys: &envoytransformation.MergeJsonKeys{
														JsonKeys: map[string]*envoytransformation.MergeJsonKeys_OverridableTemplate{
															"model": {
																Tmpl: &envoytransformation.InjaTemplate{
																	Text: `{% if host_metadata("model") != "" %}"{{host_metadata("model")}}"{% else %}"{{model}}"{% endif %}`,
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Single LLM provider with custom path",
			aiBackend: &v1alpha1.AIBackend{
				LLM: &v1alpha1.LLMProvider{
					PathOverride: &v1alpha1.PathOverride{
						FullPath: &customPath,
					},
					AuthHeaderOverride: &v1alpha1.AuthHeaderOverride{
						Prefix:     &customPrefix,
						HeaderName: &customHeader,
					},
					Provider: v1alpha1.SupportedLLMProvider{
						OpenAI: &v1alpha1.OpenAIConfig{
							Model: ptr.To("gpt-3"),
							AuthToken: v1alpha1.SingleAuthToken{
								Kind:   v1alpha1.SingleAuthTokenKind("Inline"),
								Inline: ptr.To("test1"),
							},
						},
					},
				},
			},
			out:           outRoute,
			expectedError: "",
			expectedTypedConfig: &map[string]proto.Message{
				wellknown.AIExtProcFilterName: &envoy_ext_proc_v3.ExtProcPerRoute{
					Override: &envoy_ext_proc_v3.ExtProcPerRoute_Overrides{
						Overrides: &envoy_ext_proc_v3.ExtProcOverrides{
							GrpcInitialMetadata: []*envoy_config_core_v3.HeaderValue{
								{
									Key:   "x-llm-provider",
									Value: "openai",
								},
								{
									Key:   "x-llm-model",
									Value: "gpt-3",
								},
								{
									Key:   "x-request-id",
									Value: "%REQ(X-REQUEST-ID)%",
								},
							},
						},
					},
				},
				wellknown.AIBackendTransformationFilterName: &envoytransformation.RouteTransformations{
					Transformations: []*envoytransformation.RouteTransformations_RouteTransformation{
						{
							Match: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch_{
								RequestMatch: &envoytransformation.RouteTransformations_RouteTransformation_RequestMatch{
									RequestTransformation: &envoytransformation.Transformation{
										LogRequestResponseInfo: &wrapperspb.BoolValue{},
										TransformationType: &envoytransformation.Transformation_TransformationTemplate{
											TransformationTemplate: &envoytransformation.TransformationTemplate{
												Headers: map[string]*envoytransformation.InjaTemplate{
													":path": {
														Text: customPath,
													},
													customHeader: {
														Text: customPrefix + `{% if host_metadata("auth_token") != "" %}{{host_metadata("auth_token")}}{% else %}{{dynamic_metadata("auth_token","ai.kgateway.io")}}{% endif %}`,
													},
												},
												BodyTransformation: &envoytransformation.TransformationTemplate_MergeJsonKeys{
													MergeJsonKeys: &envoytransformation.MergeJsonKeys{
														JsonKeys: map[string]*envoytransformation.MergeJsonKeys_OverridableTemplate{
															"model": {
																Tmpl: &envoytransformation.InjaTemplate{
																	Text: `{% if host_metadata("model") != "" %}"{{host_metadata("model")}}"{% else %}"{{model}}"{% endif %}`,
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Multiple LLM providers with different types",
			aiBackend: &v1alpha1.AIBackend{
				MultiPool: &v1alpha1.MultiPoolConfig{
					Priorities: []v1alpha1.Priority{
						{
							Pool: []v1alpha1.LLMProvider{
								{
									Provider: v1alpha1.SupportedLLMProvider{
										OpenAI: &v1alpha1.OpenAIConfig{
											Model: ptr.To("gpt-3"),
											AuthToken: v1alpha1.SingleAuthToken{
												Kind:   v1alpha1.SingleAuthTokenKind("Inline"),
												Inline: ptr.To("test1"),
											},
										},
									},
								},
								{
									Provider: v1alpha1.SupportedLLMProvider{
										Anthropic: &v1alpha1.AnthropicConfig{
											Model: ptr.To("claude"),
											AuthToken: v1alpha1.SingleAuthToken{
												Kind:   v1alpha1.SingleAuthTokenKind("Inline"),
												Inline: ptr.To("test2"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
			out:                 outRoute,
			expectedError:       "multiple AI backend types found for single ai route",
			expectedTypedConfig: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typedFilterConfig := ir.TypedFilterConfigMap(map[string]proto.Message{})
			pCtx := &ir.RouteBackendContext{
				TypedFilterConfig: typedFilterConfig,
				Backend: &ir.BackendObjectIR{
					ObjectSource: ir.ObjectSource{
						Group:     "test",
						Kind:      "test-backend-plugin",
						Namespace: "test-backend-plugin-ns",
						Name:      "test-backend-plugin-us",
					},
				},
				FilterChainName: "test",
			}
			tt.pCtx = pCtx
			aiIR := &IR{}
			err := PreprocessAIBackend(context.Background(), tt.aiBackend, aiIR)
			if tt.expectedError != "" && err == nil {
				t.Errorf("expected error but got nil")
			} else if tt.expectedError == "" && err != nil {
				t.Errorf("expected no error but got %v", err)
			} else if tt.expectedError != "" && err != nil {
				if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("expected error %v but got %v", tt.expectedError, err)
				}
			} else if tt.expectedError == "" {
				err := ApplyAIBackend(aiIR, tt.pCtx, tt.out)
				if err != nil {
					t.Errorf("expected no error but got %v", err)
				}

				if !tt.out.GetRoute().GetAutoHostRewrite().GetValue() {
					t.Errorf("expected auto host rewrite to be set after AI Backend translation")
				}

				// assert outputs
				if len(*tt.expectedTypedConfig) != len(tt.pCtx.TypedFilterConfig) {
					t.Errorf("expected %v typed config but got %v", tt.pCtx.TypedFilterConfig, tt.expectedTypedConfig)
				}
				expected := *tt.expectedTypedConfig
				actual := tt.pCtx.TypedFilterConfig
				for k, v := range expected {
					expectedVal := expected[k]
					actualVal := actual[k]
					if !proto.Equal(expectedVal, actualVal) {
						t.Errorf("expected %v typed config but got %v", expectedVal, actualVal)
					} else {
						println("expected", k, v)
					}
				}
			}
		})
	}
}
