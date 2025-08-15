# OTel Span Attributes for AI Extensions

To enhance the observability of the **AI Gateway API**, we've introduced a new set of **Span Attributes**. These attributes adhere to OpenTelemetry (OTel)'s [Semantic conventions for generative client AI spans](https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-spans/) and 
define custom attributes for AI Gateway-specific features such as **Prompt Guard** and **Webhook Integration**. This will allow us to gain deeper insights into the lifecycle of AI traffic, identify performance bottlenecks, and understand policy application.

## Span Attributes for Standard AI Requests/Responses

We've added rich **Span Attributes** for standard (**non-streaming**) LLM requests and responses passing through the AI Gateway. This ensures consistency with OpenTelemetry standards. 
These attributes help us track core information about AI interactions.

### Standard AI Request

**Span Name**: `gen_ai.request {operation_name} {request_model}`
* `operation_name`: Represents the name of the current AI operation being executed (e.g., chat; generate_content; text_completion).
* `request_model`: Represents the specific model name used in the request (e.g., `gpt-4`).

The `gen_ai.request` span should include the following attributes:

* `gen_ai.operation_name`: The operation name, identical to `operation_name` in the Span Name.
* `gen_ai.system`: The LLM provider (e.g., `OpenAI`, `Anthropic`, `HuggingFace`).
* `gen_ai.output.type`: The expected output type.
* `gen_ai.request.choice.count`: The number of desired generated results in the request.
* `gen_ai.request.model`: The name of the model used in the request.
* `gen_ai.request.seed`: The seed value used for reproducible sampling.
* `gen_ai.request.frequency_penalty`: The frequency penalty parameter.
* `gen_ai.request.max_tokens`: The maximum number of tokens to generate.
* `gen_ai.request.presence_penalty`: The presence penalty parameter.
* `gen_ai.request.stop_sequences`: Stop sequences.
* `gen_ai.request.temperature`: The temperature parameter.
* `gen_ai.request.top_k`: The top_k parameter.
* `gen_ai.request.top_p`: The top_p parameter.

### Standard AI Response

**Span Name**: `gen_ai.response`

The `gen_ai.response` span should include the following response-related attributes:

* `gen_ai.response.id`: The unique ID of the response (e.g., `chatcmpl-8ss8yY3P30yX3WjS3N3B3A3C3`).
* `gen_ai.response.model`: The name of the model actually used to generate the response (e.g., `gpt-4-0613`).
* `gen_ai.response.finish_reasons`: The reason the response completed (e.g., `stop`).
* `gen_ai.usage.input_tokens`: The number of input tokens used in the request (e.g., `150`).
* `gen_ai.usage.output_tokens`: The number of output tokens used in the response (e.g., `75`).

---

## Span Attributes for PromptGuard Features

To provide visibility into the AI Gateway's **PromptGuard** capabilities, we've defined a set of custom attributes. 
These attributes offer critical insights into how requests and responses are handled by the protection mechanisms.

### Webhook

**Span Name**: `handle_request_body_req_webhook` or `handle_response_body_resp_webhook`

**Span Attributes**

* `ai.webhook.endpoint`: The address of the invoked Webhook service, facilitating endpoint identification (e.g., `localhost:1234`).
* `ai.webhook.result`: The decision made by the Webhook based on the Prompt content (`modified`, `rejected`, or `passed`). This is a core attribute for understanding key decision points in the request flow.
* `ai.webhook.reject_reason`: If the request was rejected, this attribute provides the specific reason for rejection, which is crucial for problem diagnosis.

### Regex

**Span Name**: `handle_request_body_req_regex` or `handle_response_body_resp_regex`

**Span Attributes**

* `ai.regex.action`: The user-configured prompt guard action (`mask` or `reject`), indicating the intended behavior.
* `ai.regex.result`: Indicates the outcome of the regular expression guard. It's `reject` if the action is `reject` and the Prompt was indeed rejected; otherwise, it's `pass`. This helps quickly identify rejected traffic.

### Moderation

**Span Name**: `handle_request_body_req_moderation` or `handle_response_body_resp_moderation`

**Span Attributes**

* `ai.moderation.model`: Indicates the model used for moderation (e.g., `omni-moderation-latest`), distinct from the main LLM model.
* `ai.moderation.flagged`: A boolean value indicating whether the request was rejected by the moderation guardrails due to content moderation (`true` or `false`), serving as a direct measure of moderation effectiveness.
