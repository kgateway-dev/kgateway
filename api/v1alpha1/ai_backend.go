package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// AIBackend specifies the AI backend configuration
// +kubebuilder:validation:ExactlyOneOf=llm;priorityGroups
type AIBackend struct {
	// The LLM configures the AI gateway to use a single LLM provider backend.
	// +optional
	LLM *LLMProvider `json:"llm,omitempty"`

	// PriorityGroups specifies a list of groups in priority order where each group defines
	// a set of LLM providers. The priority determines the priority of the backend endpoints chosen.
	// Note: provider names must be unique across all providers in all priority groups. Backend policies
	// may target a specific provider by name using targetRefs[].sectionName.
	//
	// Example configuration with two priority groups:
	// ```yaml
	// priorityGroups:
	//	- providers:
	//	  - azureOpenai:
	//	      deploymentName: gpt-4o-mini
	//	      apiVersion: 2024-02-15-preview
	//	      endpoint: ai-gateway.openai.azure.com
	//	      authToken:
	//	        secretRef:
	//	          name: azure-secret
	//	          namespace: kgateway-system
	//	- providers:
	//	  - azureOpenai:
	//	      deploymentName: gpt-4o-mini-2
	//	      apiVersion: 2024-02-15-preview
	//	      endpoint: ai-gateway-2.openai.azure.com
	//	      authToken:
	//	        secretRef:
	//	          name: azure-secret-2
	//	          namespace: kgateway-system
	// ```
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	// TODO: enable this rule when we don't need to support older k8s versions where this rule breaks // +kubebuilder:validation:XValidation:message="provider names must be unique across groups",rule="self.map(pg, pg.providers.map(pp, pp.name)).map(p, self.map(pg, pg.providers.map(pp, pp.name)).filter(cp, cp != p).exists(cp, p.exists(pn, pn in cp))).exists(p, !p)"
	PriorityGroups []PriorityGroup `json:"priorityGroups,omitempty"`
}

// LLMProvider specifies the target large language model provider that the backend should route requests to.
// +kubebuilder:validation:ExactlyOneOf=openai;azureopenai;anthropic;gemini;vertexai;bedrock
// +kubebuilder:validation:XValidation:rule="has(self.host) || has(self.port) ? has(self.host) && has(self.port) : true",message="both host and port must be set together"
// TODO: Move auth options off of SupportedLLMProvider to BackendConfigPolicy: https://github.com/kgateway-dev/kgateway/issues/11930
type LLMProvider struct {
	// OpenAI provider
	// +optional
	OpenAI *OpenAIConfig `json:"openai,omitempty"`

	// Azure OpenAI provider
	// +optional
	AzureOpenAI *AzureOpenAIConfig `json:"azureopenai,omitempty"`

	// Anthropic provider
	// +optional
	Anthropic *AnthropicConfig `json:"anthropic,omitempty"`

	// Gemini provider
	// +optional
	Gemini *GeminiConfig `json:"gemini,omitempty"`

	// Vertex AI provider
	// +optional
	VertexAI *VertexAIConfig `json:"vertexai,omitempty"`

	// Bedrock provider
	// +optional
	Bedrock *BedrockConfig `json:"bedrock,omitempty"`

	// Host specifies the hostname to send the requests to.
	// If not specified, the default hostname for the provider is used.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Host *string `json:"host,omitempty"`

	// Port specifies the port to send the requests to.
	// +optional
	Port *gwv1.PortNumber `json:"port,omitempty"`

	// Path specifies the URL path to use for the LLM provider API requests.
	// This is useful when you need to route requests to a different API endpoint while maintaining
	// compatibility with the original provider's API structure.
	// If not specified, the default path for the provider is used.
	// +optional
	Path *PathOverride `json:"path,omitempty"`

	// AuthHeader specifies how the Authorization header is set in the request sent to the LLM provider.
	// Allows changing the header name and/or the prefix (e.g., "Bearer").
	// Note: Not all LLM providers use the Authorization header and prefix.
	// For example, OpenAI uses header: "Authorization" and prefix: "Bearer" But Azure OpenAI uses header: "api-key"
	// and no Bearer.
	AuthHeader *AuthHeader `json:"authHeader,omitempty"`
}

// NamedLLMProvider wraps an LLMProvider with a name.
type NamedLLMProvider struct {
	// Name of the provider. Policies can target this provider by name.
	Name gwv1.SectionName `json:"name"`

	LLMProvider `json:",inline"`
}

// PathOverride allows overriding the default URL path used for LLM provider API requests.
type PathOverride struct {
	// +kubebuilder:validation:MinLength=1
	Full *string `json:"full,omitempty"`
}

// AuthHeader allows customization of the default Authorization header sent to the LLM Provider.
// The default header is `Authorization: Bearer <token>`. HeaderName can change the Authorization
// header name and Prefix can change the Bearer prefix
// +kubebuilder:validation:XValidation:rule="has(self.prefix) || has(self.headerName)",message="at least one of prefix or headerName must be set"
type AuthHeader struct {
	// Prefix specifies the prefix to use in the Authorization header.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Prefix *string `json:"prefix,omitempty"`

	// HeaderName specifies the name of the header to use for authorization.
	// +optional
	// +kubebuilder:validation:MinLength=1
	HeaderName *string `json:"headerName,omitempty"`
}

