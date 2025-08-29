package agentgatewaybackend

import (
	"context"
	"testing"
	"time"

	"github.com/agentgateway/agentgateway/go/api"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	"istio.io/istio/pkg/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestBuildMCPIr(t *testing.T) {
	krtctx := krt.TestingDummyContext{}
	tests := []struct {
		name        string
		backend     *v1alpha1.Backend
		services    krt.Collection[*corev1.Service]
		namespaces  krt.Collection[krtcollections.NamespaceMetadata]
		expectError bool
		validate    func(mcpIr *MCPIr) bool
	}{
		{
			name: "Static MCP target backend",
			backend: &v1alpha1.Backend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "static-mcp-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.BackendSpec{
					Type: v1alpha1.BackendTypeMCP,
					MCP: &v1alpha1.MCP{
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
			services:    createMockServiceCollection(t),
			namespaces:  createMockNamespaceCollection(t),
			expectError: false,
			validate: func(ir *MCPIr) bool {
				if ir.Backends == nil || len(ir.Backends) != 2 {
					return false
				}
				for _, backend := range ir.Backends {
					if backend.Name == "test-ns/static-mcp-backend" {
						mcp := backend.GetMcp()
						if mcp == nil || len(mcp.Targets) != 1 {
							return false
						}
						target := mcp.Targets[0]
						if !(target.Name == "static-target" &&
							target.Backend.Port == 8080 &&
							target.Protocol == api.MCPTarget_SSE &&
							target.Path == "override-sse" &&
							target.Backend.GetBackend() == "test-ns/static-mcp-backend/static-target") {
							return false
						}
					} else if backend.Name == "test-ns/static-mcp-backend/static-target" {
						static := backend.GetStatic()
						if static == nil {
							return false
						}
						if !(static.Host == "mcp-server.example.com" &&
							static.Port == 8080) {
							return false
						}
					}
				}
				return true
			},
		},
		{
			name: "Service selector MCP backend - same namespace",
			backend: &v1alpha1.Backend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service-mcp-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.BackendSpec{
					Type: v1alpha1.BackendTypeMCP,
					MCP: &v1alpha1.MCP{
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
			services:    createMockServiceCollectionWithMCPService(t, "test-ns", "mcp-service", "app=mcp-server"),
			namespaces:  createMockNamespaceCollection(t),
			expectError: false,
			validate: func(ir *MCPIr) bool {
				if ir.Backends == nil || len(ir.Backends) != 1 {
					return false
				}
				backend := ir.Backends[0]
				if backend.Name != "test-ns/service-mcp-backend" {
					return false
				}
				mcp := backend.GetMcp()
				if mcp == nil || len(mcp.Targets) != 1 {
					return false
				}
				target := mcp.Targets[0]
				return target.Name == "mcp-service-mcp" &&
					target.Backend.Port == 8080 &&
					target.Backend.GetService() == "test-ns/mcp-service.test-ns.svc.cluster.local"
			},
		},
		{
			name: "Namespace selector MCP backend",
			backend: &v1alpha1.Backend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "namespace-mcp-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.BackendSpec{
					Type: v1alpha1.BackendTypeMCP,
					MCP: &v1alpha1.MCP{
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
			services:    createMockServiceCollectionMultiNamespace(t),
			namespaces:  createMockNamespaceCollectionWithLabels(t),
			expectError: false,
			validate: func(ir *MCPIr) bool {
				if ir.Backends == nil || len(ir.Backends) != 1 {
					return false
				}
				backend := ir.Backends[0]
				if backend.Name != "test-ns/namespace-mcp-backend" {
					return false
				}
				mcp := backend.GetMcp()
				if mcp == nil || len(mcp.Targets) != 1 {
					return false
				}
				target := mcp.Targets[0]
				// Should find the service in prod-ns which has environment=production label
				return target.Name == "prod-mcp" &&
					target.Backend.Port == 8080 &&
					target.Backend.GetService() == "prod-ns/prod.prod-ns.svc.cluster.local"
			},
		},
		{
			name: "Error case - nil MCP spec",
			backend: &v1alpha1.Backend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.BackendSpec{
					Type: v1alpha1.BackendTypeMCP,
					MCP:  nil,
				},
			},
			services:    createMockServiceCollection(t),
			namespaces:  createMockNamespaceCollection(t),
			expectError: true,
		},
		{
			name: "Error case - invalid service selector",
			backend: &v1alpha1.Backend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-selector-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.BackendSpec{
					Type: v1alpha1.BackendTypeMCP,
					MCP: &v1alpha1.MCP{
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
			services:    createMockServiceCollection(t),
			namespaces:  createMockNamespaceCollection(t),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildMCPIr(krtctx, tt.backend, tt.services, tt.namespaces)

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

			if tt.validate != nil && !tt.validate(result) {
				t.Errorf("validation failed")
			}
		})
	}
}

func TestBuildAIBackendIr(t *testing.T) {
	krtctx := krt.TestingDummyContext{}

	tests := []struct {
		name        string
		backend     *v1alpha1.Backend
		secrets     *krtcollections.SecretIndex
		expectError bool
		validate    func(aiIr *AIIr) bool
	}{
		{
			name: "Valid OpenAI backend with inline auth",
			backend: &v1alpha1.Backend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openai-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.BackendSpec{
					Type: v1alpha1.BackendTypeAI,
					AI: &v1alpha1.AIBackend{
						LLM: &v1alpha1.LLMProvider{
							Provider: v1alpha1.SupportedLLMProvider{
								OpenAI: &v1alpha1.OpenAIConfig{
									Model: stringPtr("gpt-4"),
									AuthToken: v1alpha1.SingleAuthToken{
										Kind:   v1alpha1.Inline,
										Inline: stringPtr("sk-test-token"),
									},
								},
							},
						},
					},
				},
			},
			secrets:     nil,
			expectError: false,
			validate: func(aiIr *AIIr) bool {
				return aiIr != nil &&
					aiIr.Name == "test-ns/openai-backend" &&
					aiIr.Backend != nil &&
					aiIr.Backend.GetOpenai() != nil &&
					aiIr.Backend.GetOpenai().Model != nil &&
					aiIr.Backend.GetOpenai().Model.Value == "gpt-4" &&
					aiIr.AuthPolicy != nil &&
					aiIr.AuthPolicy.GetKey() != nil &&
					aiIr.AuthPolicy.GetKey().Secret == "sk-test-token"
			},
		},
		{
			name: "Valid Anthropic backend with model",
			backend: &v1alpha1.Backend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "anthropic-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.BackendSpec{
					Type: v1alpha1.BackendTypeAI,
					AI: &v1alpha1.AIBackend{
						LLM: &v1alpha1.LLMProvider{
							Provider: v1alpha1.SupportedLLMProvider{
								Anthropic: &v1alpha1.AnthropicConfig{
									Model: stringPtr("claude-3-sonnet"),
									AuthToken: v1alpha1.SingleAuthToken{
										Kind:   v1alpha1.Inline,
										Inline: stringPtr("test-api-key"),
									},
								},
							},
						},
					},
				},
			},
			secrets:     nil,
			expectError: false,
			validate: func(aiIr *AIIr) bool {
				return aiIr != nil &&
					aiIr.Name == "test-ns/anthropic-backend" &&
					aiIr.Backend != nil &&
					aiIr.Backend.GetAnthropic() != nil &&
					aiIr.Backend.GetAnthropic().Model != nil &&
					aiIr.Backend.GetAnthropic().Model.Value == "claude-3-sonnet" &&
					aiIr.AuthPolicy != nil &&
					aiIr.AuthPolicy.GetKey() != nil &&
					aiIr.AuthPolicy.GetKey().Secret == "test-api-key"
			},
		},
		{
			name: "Valid Gemini backend",
			backend: &v1alpha1.Backend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gemini-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.BackendSpec{
					Type: v1alpha1.BackendTypeAI,
					AI: &v1alpha1.AIBackend{
						LLM: &v1alpha1.LLMProvider{
							Provider: v1alpha1.SupportedLLMProvider{
								Gemini: &v1alpha1.GeminiConfig{
									Model: "gemini-pro",
									AuthToken: v1alpha1.SingleAuthToken{
										Kind:   v1alpha1.Inline,
										Inline: stringPtr("gemini-api-key"),
									},
								},
							},
						},
					},
				},
			},
			secrets:     nil,
			expectError: false,
			validate: func(aiIr *AIIr) bool {
				return aiIr != nil &&
					aiIr.Name == "test-ns/gemini-backend" &&
					aiIr.Backend != nil &&
					aiIr.Backend.GetGemini() != nil &&
					aiIr.Backend.GetGemini().Model != nil &&
					aiIr.Backend.GetGemini().Model.Value == "gemini-pro" &&
					aiIr.AuthPolicy != nil &&
					aiIr.AuthPolicy.GetKey() != nil &&
					aiIr.AuthPolicy.GetKey().Secret == "gemini-api-key"
			},
		},
		{
			name: "Valid VertexAI backend",
			backend: &v1alpha1.Backend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vertex-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.BackendSpec{
					Type: v1alpha1.BackendTypeAI,
					AI: &v1alpha1.AIBackend{
						LLM: &v1alpha1.LLMProvider{
							Provider: v1alpha1.SupportedLLMProvider{
								VertexAI: &v1alpha1.VertexAIConfig{
									Model: "gemini-pro",
									AuthToken: v1alpha1.SingleAuthToken{
										Kind: v1alpha1.Passthrough,
									},
								},
							},
						},
					},
				},
			},
			secrets:     nil,
			expectError: false,
			validate: func(aiIr *AIIr) bool {
				print(aiIr.AuthPolicy.GetPassthrough().String())
				return aiIr != nil &&
					aiIr.Name == "test-ns/vertex-backend" &&
					aiIr.Backend != nil &&
					aiIr.Backend.GetVertex() != nil &&
					aiIr.Backend.GetVertex().Model != nil &&
					aiIr.Backend.GetVertex().Model.Value == "gemini-pro" &&
					aiIr.AuthPolicy != nil &&
					aiIr.AuthPolicy.GetPassthrough() != nil
			},
		},
		{
			name: "Valid Bedrock backend with custom region and guardrail",
			backend: &v1alpha1.Backend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bedrock-backend-custom",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.BackendSpec{
					Type: v1alpha1.BackendTypeAI,
					AI: &v1alpha1.AIBackend{
						LLM: &v1alpha1.LLMProvider{
							Provider: v1alpha1.SupportedLLMProvider{
								Bedrock: &v1alpha1.BedrockConfig{
									Model:  "anthropic.claude-3-haiku-20240307-v1:0",
									Region: "eu-west-1",
									Guardrail: &v1alpha1.AWSGuardrailConfig{
										GuardrailIdentifier: "test-guardrail",
										GuardrailVersion:    "1.0",
									},
									Auth: &v1alpha1.AwsAuth{
										Type: v1alpha1.AwsAuthTypeSecret,
										SecretRef: &corev1.LocalObjectReference{
											Name: "aws-secret-custom",
										},
									},
								},
							},
						},
					},
				},
			},
			secrets: createMockSecretIndex(t, "test-ns", "aws-secret-custom", map[string]string{
				"accessKey":    "AKIACUSTOM",
				"secretKey":    "secretcustom",
				"sessionToken": "token123",
			}),
			expectError: false,
			validate: func(aiIr *AIIr) bool {
				bedrock := aiIr.Backend.GetBedrock()
				aws := aiIr.AuthPolicy.GetAws().GetExplicitConfig()
				return aiIr != nil &&
					aiIr.Name == "test-ns/bedrock-backend-custom" &&
					bedrock != nil &&
					bedrock.Model.Value == "anthropic.claude-3-haiku-20240307-v1:0" &&
					bedrock.Region == "eu-west-1" &&
					bedrock.GuardrailIdentifier != nil &&
					bedrock.GuardrailIdentifier.Value == "test-guardrail" &&
					bedrock.GuardrailVersion != nil &&
					bedrock.GuardrailVersion.Value == "1.0" &&
					aws != nil &&
					aws.AccessKeyId == "AKIACUSTOM" &&
					aws.SecretAccessKey == "secretcustom" &&
					aws.SessionToken != nil &&
					*aws.SessionToken == "token123" &&
					aws.Region == "eu-west-1"
			},
		},
		{
			name: "OpenAI backend with secret reference auth",
			backend: &v1alpha1.Backend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "openai-secret-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.BackendSpec{
					Type: v1alpha1.BackendTypeAI,
					AI: &v1alpha1.AIBackend{
						LLM: &v1alpha1.LLMProvider{
							Provider: v1alpha1.SupportedLLMProvider{
								OpenAI: &v1alpha1.OpenAIConfig{
									Model: stringPtr("gpt-3.5-turbo"),
									AuthToken: v1alpha1.SingleAuthToken{
										Kind: v1alpha1.SecretRef,
										SecretRef: &corev1.LocalObjectReference{
											Name: "openai-secret",
										},
									},
								},
							},
						},
					},
				},
			},
			secrets: createMockSecretIndex(t, "test-ns", "openai-secret", map[string]string{
				"Authorization": "Bearer sk-secret-token",
			}),
			expectError: false,
			validate: func(aiIr *AIIr) bool {
				return aiIr != nil &&
					aiIr.Name == "test-ns/openai-secret-backend" &&
					aiIr.Backend.GetOpenai().Model.Value == "gpt-3.5-turbo" &&
					aiIr.AuthPolicy.GetKey().Secret == "sk-secret-token" // Bearer prefix should be stripped
			},
		},
		{
			name: "MultiPool backend - uses first priority",
			backend: &v1alpha1.Backend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multipool-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.BackendSpec{
					Type: v1alpha1.BackendTypeAI,
					AI: &v1alpha1.AIBackend{
						MultiPool: &v1alpha1.MultiPoolConfig{
							Priorities: []v1alpha1.Priority{
								{
									Pool: []v1alpha1.LLMProvider{
										{
											Provider: v1alpha1.SupportedLLMProvider{
												OpenAI: &v1alpha1.OpenAIConfig{
													Model: stringPtr("gpt-4"),
													AuthToken: v1alpha1.SingleAuthToken{
														Kind:   v1alpha1.Inline,
														Inline: stringPtr("first-token"),
													},
												},
											},
										},
										{
											Provider: v1alpha1.SupportedLLMProvider{
												Anthropic: &v1alpha1.AnthropicConfig{
													Model: stringPtr("claude-3"),
													AuthToken: v1alpha1.SingleAuthToken{
														Kind:   v1alpha1.Inline,
														Inline: stringPtr("second-token"),
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
			secrets:     nil,
			expectError: false,
			validate: func(aiIr *AIIr) bool {
				return aiIr != nil &&
					aiIr.Name == "test-ns/multipool-backend" &&
					aiIr.Backend.GetOpenai() != nil &&
					aiIr.Backend.GetOpenai().Model.Value == "gpt-4" &&
					aiIr.AuthPolicy.GetKey().Secret == "first-token"
			},
		},
		{
			name: "Error case - nil AI spec",
			backend: &v1alpha1.Backend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.BackendSpec{
					Type: v1alpha1.BackendTypeAI,
					AI:   nil,
				},
			},
			secrets:     nil,
			expectError: true,
		},
		{
			name: "Error case - no LLM or MultiPool configured",
			backend: &v1alpha1.Backend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-backend-2",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.BackendSpec{
					Type: v1alpha1.BackendTypeAI,
					AI: &v1alpha1.AIBackend{
						LLM:       nil,
						MultiPool: nil,
					},
				},
			},
			secrets:     nil,
			expectError: true,
		},
		{
			name: "Error case - empty MultiPool",
			backend: &v1alpha1.Backend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-backend-3",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.BackendSpec{
					Type: v1alpha1.BackendTypeAI,
					AI: &v1alpha1.AIBackend{
						MultiPool: &v1alpha1.MultiPoolConfig{
							Priorities: []v1alpha1.Priority{},
						},
					},
				},
			},
			secrets:     nil,
			expectError: true,
		},
		{
			name: "Error case - no supported provider configured",
			backend: &v1alpha1.Backend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-backend-4",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.BackendSpec{
					Type: v1alpha1.BackendTypeAI,
					AI: &v1alpha1.AIBackend{
						LLM: &v1alpha1.LLMProvider{
							Provider: v1alpha1.SupportedLLMProvider{
								// No providers configured
							},
						},
					},
				},
			},
			secrets:     nil,
			expectError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildAIIr(krtctx, tt.backend, tt.secrets)

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

			if tt.validate != nil && !tt.validate(result) {
				t.Errorf("validation failed")
			}
		})
	}
}

