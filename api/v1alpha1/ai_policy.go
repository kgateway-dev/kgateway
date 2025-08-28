package v1alpha1

import (
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// AIPolicy config is used to configure the behavior of the LLM provider
// on the level of individual routes. These route settings, such as prompt enrichment,
// retrieval augmented generation (RAG), and semantic caching, are applicable only
// for routes that send requests to an LLM provider backend.
type AIPolicy struct {
	// Enrich requests sent to the LLM provider by appending and prepending system prompts.
	// This can be configured only for LLM providers that use the `CHAT` or `CHAT_STREAMING` API route type.
	PromptEnrichment *AIPromptEnrichment `json:"promptEnrichment,omitempty"`

	// Set up prompt guards to block unwanted requests to the LLM provider and mask sensitive data.
	// Prompt guards can be used to reject requests based on the content of the prompt, as well as
	// mask responses based on the content of the response.
	PromptGuard *AIPromptGuard `json:"promptGuard,omitempty"`

	// Provide defaults to merge with user input fields.
	// Defaults do _not_ override the user input fields, unless you explicitly set `override` to `true`.
	Defaults []FieldDefault `json:"defaults,omitempty"`

	// The type of route to the LLM provider API. Currently, `CHAT` and `CHAT_STREAMING` are supported.
	// Note: This field is not applicable when using agentgateway
	// +kubebuilder:validation:Enum=CHAT;CHAT_STREAMING
	// +kubebuilder:default=CHAT
	RouteType *RouteType `json:"routeType,omitempty"`
}

// AIPromptEnrichment defines the config to enrich requests sent to the LLM provider by appending and prepending system prompts.
// This can be configured only for LLM providers that use the `CHAT` or `CHAT_STREAMING` API type.
//
// Prompt enrichment allows you to add additional context to the prompt before sending it to the model.
// Unlike RAG or other dynamic context methods, prompt enrichment is static and is applied to every request.
//
// **Note**: Some providers, including Anthropic, do not support SYSTEM role messages, and instead have a dedicated
// system field in the input JSON. In this case, use the [`defaults` setting](#fielddefault) to set the system field.
//
// The following example prepends a system prompt of `Answer all questions in French.`
// and appends `Describe the painting as if you were a famous art critic from the 17th century.`
// to each request that is sent to the `openai` HTTPRoute.
// ```yaml
//
//	name: openai-opt
//	namespace: kgateway-system
//
// spec:
//
//	targetRefs:
//	- group: gateway.networking.k8s.io
//	  kind: HTTPRoute
//	  name: openai
//	ai:
//	    promptEnrichment:
//	      prepend:
//	      - role: SYSTEM
//	        content: "Answer all questions in French."
//	      append:
//	      - role: USER
//	        content: "Describe the painting as if you were a famous art critic from the 17th century."
//
// ```
type AIPromptEnrichment struct {
	// A list of messages to be prepended to the prompt sent by the client.
	Prepend []Message `json:"prepend,omitempty"`
	// A list of messages to be appended to the prompt sent by the client.
	Append []Message `json:"append,omitempty"`
}

// RouteType is the type of route to the LLM provider API.
type RouteType string

const (
	// The LLM generates the full response before responding to a client.
	CHAT RouteType = "CHAT"
	// Stream responses to a client, which allows the LLM to stream out tokens as they are generated.
	CHAT_STREAMING RouteType = "CHAT_STREAMING"
)

// An entry for a message to prepend or append to each prompt.
type Message struct {
	// Role of the message. The available roles depend on the backend
	// LLM provider model, such as `SYSTEM` or `USER` in the OpenAI API.
	Role string `json:"role"`
	// String content of the message.
	Content string `json:"content"`
}

// BuiltIn regex patterns for specific types of strings in prompts.
// For example, if you specify `CREDIT_CARD`, any credit card numbers
// in the request or response are matched.
// +kubebuilder:validation:Enum=SSN;CREDIT_CARD;PHONE_NUMBER;EMAIL
type BuiltIn string

const (
	// Default regex matching for Social Security numbers.
	SSN BuiltIn = "SSN"
	// Default regex matching for credit card numbers.
	CREDIT_CARD BuiltIn = "CREDIT_CARD"
	// Default regex matching for phone numbers.
	PHONE_NUMBER BuiltIn = "PHONE_NUMBER"
	// Default regex matching for email addresses.
	EMAIL BuiltIn = "EMAIL"
)

// RegexMatch configures the regular expression (regex) matching for prompt guards and data masking.
type RegexMatch struct {
	// The regex pattern to match against the request or response.
	Pattern *string `json:"pattern,omitempty"`
	// An optional name for this match, which can be used for debugging purposes.
	Name *string `json:"name,omitempty"`
}

// Action to take if a regex pattern is matched in a request or response.
// This setting applies only to request matches. PromptguardResponse matches are always masked by default.
type Action string

const (
	// Mask the matched data in the request.
	MASK Action = "MASK"
	// Reject the request if the regex matches content in the request.
	REJECT Action = "REJECT"
)

// Regex configures the regular expression (regex) matching for prompt guards and data masking.
type Regex struct {
	// A list of regex patterns to match against the request or response.
	// Matches and built-ins are additive.
	Matches []RegexMatch `json:"matches,omitempty"`
	// A list of built-in regex patterns to match against the request or response.
	// Matches and built-ins are additive.
	Builtins []BuiltIn `json:"builtins,omitempty"`
	// The action to take if a regex pattern is matched in a request or response.
	// This setting applies only to request matches. PromptguardResponse matches are always masked by default.
	// Defaults to `MASK`.
	// +kubebuilder:default=MASK
	Action *Action `json:"action,omitempty"`
}

// Webhook configures a webhook to forward requests or responses to for prompt guarding.
type Webhook struct {
	// Host to send the traffic to.
	// Note: TLS is not currently supported for webhook.
	// Example:
	// ```yaml
	// host:
	//   host: example.com  #The host name of the webhook endpoint.
	//   port: 443 	        #The port number on which the webhook is listening.
	// ```
	// +required
	Host Host `json:"host"`

	// ForwardHeaders define headers to forward with the request to the webhook.
	// Note: This is not yet supported for agentgateway.
	ForwardHeaders []gwv1.HTTPHeaderMatch `json:"forwardHeaders,omitempty"`
}

// CustomResponse configures a response to return to the client if request content
// is matched against a regex pattern and the action is `REJECT`.
type CustomResponse struct {
	// A custom response message to return to the client. If not specified, defaults to
	// "The request was rejected due to inappropriate content".
	// +kubebuilder:default="The request was rejected due to inappropriate content"
	Message *string `json:"message,omitempty"`

	// The status code to return to the client. Defaults to 403.
	// +kubebuilder:default=403
	// +kubebuilder:validation:Minimum=200
	// +kubebuilder:validation:Maximum=599
	StatusCode *uint32 `json:"statusCode,omitempty"`
}

// Moderation configures an external moderation model endpoint. This endpoint evaluates
// request prompt data against predefined content rules to determine if the content
// adheres to those rules.
//
// Any requests routed through the AI Gateway are processed by the specified
// moderation model. If the model identifies the content as harmful based on its rules,
// the request is automatically rejected.
//
// You can configure a moderation endpoint either as a standalone prompt guard setting
// or alongside other request and response guard settings.
type Moderation struct {
	// Pass prompt data through an external moderation model endpoint,
	// which compares the request prompt input to predefined content rules.
	// Configure an OpenAI moderation endpoint.
	OpenAIModeration *OpenAIConfig `json:"openAIModeration,omitempty"`

	// TODO: support other moderation models
}

// PromptguardRequest defines the prompt guards to apply to requests sent by the client.
// Multiple prompt guard configurations can be set, and they will be executed in the following order:
// webhook → regex → moderation for requests, where each step can reject the request and stop further processing.
type PromptguardRequest struct {
	// A custom response message to return to the client. If not specified, defaults to
	// "The request was rejected due to inappropriate content".
	CustomResponse *CustomResponse `json:"customResponse,omitempty"`

	// Regular expression (regex) matching for prompt guards and data masking.
	Regex *Regex `json:"regex,omitempty"`

	// Configure a webhook to forward requests to for prompt guarding.
	Webhook *Webhook `json:"webhook,omitempty"`

	// Pass prompt data through an external moderation model endpoint,
	// which compares the request prompt input to predefined content rules.
	Moderation *Moderation `json:"moderation,omitempty"`
}

// PromptguardResponse configures the response that the prompt guard applies to responses returned by the LLM provider.
// Both webhook and regex can be set, they will be executed in the following order: webhook → regex, where each step
// can reject the request and stop further processing.
// Note: This is not yet supported for agentgateway.
type PromptguardResponse struct {
	// Regular expression (regex) matching for prompt guards and data masking.
	Regex *Regex `json:"regex,omitempty"`

	// Configure a webhook to forward responses to for prompt guarding.
	Webhook *Webhook `json:"webhook,omitempty"`
}

// AIPromptGuard configures a prompt guards to block unwanted requests to the LLM provider and mask sensitive data.
// Prompt guards can be used to reject requests based on the content of the prompt, as well as
// mask responses based on the content of the response.
//
// This example rejects any request prompts that contain
// the string "credit card", and masks any credit card numbers in the response.
// ```yaml
// promptGuard:
//
//	request:
//	  customResponse:
//	    message: "Rejected due to inappropriate content"
//	  regex:
//	    action: REJECT
//	    matches:
//	    - pattern: "credit card"
//	      name: "CC"
//	response:
//	  regex:
//	    builtins:
//	    - CREDIT_CARD
//	    action: MASK
//
// ```
type AIPromptGuard struct {
	// Prompt guards to apply to requests sent by the client.
	Request *PromptguardRequest `json:"request,omitempty"`
	// Prompt guards to apply to responses returned by the LLM provider.
	Response *PromptguardResponse `json:"response,omitempty"`
}

// FieldDefault provides default values for specific fields in the JSON request body sent to the LLM provider.
// These defaults are merged with the user-provided request to ensure missing fields are populated.
//
// User input fields here refer to the fields in the JSON request body that a client sends when making a request to the LLM provider.
// Defaults set here do _not_ override those user-provided values unless you explicitly set `override` to `true`.
//
// Example: Setting a default system field for Anthropic, which does not support system role messages:
// ```yaml
// defaults:
//   - field: "system"
//     value: "answer all questions in French"
//
// ```
//
// Example: Setting a default temperature and overriding `max_tokens`:
// ```yaml
// defaults:
//   - field: "temperature"
//     value: "0.5"
//   - field: "max_tokens"
//     value: "100"
//     override: true
//
// ```
//
// Example: Overriding a custom list field:
// ```yaml
// defaults:
//   - field: "custom_list"
//     value: "[a,b,c]"
//
// ```
//
// Note: The `field` values correspond to keys in the JSON request body, not fields in this CRD.
type FieldDefault struct {
	// The name of the field.
	// +required
	// +kubebuilder:validation:MinLength=1
	Field string `json:"field"`
	// The field default value, which can be any JSON Data Type.
	// +required
	// +kubebuilder:validation:MinLength=1
	Value string `json:"value"`
	// Whether to override the field's value if it already exists.
	// Defaults to false.
	// +kubebuilder:default=false
	Override *bool `json:"override,omitempty"`
}