type SingleAuthTokenKind string

const (
	// Inline provides the token directly in the configuration for the Backend.
	Inline SingleAuthTokenKind = "Inline"

	// SecretRef provides the token directly in the configuration for the Backend.
	SecretRef SingleAuthTokenKind = "SecretRef"

	// Passthrough the existing token. This token can either
	// come directly from the client, or be generated by an OIDC flow
	// early in the request lifecycle. This option is useful for
	// backends which have federated identity setup and can re-use
	// the token from the client.
	// Currently, this token must exist in the `Authorization` header.
	Passthrough SingleAuthTokenKind = "Passthrough"
)

// SingleAuthToken configures the authorization token that the AI gateway uses to access the LLM provider API.
// This token is automatically sent in a request header, depending on the LLM provider.
//
// +kubebuilder:validation:AtMostOneOf=inline;secretRef
type SingleAuthToken struct {
	// Kind specifies which type of authorization token is being used.
	// Must be one of: "Inline", "SecretRef", "Passthrough".
	//
	// +kubebuilder:validation:Enum=Inline;SecretRef;Passthrough
	Kind SingleAuthTokenKind `json:"kind"`

	// Provide the token directly in the configuration for the Backend.
	// This option is the least secure. Only use this option for quick tests such as trying out AI Gateway.
	Inline *string `json:"inline,omitempty"`

	// Store the API key in a Kubernetes secret in the same namespace as the Backend.
	// Then, refer to the secret in the Backend configuration. This option is more secure than an inline token,
	// because the API key is encoded and you can restrict access to secrets through RBAC rules.
	// You might use this option in proofs of concept, controlled development and staging environments,
	// or well-controlled prod environments that use secrets.
	SecretRef *corev1.LocalObjectReference `json:"secretRef,omitempty"`
}

// OpenAIConfig settings for the [OpenAI](https://platform.openai.com/docs/api-reference/streaming) LLM provider.
type OpenAIConfig struct {
	// The authorization token that the AI gateway uses to access the OpenAI API.
	// This token is automatically sent in the `Authorization` header of the
	// request and prefixed with `Bearer`.
	// +required
	AuthToken SingleAuthToken `json:"authToken"`
	// Optional: Override the model name, such as `gpt-4o-mini`.
	// If unset, the model name is taken from the request.
	// This setting can be useful when setting up model failover within the same LLM provider.
	Model *string `json:"model,omitempty"`
}