// Helper function to create a string pointer
func stringPtr(s string) *string {
	return &s
}

// Helper function to create a mock SecretIndex for testing
func createMockSecretIndex(t test.Failer, namespace, name string, data map[string]string) *krtcollections.SecretIndex {
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

	// Create a mock collection with the secret
	var inputs []any
	inputs = append(inputs, mockSecret)

	mock := krttest.NewMock(t, inputs)

	// Get the underlying mock collections
	mockSecretCollection := krttest.GetMockCollection[*corev1.Secret](mock)
	mockRefGrantCollection := krttest.GetMockCollection[*gwv1beta1.ReferenceGrant](mock)

	// Wait for the mock collections to sync
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // long timeout - just in case. we should never reach it.
	defer cancel()
	mockSecretCollection.WaitUntilSynced(ctx.Done())
	mockRefGrantCollection.WaitUntilSynced(ctx.Done())

	// Create the secret collection
	secretsCol := map[schema.GroupKind]krt.Collection[ir.Secret]{
		corev1.SchemeGroupVersion.WithKind("Secret").GroupKind(): krt.NewCollection(
			mockSecretCollection,
			func(kctx krt.HandlerContext, i *corev1.Secret) *ir.Secret {
				res := ir.Secret{
					ObjectSource: ir.ObjectSource{
						Group:     "",
						Kind:      "Secret",
						Namespace: i.Namespace,
						Name:      i.Name,
					},
					Obj:  i,
					Data: i.Data,
				}
				return &res
			},
		),
	}

	// Create a minimal RefGrantIndex for the SecretIndex
	refgrants := krtcollections.NewRefGrantIndex(mockRefGrantCollection)

	// Wait for the transformed secret collection to sync
	secretCollection := secretsCol[corev1.SchemeGroupVersion.WithKind("Secret").GroupKind()]
	secretCollection.WaitUntilSynced(ctx.Done())

	// Create the SecretIndex
	index := krtcollections.NewSecretIndex(secretsCol, refgrants)

	// Ensure the index is fully synced before returning
	for !index.HasSynced() {
		time.Sleep(50 * time.Millisecond)
	}

	return index
}

