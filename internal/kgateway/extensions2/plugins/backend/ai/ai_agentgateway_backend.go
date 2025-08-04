package ai

import (
	"fmt"
	"strings"

	"github.com/agentgateway/agentgateway/go/api"
	wrappers "google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

func ProcessAIBackendForAgentGateway(ctx krt.HandlerContext, be *v1alpha1.Backend, secrets krt.Collection[*corev1.Secret]) ([]*api.Backend, []*api.Policy, error) {
	if be.Spec.AI == nil {
		return nil, nil, fmt.Errorf("ai backend spec must not be nil for AI backend type")
	}

	// Extract the provider configuration
	var authPolicy *api.Policy
	var aiBackend *api.Backend

	if be.Spec.AI.LLM != nil {
		aiBackend, authPolicy = buildAIBackendFromLLM(ctx, be.Namespace, be.Name, be.Spec.AI.LLM, secrets)
	} else if be.Spec.AI.MultiPool != nil && len(be.Spec.AI.MultiPool.Priorities) > 0 &&
		len(be.Spec.AI.MultiPool.Priorities[0].Pool) > 0 {
		// For MultiPool, use the first provider from the first priority pool
		aiBackend, authPolicy = buildAIBackendFromLLM(ctx, be.Namespace, be.Name, &be.Spec.AI.MultiPool.Priorities[0].Pool[0], secrets)
	} else {
		return nil, nil, fmt.Errorf("AI backend has no valid LLM or MultiPool configuration")
	}

	return []*api.Backend{aiBackend}, []*api.Policy{authPolicy}, nil
}

// buildAIBackendFromLLM converts a kgateway LLMProvider to an agentgateway AIBackend
func buildAIBackendFromLLM(
	ctx krt.HandlerContext,
	namespace, name string,
	llm *v1alpha1.LLMProvider,
	secrets krt.Collection[*corev1.Secret]) (*api.Backend, *api.Policy) {
	beName := namespace + "/" + name
	// Create AIBackend structure with provider-specific configuration
	aiBackend := &api.AIBackend{}

	// Extract and set provider configuration based on the LLM provider type
	provider := llm.Provider

	var auth *api.BackendAuthPolicy
	if provider.OpenAI != nil {
		var model *wrappers.StringValue
		if provider.OpenAI.Model != nil {
			model = &wrappers.StringValue{
				Value: *provider.OpenAI.Model,
			}
		}
		aiBackend.Provider = &api.AIBackend_Openai{
			Openai: &api.AIBackend_OpenAI{
				Model: model,
			},
		}
		auth = buildAuthPolicy(ctx, &provider.OpenAI.AuthToken, secrets, namespace)
	} else if provider.AzureOpenAI != nil {
		aiBackend.Provider = &api.AIBackend_Openai{
			Openai: &api.AIBackend_OpenAI{},
		}
		auth = buildAuthPolicy(ctx, &provider.AzureOpenAI.AuthToken, secrets, namespace)
	} else if provider.Anthropic != nil {
		var model *wrappers.StringValue
		if provider.Anthropic.Model != nil {
			model = &wrappers.StringValue{
				Value: *provider.Anthropic.Model,
			}
		}
		aiBackend.Provider = &api.AIBackend_Anthropic_{
			Anthropic: &api.AIBackend_Anthropic{
				Model: model,
			},
		}
		auth = buildAuthPolicy(ctx, &provider.Anthropic.AuthToken, secrets, namespace)
	} else if provider.Gemini != nil {
		model := &wrappers.StringValue{
			Value: provider.Gemini.Model,
		}
		aiBackend.Provider = &api.AIBackend_Gemini_{
			Gemini: &api.AIBackend_Gemini{
				Model: model,
			},
		}
		auth = buildAuthPolicy(ctx, &provider.Gemini.AuthToken, secrets, namespace)
	} else if provider.VertexAI != nil {
		model := &wrappers.StringValue{
			Value: provider.VertexAI.Model,
		}
		aiBackend.Provider = &api.AIBackend_Vertex_{
			Vertex: &api.AIBackend_Vertex{
				Model: model,
			},
		}
		auth = buildAuthPolicy(ctx, &provider.VertexAI.AuthToken, secrets, namespace)
	}
	// TODO: add bedrock support

	// Map common override configurations
	if llm.HostOverride != nil {
		aiBackend.Override = &api.AIBackend_Override{
			Host: llm.HostOverride.Host,
			Port: int32(llm.HostOverride.Port),
		}
	}

	return &api.Backend{
			Name: beName,
			Kind: &api.Backend_Ai{
				Ai: aiBackend,
			},
		}, &api.Policy{
			Name: fmt.Sprintf("auth-%s", beName),
			Target: &api.PolicyTarget{Kind: &api.PolicyTarget_Backend{
				Backend: beName,
			}},
			Spec: &api.PolicySpec{Kind: &api.PolicySpec_Auth{
				Auth: auth,
			}},
		}
}

// buildAuthPolicy creates auth policy for the given auth token configuration
func buildAuthPolicy(ctx krt.HandlerContext, authToken *v1alpha1.SingleAuthToken, secrets krt.Collection[*corev1.Secret], namespace string) *api.BackendAuthPolicy {
	if authToken == nil {
		return nil
	}

	switch authToken.Kind {
	case v1alpha1.SecretRef:
		if authToken.SecretRef == nil {
			return nil
		}

		// Build the secret key in namespace/name format
		secretKey := namespace + "/" + authToken.SecretRef.Name
		secret := krt.FetchOne(ctx, secrets, krt.FilterKey(secretKey))
		if secret == nil {
			// Return nil auth policy if secret not found - this will be handled upstream
			return nil
		}

		// Extract the authorization key from the secret data
		authKey := ""
		if (*secret).Data != nil {
			if val, ok := (*secret).Data["Authorization"]; ok {
				// Strip the "Bearer " prefix if present, as it will be added by the provider
				authValue := strings.TrimSpace(string(val))
				authKey = strings.TrimSpace(strings.TrimPrefix(authValue, "Bearer "))
			}
		}

		if authKey == "" {
			return nil
		}

		return &api.BackendAuthPolicy{
			Kind: &api.BackendAuthPolicy_Key{
				Key: &api.Key{Secret: authKey},
			},
		}
	case v1alpha1.Passthrough:
		return &api.BackendAuthPolicy{
			Kind: &api.BackendAuthPolicy_Passthrough{},
		}
	default:
		return nil
	}
}
