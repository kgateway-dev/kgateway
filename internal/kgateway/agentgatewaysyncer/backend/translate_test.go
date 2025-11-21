package agentgatewaybackend_test

import (
	"fmt"
	"testing"

	"github.com/agentgateway/agentgateway/go/api"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/slices"
	"istio.io/istio/pkg/test/util/assert"
	"istio.io/istio/pkg/util/protomarshal"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	agentgatewaybackend "github.com/kgateway-dev/kgateway/v2/internal/kgateway/agentgatewaysyncer/backend"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/plugins"
	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/testutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

func TestBuildMCP(t *testing.T) {
	tests := []struct {
		name        string
		backend     *v1alpha1.AgentgatewayBackend
		expectError bool
		inputs      []any
	}{
		{
			name: "Static MCPBackend target backend",
			backend: &v1alpha1.AgentgatewayBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "static-mcp-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.AgentgatewayBackendSpec{
					MCP: &v1alpha1.MCPBackend{
						Targets: []v1alpha1.McpTargetSelector{
							{
								Name: "static-target",
								Static: &v1alpha1.McpTarget{
									Host:     "mcp-server.example.com",
									Port:     8080,
									Path:     stringPtr("override-sse"),
									Protocol: ptr.To(v1alpha1.MCPProtocolSSE),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Service selector MCPBackend backend - same namespace",
			backend: &v1alpha1.AgentgatewayBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service-mcp-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.AgentgatewayBackendSpec{
					MCP: &v1alpha1.MCPBackend{
						Targets: []v1alpha1.McpTargetSelector{
							{
								Selector: &v1alpha1.McpSelector{
									Service: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"app": "mcp-server",
										},
									},
								},
							},
						},
					},
				},
			},
			inputs: []any{createMockMCPService("test-ns", "mcp-service", "app=mcp-server")},
		},
		{
			name: "Namespace selector MCPBackend backend",
			backend: &v1alpha1.AgentgatewayBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "namespace-mcp-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.AgentgatewayBackendSpec{
					MCP: &v1alpha1.MCPBackend{
						Targets: []v1alpha1.McpTargetSelector{
							{
								Selector: &v1alpha1.McpSelector{
									Namespace: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"environment": "production",
										},
									},
									Service: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"type": "mcp",
										},
									},
								},
							},
						},
					},
				},
			},
			inputs: append(createMockMultipleNamespaceServices(), createMockNamespaceCollectionWithLabels()...),
		},
		{
			name: "Error case - invalid service selector",
			backend: &v1alpha1.AgentgatewayBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-selector-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.AgentgatewayBackendSpec{
					MCP: &v1alpha1.MCPBackend{
						Targets: []v1alpha1.McpTargetSelector{
							{
								Selector: &v1alpha1.McpSelector{
									Service: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      "invalid",
												Operator: "InvalidOperator",
												Values:   []string{"value"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := testutils.BuildMockPolicyContext(t, tt.inputs)
			result, err := agentgatewaybackend.BuildAgwBackend(ctx, tt.backend)
			if tt.expectError {
				assert.Error(t, err)
				return
			} else {
				assert.NoError(t, err)
			}

			b, err := yaml.Marshal(slices.Map(result, func(e *api.Backend) jsonMarshalProto {
				return jsonMarshalProto{e}
			}))
			assert.NoError(t, err)
			testutils.CompareGolden(t, b, fmt.Sprintf("testdata/%v.yaml", tt.name))
		})
	}
}

func TestBuildAIBackend(t *testing.T) {
	tests := []struct {
		name    string
		backend *v1alpha1.AgentgatewayBackend
		inputs  []any
	}{
		{
			name: "Valid OpenAI backend with inline auth",
			backend: &v1alpha1.AgentgatewayBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openai-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.AgentgatewayBackendSpec{
					Policies: &v1alpha1.AgentgatewayPolicyBackendFull{
						AgentgatewayPolicyBackendSimple: v1alpha1.AgentgatewayPolicyBackendSimple{
							Auth: &v1alpha1.BackendAuth{InlineKey: stringPtr("sk-test-token")},
						},
					},
					AI: &v1alpha1.AIBackend{
						LLM: &v1alpha1.LLMProvider{
							OpenAI: &v1alpha1.OpenAIConfig{
								Model: stringPtr("gpt-4"),
							},
						},
					},
				},
			},
		},
		{
			name: "Valid Azure OpenAI backend",
			backend: &v1alpha1.AgentgatewayBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "azure-openai-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.AgentgatewayBackendSpec{
					AI: &v1alpha1.AIBackend{
						LLM: &v1alpha1.LLMProvider{
							AzureOpenAI: &v1alpha1.AzureOpenAIConfig{
								Endpoint:       "endpoint-123.openai.azure.com",
								DeploymentName: ptr.To("my-deployment"),
								ApiVersion:     ptr.To("2024-02-15-preview"),
							},
						},
					},
				},
			},
		},
		{
			name: "Valid Anthropic backend with model",
			backend: &v1alpha1.AgentgatewayBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "anthropic-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.AgentgatewayBackendSpec{
					AI: &v1alpha1.AIBackend{
						LLM: &v1alpha1.LLMProvider{
							Anthropic: &v1alpha1.AnthropicConfig{
								Model: stringPtr("claude-3-sonnet"),
							},
						},
					},
				},
			},
		},
		{
			name: "Valid Gemini backend",
			backend: &v1alpha1.AgentgatewayBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gemini-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.AgentgatewayBackendSpec{
					AI: &v1alpha1.AIBackend{
						LLM: &v1alpha1.LLMProvider{
							Gemini: &v1alpha1.GeminiConfig{
								Model: stringPtr("gemini-pro"),
							},
						},
					},
				},
			},
		},
		{
			name: "Valid VertexAI backend",
			backend: &v1alpha1.AgentgatewayBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vertex-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.AgentgatewayBackendSpec{
					AI: &v1alpha1.AIBackend{
						LLM: &v1alpha1.LLMProvider{
							VertexAI: &v1alpha1.VertexAIConfig{
								Model: stringPtr("gemini-pro"),
							},
						},
					},
				},
			},
		},
		{
			name: "Valid Bedrock backend with custom region and guardrail",
			backend: &v1alpha1.AgentgatewayBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bedrock-backend-custom",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.AgentgatewayBackendSpec{
					// TODO: Add AWS auth
					//Policies: &v1alpha1.AgentgatewayPolicyBackendFull{
					//	AgentgatewayPolicyBackendSimple: v1alpha1.AgentgatewayPolicyBackendSimple{
					//		Auth: &v1alpha1.BackendAuth{},
					//	},
					//},
					AI: &v1alpha1.AIBackend{
						LLM: &v1alpha1.LLMProvider{
							Bedrock: &v1alpha1.BedrockConfig{
								Model:  ptr.To("anthropic.claude-3-haiku-20240307-v1:0"),
								Region: "eu-west-1",
								Guardrail: &v1alpha1.AWSGuardrailConfig{
									GuardrailIdentifier: "test-guardrail",
									GuardrailVersion:    "1.0",
								},
							},
						},
					},
				},
			},
			inputs: []any{
				createMockSecret("test-ns", "aws-secret-custom", map[string]string{
					"accessKey":    "AKIACUSTOM",
					"secretKey":    "secretcustom",
					"sessionToken": "token123",
				}),
			},
		},
		{
			name: "OpenAI backend with secret reference auth",
			backend: &v1alpha1.AgentgatewayBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openai-secret-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.AgentgatewayBackendSpec{
					Policies: &v1alpha1.AgentgatewayPolicyBackendFull{
						AgentgatewayPolicyBackendSimple: v1alpha1.AgentgatewayPolicyBackendSimple{
							Auth: &v1alpha1.BackendAuth{SecretRef: &corev1.LocalObjectReference{
								Name: "openai-secret",
							}},
						},
					},
					AI: &v1alpha1.AIBackend{
						LLM: &v1alpha1.LLMProvider{
							OpenAI: &v1alpha1.OpenAIConfig{
								Model: stringPtr("gpt-3.5-turbo"),
							},
						},
					},
				},
			},
			inputs: []any{
				createMockSecret("test-ns", "openai-secret", map[string]string{
					"Authorization": "Bearer sk-secret-token",
				}),
			},
		},
		{
			name: "MultiPool backend - translates all providers for failover",
			backend: &v1alpha1.AgentgatewayBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multipool-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.AgentgatewayBackendSpec{
					AI: &v1alpha1.AIBackend{
						PriorityGroups: []v1alpha1.PriorityGroup{
							{
								Providers: []v1alpha1.NamedLLMProvider{
									{
										Name: "openai",
										Policies: &v1alpha1.AgentgatewayPolicyBackendAI{
											AgentgatewayPolicyBackendSimple: v1alpha1.AgentgatewayPolicyBackendSimple{
												Auth: &v1alpha1.BackendAuth{InlineKey: stringPtr("first-token")},
											},
										},
										LLMProvider: v1alpha1.LLMProvider{
											OpenAI: &v1alpha1.OpenAIConfig{
												Model: stringPtr("gpt-4"),
											},
										},
									},
									{
										Name: "anthropic",
										Policies: &v1alpha1.AgentgatewayPolicyBackendAI{
											AgentgatewayPolicyBackendSimple: v1alpha1.AgentgatewayPolicyBackendSimple{
												Auth: &v1alpha1.BackendAuth{InlineKey: stringPtr("second-token")},
											},
										},
										LLMProvider: v1alpha1.LLMProvider{
											Anthropic: &v1alpha1.AnthropicConfig{
												Model: stringPtr("claude-3"),
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
			name: "MultiPool backend with multiple priority levels - creates separate provider groups",
			backend: &v1alpha1.AgentgatewayBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multipool-priority-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.AgentgatewayBackendSpec{
					AI: &v1alpha1.AIBackend{
						PriorityGroups: []v1alpha1.PriorityGroup{
							{
								Providers: []v1alpha1.NamedLLMProvider{
									{
										Name: "openai",
										Policies: &v1alpha1.AgentgatewayPolicyBackendAI{
											AgentgatewayPolicyBackendSimple: v1alpha1.AgentgatewayPolicyBackendSimple{
												Auth: &v1alpha1.BackendAuth{InlineKey: stringPtr("openai-primary")},
											},
										},
										LLMProvider: v1alpha1.LLMProvider{
											OpenAI: &v1alpha1.OpenAIConfig{
												Model: stringPtr("gpt-4"),
											},
										},
									},
									{
										Name: "anthropic",
										Policies: &v1alpha1.AgentgatewayPolicyBackendAI{
											AgentgatewayPolicyBackendSimple: v1alpha1.AgentgatewayPolicyBackendSimple{
												Auth: &v1alpha1.BackendAuth{InlineKey: stringPtr("anthropic-primary")},
											},
										},
										LLMProvider: v1alpha1.LLMProvider{
											Anthropic: &v1alpha1.AnthropicConfig{
												Model: stringPtr("claude-3-opus"),
											},
										},
									},
								},
							},
							{
								Providers: []v1alpha1.NamedLLMProvider{
									{
										Name: "gemini",
										Policies: &v1alpha1.AgentgatewayPolicyBackendAI{
											AgentgatewayPolicyBackendSimple: v1alpha1.AgentgatewayPolicyBackendSimple{
												Auth: &v1alpha1.BackendAuth{InlineKey: stringPtr("gemini-fallback")},
											},
										},
										LLMProvider: v1alpha1.LLMProvider{
											Gemini: &v1alpha1.GeminiConfig{
												Model: stringPtr("gemini-pro"),
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
			name: "OpenAI backend with routes configuration",
			backend: &v1alpha1.AgentgatewayBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openai-with-routes",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.AgentgatewayBackendSpec{
					Policies: &v1alpha1.AgentgatewayPolicyBackendFull{
						AI: &v1alpha1.BackendAI{
							Routes: map[string]v1alpha1.RouteType{
								"/v1/chat/completions": v1alpha1.RouteTypeCompletions,
								"/v1/messages":         v1alpha1.RouteTypeMessages,
								"/v1/models":           v1alpha1.RouteTypeModels,
								"*":                    v1alpha1.RouteTypePassthrough,
							},
						},
					},
					AI: &v1alpha1.AIBackend{
						LLM: &v1alpha1.LLMProvider{
							OpenAI: &v1alpha1.OpenAIConfig{
								Model: stringPtr("gpt-4o-mini"),
							},
						},
					},
				},
			},
		},
		{
			name: "Bedrock backend with new route types (responses and anthropic_token_count)",
			backend: &v1alpha1.AgentgatewayBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bedrock-with-new-routes",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.AgentgatewayBackendSpec{
					Policies: &v1alpha1.AgentgatewayPolicyBackendFull{
						AI: &v1alpha1.BackendAI{
							Routes: map[string]v1alpha1.RouteType{
								"/v1/chat/completions":      v1alpha1.RouteTypeCompletions,
								"/v1/messages":              v1alpha1.RouteTypeMessages,
								"/v1/responses":             v1alpha1.RouteTypeResponses,
								"/v1/messages/count_tokens": v1alpha1.RouteTypeAnthropicTokenCount,
								"/v1/models":                v1alpha1.RouteTypeModels,
							},
						},
					},
					AI: &v1alpha1.AIBackend{
						LLM: &v1alpha1.LLMProvider{
							Bedrock: &v1alpha1.BedrockConfig{
								Region: "us-east-1",
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := testutils.BuildMockPolicyContext(t, tt.inputs)
			result, err := agentgatewaybackend.BuildAgwBackend(ctx, tt.backend)
			assert.NoError(t, err)

			b, err := protomarshal.ToYAML(result[0])
			assert.NoError(t, err)
			testutils.CompareGolden(t, []byte(b), fmt.Sprintf("testdata/%v.yaml", tt.name))
		})
	}
}

// Helper function to create a string pointer
func stringPtr(s string) *string {
	return &s
}

// Helper function to create a mock SecretIndex for testing
func createMockSecret(namespace, name string, data map[string]string) *corev1.Secret {
	// Create mock secret data
	secretData := make(map[string][]byte)
	for k, v := range data {
		secretData[k] = []byte(v)
	}

	// Create a mock Secret object for KRT
	mockSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: secretData,
	}

	return mockSecret
}

func TestBuildStaticIr(t *testing.T) {
	tests := []struct {
		name        string
		backend     *v1alpha1.AgentgatewayBackend
		expectError bool
		validate    func(backend *api.Backend) bool
	}{
		{
			name: "Valid single host backend",
			backend: &v1alpha1.AgentgatewayBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.AgentgatewayBackendSpec{
					Static: &v1alpha1.AgentStaticBackend{
						Host: "api.example.com", Port: 443,
					},
				},
			},
			validate: func(backend *api.Backend) bool {
				return backend != nil &&
					backend.Name == "test-ns/test-backend" &&
					backend.GetStatic().Host == "api.example.com" &&
					backend.GetStatic().Port == 443
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := agentgatewaybackend.BuildAgwBackend(plugins.PolicyCtx{}, tt.backend)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error = %v", err)
				return
			}

			if tt.validate != nil && !tt.validate(result[0]) {
				t.Errorf("validation failed")
			}
		})
	}
}

func TestGetSecretValue(t *testing.T) {
	tests := []struct {
		name         string
		secret       *corev1.Secret
		key          string
		expectedVal  string
		expectedBool bool
	}{
		{
			name: "Valid secret value",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-secret",
				},
				Data: map[string][]byte{
					"key1": []byte("value1"),
				},
			},
			key:          "key1",
			expectedVal:  "value1",
			expectedBool: true,
		},
		{
			name: "Secret value with spaces",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-secret",
				},
				Data: map[string][]byte{
					"key1": []byte("  value with spaces  "),
				},
			},
			key:          "key1",
			expectedVal:  "value with spaces",
			expectedBool: true,
		},
		{
			name: "Key not found",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-secret",
				},
				Data: map[string][]byte{
					"other-key": []byte("value"),
				},
			},
			key:          "missing-key",
			expectedVal:  "",
			expectedBool: false,
		},
		{
			name: "Invalid UTF-8",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-secret",
				},
				Data: map[string][]byte{
					"key1": {0xff, 0xfe, 0xfd},
				},
			},
			key:          "key1",
			expectedVal:  "",
			expectedBool: false,
		},
		{
			name: "Empty secret data",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "test-secret",
				},
				Data: map[string][]byte{},
			},
			key:          "key1",
			expectedVal:  "",
			expectedBool: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, found := kubeutils.GetSecretValue(tt.secret, tt.key)

			if found != tt.expectedBool {
				t.Errorf("found = %v, expected %v", found, tt.expectedBool)
			}

			if val != tt.expectedVal {
				t.Errorf("value = %v, expected %v", val, tt.expectedVal)
			}
		})
	}
}