// AzureOpenAIConfig settings for the [Azure OpenAI](https://learn.microsoft.com/en-us/azure/ai-services/openai/) LLM provider.
type AzureOpenAIConfig struct {
	// The authorization token that the AI gateway uses to access the Azure OpenAI API.
	// This token is automatically sent in the `api-key` header of the request.
	// +required
	AuthToken SingleAuthToken `json:"authToken"`

	// The endpoint for the Azure OpenAI API to use, such as `my-endpoint.openai.azure.com`.
	// If the scheme is included, it is stripped.
	// +required
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`

	// The name of the Azure OpenAI model deployment to use.
	// For more information, see the [Azure OpenAI model docs](https://learn.microsoft.com/en-us/azure/ai-services/openai/concepts/models).
	// +required
	// +kubebuilder:validation:MinLength=1
	DeploymentName string `json:"deploymentName"`

	// The version of the Azure OpenAI API to use.
	// For more information, see the [Azure OpenAI API version reference](https://learn.microsoft.com/en-us/azure/ai-services/openai/reference#api-specs).
	// +required
	// +kubebuilder:validation:MinLength=1
	ApiVersion string `json:"apiVersion"`
}

// GeminiConfig settings for the [Gemini](https://ai.google.dev/gemini-api/docs) LLM provider.
type GeminiConfig struct {
	// The authorization token that the AI gateway uses to access the Gemini API.
	// This token is automatically sent in the `key` query parameter of the request.
	// +required
	AuthToken SingleAuthToken `json:"authToken"`

	// The Gemini model to use.
	// For more information, see the [Gemini models docs](https://ai.google.dev/gemini-api/docs/models/gemini).
	// +required
	Model string `json:"model"`

	// The version of the Gemini API to use.
	// For more information, see the [Gemini API version docs](https://ai.google.dev/gemini-api/docs/api-versions).
	// +required
	ApiVersion string `json:"apiVersion"`
}

// Publisher configures the type of publisher model to use for VertexAI. Currently, only Google is supported.
type Publisher string

const GOOGLE Publisher = "GOOGLE"

// VertexAIConfig settings for the [Vertex AI](https://cloud.google.com/vertex-ai/docs) LLM provider.
// To find the values for the project ID, project location, and publisher, you can check the fields of an API request, such as
// `https://{LOCATION}-aiplatform.googleapis.com/{VERSION}/projects/{PROJECT_ID}/locations/{LOCATION}/publishers/{PROVIDER}/<model-path>`.
type VertexAIConfig struct {
	// The authorization token that the AI gateway uses to access the Vertex AI API.
	// This token is automatically sent in the `key` header of the request.
	// +required
	AuthToken SingleAuthToken `json:"authToken"`

	// The Vertex AI model to use.
	// For more information, see the [Vertex AI model docs](https://cloud.google.com/vertex-ai/generative-ai/docs/learn/models).
	// +required
	// +kubebuilder:validation:MinLength=1
	Model string `json:"model"`

	// The version of the Vertex AI API to use.
	// For more information, see the [Vertex AI API reference](https://cloud.google.com/vertex-ai/docs/reference#versions).
	// +required
	// +kubebuilder:validation:MinLength=1
	ApiVersion string `json:"apiVersion"`

	// The ID of the Google Cloud Project that you use for the Vertex AI.
	// +required
	// +kubebuilder:validation:MinLength=1
	ProjectId string `json:"projectId"`

	// The location of the Google Cloud Project that you use for the Vertex AI.
	// +required
	// +kubebuilder:validation:MinLength=1
	Location string `json:"location"`

	// Optional: The model path to route to. Defaults to the Gemini model path, `generateContent`.
	ModelPath *string `json:"modelPath,omitempty"`

	// The type of publisher model to use. Currently, only Google is supported.
	// +kubebuilder:validation:Enum=GOOGLE
	Publisher Publisher `json:"publisher"`
}

// AnthropicConfig settings for the [Anthropic](https://docs.anthropic.com/en/release-notes/api) LLM provider.
type AnthropicConfig struct {
	// The authorization token that the AI gateway uses to access the Anthropic API.
	// This token is automatically sent in the `x-api-key` header of the request.
	// +required
	AuthToken SingleAuthToken `json:"authToken"`
	// Optional: A version header to pass to the Anthropic API.
	// For more information, see the [Anthropic API versioning docs](https://docs.anthropic.com/en/api/versioning).
	Version string `json:"apiVersion,omitempty"`
	// Optional: Override the model name.
	// If unset, the model name is taken from the request.
	// This setting can be useful when testing model failover scenarios.
	Model *string `json:"model,omitempty"`
}

type BedrockConfig struct {
	// Auth specifies an explicit AWS authentication method for the backend.
	// When omitted, the following credential providers are tried in order, stopping when one
	// of them returns an access key ID and a secret access key (the session token is optional):
	// 1. Environment variables: when the environment variables AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, and AWS_SESSION_TOKEN are set.
	// 2. AssumeRoleWithWebIdentity API call: when the environment variables AWS_WEB_IDENTITY_TOKEN_FILE and AWS_ROLE_ARN are set.
	// 3. EKS Pod Identity: when the environment variable AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE is set.
	//
	// See the Envoy docs for more info:
	// https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/aws_request_signing_filter#credentials
	//
	// +optional
	Auth *AwsAuth `json:"auth,omitempty"`

	// The model field is the supported model id published by AWS. See <https://docs.aws.amazon.com/bedrock/latest/userguide/models-supported.html>
	// +kubebuilder:validation:MinLength=1
	Model string `json:"model"`

	// Region is the AWS region to use for the backend.
	// Defaults to us-east-1 if not specified.
	// +optional
	// +kubebuilder:default=us-east-1
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern="^[a-z0-9-]+$"
	Region string `json:"region,omitempty"`

	// Guardrail configures the Guardrail policy to use for the backend. See <https://docs.aws.amazon.com/bedrock/latest/userguide/guardrails.html>
	// If not specified, the AWS Guardrail policy will not be used.
	// +optional
	Guardrail *AWSGuardrailConfig `json:"guardrail,omitempty"`
}

type AWSGuardrailConfig struct {
	// GuardrailIdentifier is the identifier of the Guardrail policy to use for the backend.
	// +kubebuilder:validation:MinLength=1
	GuardrailIdentifier string `json:"identifier"`

	// GuardrailVersion is the version of the Guardrail policy to use for the backend.
	// +kubebuilder:validation:MinLength=1
	GuardrailVersion string `json:"version"`
}

// MultiPoolConfig configures the backends for multiple hosts or models from the same provider in one Backend resource.
// This method can be useful for creating one logical endpoint that is backed
// by multiple hosts or models.
//
// In the `priorities` section, the order of `pool` entries defines the priority of the backend endpoints.
// The `pool` entries can either define a list of backends or a single backend.
// Note: Only two levels of nesting are permitted. Any nested entries after the second level are ignored.
type PriorityGroup struct {
	// A list of LLM provider backends within a single endpoint pool entry.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	// +kubebuilder:validation:XValidation:message="provider names must be unique within a group",rule="self.all(p1, self.exists_one(p2, p1.name == p2.name))"
	Providers []NamedLLMProvider `json:"providers,omitempty"`
}