func TestBuildStaticIr(t *testing.T) {
	tests := []struct {
		name        string
		backend     *v1alpha1.Backend
		expectError bool
		validate    func(*StaticIr) bool
	}{
		{
			name: "Valid single host backend",
			backend: &v1alpha1.Backend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.BackendSpec{
					Static: &v1alpha1.StaticBackend{
						Hosts: []v1alpha1.Host{
							{Host: "api.example.com", Port: 443},
						},
					},
				},
			},
			expectError: false,
			validate: func(ir *StaticIr) bool {
				return ir.Backend != nil &&
					ir.Backend.Name == "test-ns/test-backend" &&
					ir.Backend.GetStatic().Host == "api.example.com" &&
					ir.Backend.GetStatic().Port == 443
			},
		},
		{
			name: "Multiple hosts - should error",
			backend: &v1alpha1.Backend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.BackendSpec{
					Static: &v1alpha1.StaticBackend{
						Hosts: []v1alpha1.Host{
							{Host: "host1.example.com", Port: 443},
							{Host: "host2.example.com", Port: 443},
						},
					},
				},
			},
			expectError: true,
			validate:    nil,
		},
		{
			name: "No hosts - should error",
			backend: &v1alpha1.Backend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backend",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.BackendSpec{
					Static: &v1alpha1.StaticBackend{
						Hosts: []v1alpha1.Host{},
					},
				},
			},
			expectError: true,
			validate:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildStaticIr(tt.backend)

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

			if tt.validate != nil && !tt.validate(result) {
				t.Errorf("validation failed")
			}
		})
	}
}

