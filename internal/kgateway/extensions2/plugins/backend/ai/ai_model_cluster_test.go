package ai

import (
	"strings"
	"testing"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

func TestProcessAIBackend_Empty(t *testing.T) {
	cluster := &envoyclusterv3.Cluster{
		Name: "test-cluster",
	}

	err := ProcessAIBackend(nil, nil, nil, cluster)

	assert.NoError(t, err)
	assert.Equal(t, "test-cluster", cluster.Name)
	assert.Nil(t, cluster.ClusterDiscoveryType)
}

func TestProcessAIBackend_OpenAI(t *testing.T) {
	cluster := &envoyclusterv3.Cluster{
		Name: "openai-cluster",
	}

	model := "gpt-4"
	aiBackend := &v1alpha1.AIBackend{
		LLM: &v1alpha1.LLMProvider{
			OpenAI: &v1alpha1.OpenAIConfig{
				Model: &model,
				AuthToken: v1alpha1.SingleAuthToken{
					Kind:   v1alpha1.Inline,
					Inline: ptr.To("test-token"),
				},
			},
		},
	}

	secrets := &ir.Secret{}
	multiSecrets := map[string]*ir.Secret{}

	err := ProcessAIBackend(aiBackend, secrets, multiSecrets, cluster)

	require.NoError(t, err)

	// Verify cluster type
	assert.Equal(t, envoyclusterv3.Cluster_STRICT_DNS, cluster.GetType())

	// Verify load assignment
	require.NotNil(t, cluster.LoadAssignment)
	assert.Equal(t, cluster.Name, cluster.LoadAssignment.ClusterName)
	require.Len(t, cluster.LoadAssignment.Endpoints, 1)

	// Verify endpoint
	endpoints := cluster.LoadAssignment.Endpoints[0].LbEndpoints
	require.Len(t, endpoints, 1)

	endpoint := endpoints[0]
	hostEndpoint := endpoint.GetEndpoint()
	require.NotNil(t, hostEndpoint)

	// Verify socket address
	address := hostEndpoint.Address.GetSocketAddress()
	require.NotNil(t, address)
	assert.Equal(t, "api.openai.com", address.Address)
	assert.Equal(t, uint32(443), address.GetPortValue())

	// Verify metadata (auth token and model)
	require.NotNil(t, endpoint.Metadata)
	filterMeta := endpoint.Metadata.FilterMetadata["io.solo.transformation"]
	require.NotNil(t, filterMeta)
	assert.Equal(t, "test-token", filterMeta.Fields["auth_token"].GetStringValue())
	assert.Equal(t, "gpt-4", filterMeta.Fields["model"].GetStringValue())

	// Verify Transport Socket Matches
	require.GreaterOrEqual(t, len(cluster.TransportSocketMatches), 2)

	// One should be for TLS
	tlsMatch := findTransportSocketMatchByPrefix(cluster.TransportSocketMatches, "tls")
	require.NotNil(t, tlsMatch)

	// Another for plaintext
	plaintextMatch := findTransportSocketMatchByName(cluster.TransportSocketMatches, "plaintext")
	require.NotNil(t, plaintextMatch)
}

func TestProcessAIBackend_Anthropic(t *testing.T) {
	cluster := &envoyclusterv3.Cluster{
		Name: "anthropic-cluster",
	}

	model := "claude-3-opus-20240229"
	aiBackend := &v1alpha1.AIBackend{
		LLM: &v1alpha1.LLMProvider{
			Anthropic: &v1alpha1.AnthropicConfig{
				Model: &model,
				AuthToken: v1alpha1.SingleAuthToken{
					Kind:   v1alpha1.Inline,
					Inline: ptr.To("anthropic-token"),
				},
			},
		},
	}

	secrets := &ir.Secret{}
	multiSecrets := map[string]*ir.Secret{}

	err := ProcessAIBackend(aiBackend, secrets, multiSecrets, cluster)

	require.NoError(t, err)

	// Verify endpoint
	endpoints := cluster.LoadAssignment.Endpoints[0].LbEndpoints
	require.Len(t, endpoints, 1)

	endpoint := endpoints[0]
	hostEndpoint := endpoint.GetEndpoint()
	require.NotNil(t, hostEndpoint)

	// Verify socket address
	address := hostEndpoint.Address.GetSocketAddress()
	require.NotNil(t, address)
	assert.Equal(t, "api.anthropic.com", address.Address)
	assert.Equal(t, uint32(443), address.GetPortValue())

	// Verify metadata
	require.NotNil(t, endpoint.Metadata)
	filterMeta := endpoint.Metadata.FilterMetadata["io.solo.transformation"]
	require.NotNil(t, filterMeta)
	assert.Equal(t, "anthropic-token", filterMeta.Fields["auth_token"].GetStringValue())
	assert.Equal(t, "claude-3-opus-20240229", filterMeta.Fields["model"].GetStringValue())
}

func TestProcessAIBackend_AzureOpenAI(t *testing.T) {
	cluster := &envoyclusterv3.Cluster{
		Name: "azure-openai-cluster",
	}

	aiBackend := &v1alpha1.AIBackend{
		LLM: &v1alpha1.LLMProvider{
			AzureOpenAI: &v1alpha1.AzureOpenAIConfig{
				Endpoint:       "myendpoint.openai.azure.com",
				DeploymentName: "gpt-4-deployment",
				ApiVersion:     "2023-05-15",
				AuthToken: v1alpha1.SingleAuthToken{
					Kind:   v1alpha1.Inline,
					Inline: ptr.To("azure-token"),
				},
			},
		},
	}

	secrets := &ir.Secret{}
	multiSecrets := map[string]*ir.Secret{}

	err := ProcessAIBackend(aiBackend, secrets, multiSecrets, cluster)

	require.NoError(t, err)

	// Verify endpoint
	endpoints := cluster.LoadAssignment.Endpoints[0].LbEndpoints
	require.Len(t, endpoints, 1)

	endpoint := endpoints[0]
	hostEndpoint := endpoint.GetEndpoint()
	require.NotNil(t, hostEndpoint)

	// Verify socket address
	address := hostEndpoint.Address.GetSocketAddress()
	require.NotNil(t, address)
	assert.Equal(t, "myendpoint.openai.azure.com", address.Address)
	assert.Equal(t, uint32(443), address.GetPortValue())

	// Verify metadata
	require.NotNil(t, endpoint.Metadata)
	filterMeta := endpoint.Metadata.FilterMetadata["io.solo.transformation"]
	require.NotNil(t, filterMeta)
	assert.Equal(t, "azure-token", filterMeta.Fields["auth_token"].GetStringValue())
	assert.Equal(t, "gpt-4-deployment", filterMeta.Fields["model"].GetStringValue())
	assert.Equal(t, "2023-05-15", filterMeta.Fields["api_version"].GetStringValue())
}

func TestProcessAIBackend_Gemini(t *testing.T) {
	cluster := &envoyclusterv3.Cluster{
		Name: "gemini-cluster",
	}

	aiBackend := &v1alpha1.AIBackend{
		LLM: &v1alpha1.LLMProvider{
			Gemini: &v1alpha1.GeminiConfig{
				Model:      "gemini-pro",
				ApiVersion: "v1",
				AuthToken: v1alpha1.SingleAuthToken{
					Kind:   v1alpha1.Inline,
					Inline: ptr.To("gemini-token"),
				},
			},
		},
	}

	secrets := &ir.Secret{}
	multiSecrets := map[string]*ir.Secret{}

	err := ProcessAIBackend(aiBackend, secrets, multiSecrets, cluster)

	require.NoError(t, err)

	// Verify endpoint
	endpoints := cluster.LoadAssignment.Endpoints[0].LbEndpoints
	require.Len(t, endpoints, 1)

	endpoint := endpoints[0]
	hostEndpoint := endpoint.GetEndpoint()
	require.NotNil(t, hostEndpoint)

	// Verify socket address
	address := hostEndpoint.Address.GetSocketAddress()
	require.NotNil(t, address)
	assert.Equal(t, "generativelanguage.googleapis.com", address.Address)
	assert.Equal(t, uint32(443), address.GetPortValue())

	// Verify metadata
	require.NotNil(t, endpoint.Metadata)
	filterMeta := endpoint.Metadata.FilterMetadata["io.solo.transformation"]
	require.NotNil(t, filterMeta)
	assert.Equal(t, "gemini-token", filterMeta.Fields["auth_token"].GetStringValue())
	assert.Equal(t, "gemini-pro", filterMeta.Fields["model"].GetStringValue())
	assert.Equal(t, "v1", filterMeta.Fields["api_version"].GetStringValue())
}

func TestProcessAIBackend_VertexAI(t *testing.T) {
	cluster := &envoyclusterv3.Cluster{
		Name: "vertex-ai-cluster",
	}

	aiBackend := &v1alpha1.AIBackend{
		LLM: &v1alpha1.LLMProvider{
			VertexAI: &v1alpha1.VertexAIConfig{
				Model:      "gemini-1.5-pro",
				ApiVersion: "v1",
				Location:   "us-central1",
				ProjectId:  "my-project",
				Publisher:  v1alpha1.GOOGLE,
				AuthToken: v1alpha1.SingleAuthToken{
					Kind:   v1alpha1.Inline,
					Inline: ptr.To("vertex-token"),
				},
			},
		},
	}

	secrets := &ir.Secret{}
	multiSecrets := map[string]*ir.Secret{}

	err := ProcessAIBackend(aiBackend, secrets, multiSecrets, cluster)

	require.NoError(t, err)

	// Verify endpoint
	endpoints := cluster.LoadAssignment.Endpoints[0].LbEndpoints
	require.Len(t, endpoints, 1)

	endpoint := endpoints[0]
	hostEndpoint := endpoint.GetEndpoint()
	require.NotNil(t, hostEndpoint)

	// Verify socket address
	address := hostEndpoint.Address.GetSocketAddress()
	require.NotNil(t, address)
	assert.Equal(t, "us-central1-aiplatform.googleapis.com", address.Address)
	assert.Equal(t, uint32(443), address.GetPortValue())

	// Verify metadata
	require.NotNil(t, endpoint.Metadata)
	filterMeta := endpoint.Metadata.FilterMetadata["io.solo.transformation"]
	require.NotNil(t, filterMeta)
	assert.Equal(t, "vertex-token", filterMeta.Fields["auth_token"].GetStringValue())
	assert.Equal(t, "gemini-1.5-pro", filterMeta.Fields["model"].GetStringValue())
	assert.Equal(t, "v1", filterMeta.Fields["api_version"].GetStringValue())
	assert.Equal(t, "us-central1", filterMeta.Fields["location"].GetStringValue())
	assert.Equal(t, "my-project", filterMeta.Fields["project"].GetStringValue())
	assert.Equal(t, "google", filterMeta.Fields["publisher"].GetStringValue())
}

func TestProcessAIBackend_CustomURL(t *testing.T) {
	cluster := &envoyclusterv3.Cluster{
		Name: "custom-host-cluster",
	}

	model := "gpt-4"
	path := "/api/v1/chat/completions"
	prefix := "Bearer"
	header := "Authorization"
	aiBackend := &v1alpha1.AIBackend{
		LLM: &v1alpha1.LLMProvider{
			Host: ptr.To("custom-openai-host.example.com"),
			Port: ptr.To(gwv1.PortNumber(8443)),
			Path: &v1alpha1.PathOverride{Full: &path},
			AuthHeader: &v1alpha1.AuthHeader{
				Prefix:     &prefix,
				HeaderName: &header,
			},
			OpenAI: &v1alpha1.OpenAIConfig{
				Model: &model,
				AuthToken: v1alpha1.SingleAuthToken{
					Kind:   v1alpha1.Inline,
					Inline: ptr.To("test-token"),
				},
			},
		},
	}

	secrets := &ir.Secret{}
	multiSecrets := map[string]*ir.Secret{}

	err := ProcessAIBackend(aiBackend, secrets, multiSecrets, cluster)

	require.NoError(t, err)

	// Verify endpoint with custom host
	endpoints := cluster.LoadAssignment.Endpoints[0].LbEndpoints
	require.Len(t, endpoints, 1)

	endpoint := endpoints[0]
	hostEndpoint := endpoint.GetEndpoint()
	require.NotNil(t, hostEndpoint)

	// Verify socket address uses custom host and port
	address := hostEndpoint.Address.GetSocketAddress()
	require.NotNil(t, address)
	assert.Equal(t, "custom-openai-host.example.com", address.Address)
	assert.Equal(t, uint32(8443), address.GetPortValue())
}

func TestProcessAIBackend_MultiPool(t *testing.T) {
	cluster := &envoyclusterv3.Cluster{
		Name: "multi-pool-cluster",
	}

	// Create a multi-pool with 2 priorities, each with OpenAI endpoints
	model1 := "gpt-4"
	model2 := "gpt-3.5-turbo"

	aiBackend := &v1alpha1.AIBackend{
		PriorityGroups: []v1alpha1.PriorityGroup{
			{
				Providers: []v1alpha1.NamedLLMProvider{
					{
						Name: "openai-primary",
						LLMProvider: v1alpha1.LLMProvider{
							OpenAI: &v1alpha1.OpenAIConfig{
								Model: &model1,
								AuthToken: v1alpha1.SingleAuthToken{
									Kind:   v1alpha1.Inline,
									Inline: ptr.To("primary-token"),
								},
							},
						},
					},
				},
			},
			{
				Providers: []v1alpha1.NamedLLMProvider{
					{
						Name: "openai-fallback",
						LLMProvider: v1alpha1.LLMProvider{
							OpenAI: &v1alpha1.OpenAIConfig{
								Model: &model2,
								AuthToken: v1alpha1.SingleAuthToken{
									Kind:   v1alpha1.Inline,
									Inline: ptr.To("fallback-token"),
								},
							},
						},
					},
				},
			},
		},
	}

	secrets := &ir.Secret{}
	multiSecrets := map[string]*ir.Secret{}

	err := ProcessAIBackend(aiBackend, secrets, multiSecrets, cluster)

	require.NoError(t, err)

	// Verify we have 2 locality endpoints (priorities)
	require.Len(t, cluster.LoadAssignment.Endpoints, 2)

	// Verify first priority
	priority0 := cluster.LoadAssignment.Endpoints[0]
	assert.Equal(t, uint32(0), priority0.Priority)
	require.Len(t, priority0.LbEndpoints, 1)

	endpoint0 := priority0.LbEndpoints[0]
	address0 := endpoint0.GetEndpoint().Address.GetSocketAddress()
	require.NotNil(t, address0)
	assert.Equal(t, "api.openai.com", address0.Address)

	metadata0 := endpoint0.Metadata.FilterMetadata["io.solo.transformation"]
	require.NotNil(t, metadata0)
	assert.Equal(t, "primary-token", metadata0.Fields["auth_token"].GetStringValue())
	assert.Equal(t, "gpt-4", metadata0.Fields["model"].GetStringValue())

	// Verify second priority
	priority1 := cluster.LoadAssignment.Endpoints[1]
	assert.Equal(t, uint32(1), priority1.Priority)
	require.Len(t, priority1.LbEndpoints, 1)

	endpoint1 := priority1.LbEndpoints[0]
	address1 := endpoint1.GetEndpoint().Address.GetSocketAddress()
	require.NotNil(t, address1)
	assert.Equal(t, "api.openai.com", address1.Address)

	metadata1 := endpoint1.Metadata.FilterMetadata["io.solo.transformation"]
	require.NotNil(t, metadata1)
	assert.Equal(t, "fallback-token", metadata1.Fields["auth_token"].GetStringValue())
	assert.Equal(t, "gpt-3.5-turbo", metadata1.Fields["model"].GetStringValue())
}

// findTransportSocketMatchByPrefix finds a transport socket match with a name starting with prefix
func findTransportSocketMatchByPrefix(matches []*envoyclusterv3.Cluster_TransportSocketMatch, prefix string) *envoyclusterv3.Cluster_TransportSocketMatch {
	for _, match := range matches {
		if strings.HasPrefix(match.Name, prefix) {
			return match
		}
	}
	return nil
}

// findTransportSocketMatchByName finds a transport socket match with exact name
func findTransportSocketMatchByName(matches []*envoyclusterv3.Cluster_TransportSocketMatch, name string) *envoyclusterv3.Cluster_TransportSocketMatch {
	for _, match := range matches {
		if match.Name == name {
			return match
		}
	}
	return nil
}