// createMockMCPService creates a mock service collection with a specific MCPBackend service
func createMockMCPService(namespace, serviceName, labels string) *corev1.Service {
	// Parse labels
	labelsMap := make(map[string]string)
	if labels != "" {
		// Simple parsing for "key=value" format
		for _, label := range []string{labels} {
			if len(label) > 0 {
				parts := []string{"app", "mcp-server"} // hardcoded for test
				if len(parts) == 2 {
					labelsMap[parts[0]] = parts[1]
				}
			}
		}
	}

	mockService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
			Labels:    labelsMap,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:        "mcp",
					Port:        8080,
					AppProtocol: ptr.To("kgateway.dev/mcp"),
				},
			},
		},
	}
	return mockService
}

// createMockServiceCollectionMultiNamespace creates a mock service collection with services in multiple namespaces
func createMockMultipleNamespaceServices() []any {
	services := []any{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "test-ns",
				Labels: map[string]string{
					"type": "mcp",
				},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name:        "mcp",
						Port:        8080,
						AppProtocol: ptr.To("kgateway.dev/mcp"),
					},
				},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "prod",
				Namespace: "prod-ns",
				Labels: map[string]string{
					"type": "mcp",
				},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name:        "mcp",
						Port:        8080,
						AppProtocol: ptr.To("kgateway.dev/mcp"),
					},
				},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dev",
				Namespace: "dev-ns",
				Labels: map[string]string{
					"type": "mcp",
				},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name:        "mcp",
						Port:        8080,
						AppProtocol: ptr.To("kgateway.dev/mcp"),
					},
				},
			},
		},
	}
	return services
}

// createMockNamespaceCollectionWithLabels creates a mock namespace collection with labeled namespaces
func createMockNamespaceCollectionWithLabels() []any {
	namespaces := []any{
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-ns",
				Labels: map[string]string{
					"environment": "test",
				},
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "prod-ns",
				Labels: map[string]string{
					"environment": "production",
				},
			},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dev-ns",
				Labels: map[string]string{
					"environment": "development",
				},
			},
		},
	}
	return namespaces
}

// jsonMarshalProto wraps a proto.Message so it can be marshaled with the standard encoding/json library
type jsonMarshalProto struct {
	proto.Message
}

func (p jsonMarshalProto) MarshalJSON() ([]byte, error) {
	return protomarshal.Marshal(p.Message)
}