func TestGetSecretValue(t *testing.T) {
	tests := []struct {
		name         string
		secret       *ir.Secret
		key          string
		expectedVal  string
		expectedBool bool
	}{
		{
			name: "Valid secret value",
			secret: &ir.Secret{
				ObjectSource: ir.ObjectSource{
					Group:     "",
					Kind:      "Secret",
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
			secret: &ir.Secret{
				ObjectSource: ir.ObjectSource{
					Group:     "",
					Kind:      "Secret",
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
			secret: &ir.Secret{
				ObjectSource: ir.ObjectSource{
					Group:     "",
					Kind:      "Secret",
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
			secret: &ir.Secret{
				ObjectSource: ir.ObjectSource{
					Group:     "",
					Kind:      "Secret",
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
			secret: &ir.Secret{
				ObjectSource: ir.ObjectSource{
					Group:     "",
					Kind:      "Secret",
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
			val, found := getSecretValue(tt.secret, tt.key)

			if found != tt.expectedBool {
				t.Errorf("found = %v, expected %v", found, tt.expectedBool)
			}

			if val != tt.expectedVal {
				t.Errorf("value = %v, expected %v", val, tt.expectedVal)
			}
		})
	}
}

// Helper functions for creating mock collections

// createMockServiceCollection creates a basic mock service collection
func createMockServiceCollection(t test.Failer) krt.Collection[*corev1.Service] {
	mock := krttest.NewMock(t, []any{})
	return krttest.GetMockCollection[*corev1.Service](mock)
}

// createMockNamespaceCollection creates a basic mock namespace collection
func createMockNamespaceCollection(t test.Failer) krt.Collection[krtcollections.NamespaceMetadata] {
	mock := krttest.NewMock(t, []any{})
	return krttest.GetMockCollection[krtcollections.NamespaceMetadata](mock)
}

// createMockServiceCollectionWithMCPService creates a mock service collection with a specific MCP service
func createMockServiceCollectionWithMCPService(t test.Failer, namespace, serviceName, labels string) krt.Collection[*corev1.Service] {
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
					AppProtocol: ptr.To(mcpProtocol),
				},
			},
		},
	}

	mock := krttest.NewMock(t, []any{mockService})
	mockCol := krttest.GetMockCollection[*corev1.Service](mock)
	// Ensure the index is fully synced before returning
	for !mockCol.HasSynced() {
		time.Sleep(50 * time.Millisecond)
	}
	return mockCol
}

// createMockServiceCollectionMultiNamespace creates a mock service collection with services in multiple namespaces
func createMockServiceCollectionMultiNamespace(t test.Failer) krt.Collection[*corev1.Service] {
	services := []*corev1.Service{
		{
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
						AppProtocol: ptr.To(mcpProtocol),
					},
				},
			},
		},
		{
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
						AppProtocol: ptr.To(mcpProtocol),
					},
				},
			},
		},
		{
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
						AppProtocol: ptr.To(mcpProtocol),
					},
				},
			},
		},
	}

	var inputs []any
	for _, svc := range services {
		inputs = append(inputs, svc)
	}

	mock := krttest.NewMock(t, inputs)
	mockCol := krttest.GetMockCollection[*corev1.Service](mock)
	// Ensure the index is fully synced before returning
	for !mockCol.HasSynced() {
		time.Sleep(50 * time.Millisecond)
	}
	return mockCol
}

// createMockNamespaceCollectionWithLabels creates a mock namespace collection with labeled namespaces
func createMockNamespaceCollectionWithLabels(t test.Failer) krt.Collection[krtcollections.NamespaceMetadata] {
	namespaces := []krtcollections.NamespaceMetadata{
		{
			Name: "test-ns",
			Labels: map[string]string{
				"environment": "test",
			},
		},
		{
			Name: "prod-ns",
			Labels: map[string]string{
				"environment": "production",
			},
		},
		{
			Name: "dev-ns",
			Labels: map[string]string{
				"environment": "development",
			},
		},
	}

	var inputs []any
	for _, ns := range namespaces {
		inputs = append(inputs, ns)
	}

	mock := krttest.NewMock(t, inputs)
	mockCol := krttest.GetMockCollection[krtcollections.NamespaceMetadata](mock)
	// Ensure the index is fully synced before returning
	for !mockCol.HasSynced() {
		time.Sleep(50 * time.Millisecond)
	}
	return mockCol
}
