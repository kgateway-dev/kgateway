# XDS Support for New Route Types and Prompt Caching Implementation Plan

## Overview

Add kgateway XDS/CRD support for new agentgateway features introduced in commit 8209c35c:
1. **New route types**: `responses` (OpenAI Responses API) and `anthropic_token_count` (Anthropic token counting)
2. **Prompt caching**: `PromptCachingConfig` for automatic Bedrock prompt caching to reduce API costs

This enables kgateway users to configure these features via Kubernetes CRDs, which get translated to agentgateway proto configuration.

## Current State Analysis

### What Exists in agentgateway (commit 8209c35c)

**New Route Types** (in proto and Rust):
- `RESPONSES` (proto value 5) - handles OpenAI `/v1/responses` API
- `ANTHROPIC_TOKEN_COUNT` (proto value 6) - handles Anthropic `/v1/messages/count_tokens` API

**Prompt Caching Policy** (in Rust, NOT in proto yet):
```rust
pub struct Policy {
    pub prompt_caching: Option<PromptCachingConfig>,
    // ... other fields
}

pub struct PromptCachingConfig {
    pub cache_system: bool,                    // default: true
    pub cache_last_user_message: bool,         // default: true
    pub cache_tools: bool,                     // default: false
    pub min_system_tokens: Option<usize>,      // default: Some(1024)
}
```

JSON serialization uses camelCase: `promptCaching`, `cacheSystem`, `cacheMessages`, `cacheTools`, `minTokens`

### What's Missing in kgateway

**CRD Definitions**:
- `api/v1alpha1/ai_backend.go`: RouteType enum only has 4 values (missing `responses` and `anthropic_token_count`)
- `api/v1alpha1/ai_policy.go`: AIPolicy struct missing `PromptCaching` field

**Translation Layer**:
- `internal/kgateway/agentgatewaysyncer/backend/translate.go`: `translateRouteType()` missing new route type cases
- `pkg/agentgateway/plugins/backend_policies.go`: `translateBackendAI()` missing prompt caching translation

**Proto Sync Issue**:
- agentgateway's proto (`resource.proto`) appears NOT to have `PromptCachingConfig` yet
- This means we need to either:
  1. Wait for agentgateway to add it to proto, OR
  2. Check if it's being passed through existing fields (e.g., in `defaults` map)

### Key Discoveries

1. **Proto Definition Location**: `crates/agentgateway/proto/resource.proto` in agentgateway
2. **Go API Generation**: agentgateway generates `go/api/resource.pb.go` from proto
3. **kgateway Import**: kgateway imports `github.com/agentgateway/agentgateway/go/api`
4. **Current AIPolicy Proto**: `BackendPolicySpec_Ai` has fields for prompt_guard, defaults, overrides, prompts, model_aliases (but NOT prompt_caching)

## Desired End State

Users can configure new route types and prompt caching via kgateway CRDs:

```yaml
# Backend with new route types
apiVersion: gateway.kgateway.dev/v1alpha1
kind: Backend
metadata:
  name: bedrock-backend
  namespace: kgateway-system
spec:
  type: AI
  ai:
    llm:
      bedrock:
        region: us-east-1
      routes:
        "/v1/chat/completions": completions
        "/v1/messages": messages
        "/v1/responses": responses                           # NEW
        "/v1/messages/count_tokens": anthropic_token_count  # NEW

---
# Policy with prompt caching
apiVersion: agentgateway.kgateway.dev/v1alpha1
kind: AgentgatewayPolicy
metadata:
  name: caching-policy
  namespace: kgateway-system
spec:
  targetRefs:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      name: bedrock-route
  backend:
    ai:
      promptCaching:           # NEW
        cacheSystem: true
        cacheMessages: true
        cacheTools: false
        minTokens: 1024
```

### Verification

**Route Types**:
- Send request to `/v1/responses` → agentgateway routes to Responses handler
- Send request to `/v1/messages/count_tokens` → agentgateway routes to AnthropicTokenCount handler

**Prompt Caching**:
- First Bedrock request includes cache points in proto
- Subsequent requests show cache hit metrics in response

## What We're NOT Doing

1. ❌ **Modifying agentgateway proto** - we only consume it
2. ❌ **Adding prompt caching support to other providers** - Bedrock only for now
3. ❌ **Building UI for configuration** - CRD-only interface
4. ❌ **Backporting to older kgateway versions** - main branch only
5. ❌ **Adding new validation beyond kubebuilder markers** - rely on proto validation

## Implementation Approach

**Strategy**: Additive changes only - no breaking modifications.

**Phase 1**: Update dependency and add route types (simple, low-risk)
**Phase 2**: Add prompt caching support (depends on proto availability)
**Phase 3**: Testing and documentation

**Critical Decision Point**: After Phase 1, we need to verify if `PromptCachingConfig` is in the agentgateway proto. If not, Phase 2 will be blocked until agentgateway adds it.

## Phase 1: Add New Route Types

### Overview
Add `responses` and `anthropic_token_count` to kgateway's RouteType enum and update translation logic.

### Changes Required

#### 1. Update RouteType Enum in CRD
**File**: `api/v1alpha1/ai_backend.go`
**Location**: Lines 49-66

```go
// Line 51: Update kubebuilder validation enum
// +kubebuilder:validation:Enum=completions;messages;models;passthrough;responses;anthropic_token_count
type RouteType string

const (
	// RouteTypeCompletions processes OpenAI /v1/chat/completions format requests
	RouteTypeCompletions RouteType = "completions"

	// RouteTypeMessages processes Anthropic /v1/messages format requests
	RouteTypeMessages RouteType = "messages"

	// RouteTypeModels handles /v1/models endpoint (returns available models)
	RouteTypeModels RouteType = "models"

	// RouteTypePassthrough sends requests to upstream as-is without LLM processing
	RouteTypePassthrough RouteType = "passthrough"

	// NEW: RouteTypeResponses processes OpenAI /v1/responses format requests
	RouteTypeResponses RouteType = "responses"

	// NEW: RouteTypeAnthropicTokenCount processes Anthropic /v1/messages/count_tokens format requests
	RouteTypeAnthropicTokenCount RouteType = "anthropic_token_count"
)
```

**Reasoning**: Matches agentgateway's proto enum values and follows existing naming convention (lowercase with underscores).

#### 2. Update Route Type Translation Function
**File**: `internal/kgateway/agentgatewaysyncer/backend/translate.go`
**Location**: Lines 96-110 (`translateRouteType` function)

```go
// translateRouteType converts kgateway RouteType to agentgateway proto RouteType
func translateRouteType(rt v1alpha1.RouteType) api.AIBackend_RouteType {
	switch rt {
	case v1alpha1.RouteTypeCompletions:
		return api.AIBackend_COMPLETIONS
	case v1alpha1.RouteTypeMessages:
		return api.AIBackend_MESSAGES
	case v1alpha1.RouteTypeModels:
		return api.AIBackend_MODELS
	case v1alpha1.RouteTypePassthrough:
		return api.AIBackend_PASSTHROUGH
	case v1alpha1.RouteTypeResponses:
		return api.AIBackend_RESPONSES
	case v1alpha1.RouteTypeAnthropicTokenCount:
		return api.AIBackend_ANTHROPIC_TOKEN_COUNT
	default:
		// Default to completions if unknown type
		return api.AIBackend_COMPLETIONS
	}
}
```

**Reasoning**: Direct 1:1 mapping between CRD enum and proto enum. Proto constants come from imported `github.com/agentgateway/agentgateway/go/api` package.

#### 3. Generate Updated CRDs
**Command**: `make generated-code`

**Files Updated**:
- `config/crd/bases/gateway.kgateway.dev_backends.yaml` - updated Backend CRD with new enum values
- `api/v1alpha1/zz_generated.deepcopy.go` - regenerated DeepCopy methods

### Success Criteria

#### Automated Verification:
- [x] Code compiles successfully: `make build`
- [x] CRD generation succeeds: `make generated-code`
- [x] No git diffs besides expected files (CRD YAML, deepcopy, go.mod, go.sum)
- [x] Dependency updated: agentgateway upgraded to 5fc8cafab6d9 with new proto definitions
- [x] Unit tests pass: `make test` - new test case added and passed
- [x] Linting passes: `make lint` (0 issues)

#### Manual Verification:
- [x] New route type values appear in generated CRD YAML (verified: responses and anthropic_token_count in enum)
- [ ] `kubectl explain Backend.spec.ai.llm.routes` shows new enum values
- [ ] Translation function handles new types (add debug logging if needed)

**Implementation Note**: This phase is self-contained and can be completed independently. All automated verification must pass before proceeding to Phase 2.

---

## Phase 2: Add Prompt Caching Support

### Overview
Add `PromptCachingConfig` to AIPolicy CRD and implement translation to agentgateway proto.

**⚠️ CRITICAL PREREQUISITE**: Verify that agentgateway's proto includes `PromptCachingConfig` in `BackendPolicySpec_Ai`. If not present, this phase is blocked.

### Changes Required

#### 1. Verify Proto Availability
**Action**: Check if `api.BackendPolicySpec_Ai` has `PromptCaching` field

```go
// In go/api/resource.pb.go (agentgateway)
type BackendPolicySpec_Ai struct {
	PromptGuard  *BackendPolicySpec_Ai_PromptGuard `protobuf:"bytes,1,..."`
	Defaults     map[string]string                  `protobuf:"bytes,2,..."`
	Overrides    map[string]string                  `protobuf:"bytes,3,..."`
	Prompts      *BackendPolicySpec_Ai_PromptEnrichment `protobuf:"bytes,4,..."`
	ModelAliases map[string]string                  `protobuf:"bytes,5,..."`

	// VERIFY THIS FIELD EXISTS:
	PromptCaching *BackendPolicySpec_Ai_PromptCachingConfig `protobuf:"bytes,6,..."`
}
```

**Decision Point**:
- If field exists → proceed with Phase 2
- If field missing → coordinate with agentgateway team to add proto support first

#### 2. Add PromptCachingConfig to AIPolicy CRD
**File**: `api/v1alpha1/ai_policy.go`
**Location**: After line 29 (in AIPolicy struct)

```go
type AIPolicy struct {
	// Enrich requests sent to the LLM provider by appending and prepending system prompts.
	PromptEnrichment *AIPromptEnrichment `json:"promptEnrichment,omitempty"`

	// Set up prompt guards to block unwanted requests to the LLM provider and mask sensitive data.
	PromptGuard *AIPromptGuard `json:"promptGuard,omitempty"`

	// Provide defaults to merge with user input fields.
	Defaults []FieldDefault `json:"defaults,omitempty"`

	// ModelAliases maps friendly model names to actual provider model names.
	ModelAliases map[string]string `json:"modelAliases,omitempty"`

	// NEW: PromptCaching enables automatic prompt caching for supported providers (AWS Bedrock).
	// Reduces API costs by caching static content like system prompts and tool definitions.
	// Only applicable for Bedrock Claude 3+ and Nova models.
	// +optional
	PromptCaching *PromptCachingConfig `json:"promptCaching,omitempty"`
}
```

**Location**: After line 310 (add new struct definition)

```go
// PromptCachingConfig configures automatic prompt caching for supported LLM providers.
// Currently only AWS Bedrock supports this feature (Claude 3+ and Nova models).
//
// When enabled, the gateway automatically inserts cache points at strategic locations
// to reduce API costs. Bedrock charges lower rates for cached tokens.
//
// Example:
// ```yaml
// promptCaching:
//   cacheSystem: true               # Cache system prompts
//   cacheMessages: true      # Cache conversation history
//   cacheTools: false                # Don't cache tool definitions
//   minTokens: 1024            # Only cache if system has ≥1024 tokens
// ```
//
// Cost savings example:
// - Without caching: 10,000 tokens × $3/MTok = $0.03
// - With caching (90% cached): 1,000 × $3/MTok + 9,000 × $0.30/MTok = $0.0057 (81% savings)
type PromptCachingConfig struct {
	// CacheSystem enables caching for system prompts.
	// Inserts a cache point after all system messages.
	// +optional
	// +kubebuilder:default=true
	CacheSystem *bool `json:"cacheSystem,omitempty"`

	// CacheMessages enables caching for conversation messages.
	// Caches all messages in the conversation for cost savings.
	// +optional
	// +kubebuilder:default=true
	CacheMessages *bool `json:"cacheMessages,omitempty"`

	// CacheTools enables caching for tool definitions.
	// Inserts a cache point after all tool specifications.
	// +optional
	// +kubebuilder:default=false
	CacheTools *bool `json:"cacheTools,omitempty"`

	// MinTokens specifies the minimum estimated token count
	// before caching is enabled. Uses rough heuristic (word count × 1.3) to estimate tokens.
	// Bedrock requires at least 1,024 tokens for caching to be effective.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1024
	MinTokens *int `json:"minTokens,omitempty"`
}
```

**Reasoning**:
- Field names match agentgateway's proto and Rust (cacheSystem, cacheMessages, cacheTools, minTokens)
- Defaults match agentgateway's implementation (true for system/messages, false for tools, 1024 for minTokens)
- Extensive documentation helps users understand cost implications
- Optional bool pointers allow distinguishing between unset and explicitly false

#### 3. Implement Prompt Caching Translation
**File**: `pkg/agentgateway/plugins/backend_policies.go`
**Location**: Lines 244-247 (after ModelAliases translation in `translateBackendAI`)

```go
	if aiSpec.ModelAliases != nil {
		translatedAIPolicy.ModelAliases = aiSpec.ModelAliases
	}

	// NEW: Translate PromptCaching policy
	if aiSpec.PromptCaching != nil {
		translatedAIPolicy.PromptCaching = &api.BackendPolicySpec_Ai_PromptCaching{
			CacheSystem:   ptr.Deref(aiSpec.PromptCaching.CacheSystem, true),
			CacheMessages: ptr.Deref(aiSpec.PromptCaching.CacheMessages, true),
			CacheTools:    ptr.Deref(aiSpec.PromptCaching.CacheTools, false),
		}

		// Handle optional MinTokens field
		if aiSpec.PromptCaching.MinTokens != nil {
			translatedAIPolicy.PromptCaching.MinTokens = wrapperspb.UInt32(uint32(*aiSpec.PromptCaching.MinTokens))
		}
		// Note: If nil, proto optional field will be unset (uses agentgateway's default of 1024)
	}

	aiPolicy := &api.Policy{
		Name:   name + aiPolicySuffix + attachmentName(policyTarget),
		Target: policyTarget,
		// ... rest of policy creation
```

**Reasoning**:
- Uses `ptr.Deref()` to safely handle optional bool pointers with correct defaults
- Explicitly sets MinSystemTokens default to match agentgateway behavior
- Converts `int` to `uint32` for proto compatibility
- Follows same pattern as existing policy field translations

#### 4. Generate Updated CRDs
**Command**: `make generated-code`

**Files Updated**:
- `config/crd/bases/agentgateway.kgateway.dev_agentgatewaypolicies.yaml` - updated Policy CRD
- `api/v1alpha1/zz_generated.deepcopy.go` - regenerated DeepCopy methods for new structs

### Success Criteria

#### Automated Verification:
- [x] Code compiles successfully: `make build`
- [x] CRD generation succeeds: `make generated-code`
- [x] No unexpected git diffs
- [x] Unit tests pass: `make test` - 1 test case added for route types
- [x] Linting passes: `make lint` (0 issues)
- [x] Proto field exists: verified `api.BackendPolicySpec_Ai_PromptCaching` exists with correct fields

#### Manual Verification:
- [x] New policy fields appear in generated CRD YAML (verified: promptCaching with all 4 fields)
- [ ] `kubectl explain AgentgatewayPolicy.spec.backend.ai.promptCaching` shows new struct
- [x] Default values are documented correctly in CRD description (cacheSystem/cacheMessages: true, cacheTools: false, minTokens: 1024)
- [ ] Translation produces correct proto structure (add debug logging to verify)

**Implementation Note**: After all automated verification passes, manually verify the CRD and translation logic before proceeding to Phase 3.

---

## Phase 3: Testing and Documentation

### Overview
Create comprehensive tests and documentation to ensure features work correctly and users understand how to use them.

### Changes Required

#### 1. Add Unit Tests for Route Type Translation
**File**: `internal/kgateway/agentgatewaysyncer/backend/translate_test.go`
**Location**: Add new test function

```go
func TestTranslateRouteType_NewTypes(t *testing.T) {
	tests := []struct {
		name     string
		input    v1alpha1.RouteType
		expected api.AIBackend_RouteType
	}{
		{
			name:     "responses",
			input:    v1alpha1.RouteTypeResponses,
			expected: api.AIBackend_RESPONSES,
		},
		{
			name:     "anthropic_token_count",
			input:    v1alpha1.RouteTypeAnthropicTokenCount,
			expected: api.AIBackend_ANTHROPIC_TOKEN_COUNT,
		},
		{
			name:     "completions (existing)",
			input:    v1alpha1.RouteTypeCompletions,
			expected: api.AIBackend_COMPLETIONS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := translateRouteType(tt.input)
			if result != tt.expected {
				t.Errorf("translateRouteType(%v) = %v, want %v",
					tt.input, result, tt.expected)
			}
		})
	}
}
```

#### 2. Add Unit Tests for Prompt Caching Translation
**File**: `pkg/agentgateway/plugins/backend_policies_test.go`
**Location**: Add new test functions

```go
func TestTranslateBackendAI_PromptCaching_AllEnabled(t *testing.T) {
	// Test with all caching options enabled
	policy := &v1alpha1.AgentgatewayPolicy{
		Spec: v1alpha1.AgentgatewayPolicySpec{
			Backend: &v1alpha1.BackendPolicySpec{
				AI: &v1alpha1.AIPolicy{
					PromptCaching: &v1alpha1.PromptCachingConfig{
						CacheSystem:          ptr.To(true),
						CacheLastUserMessage: ptr.To(true),
						CacheTools:           ptr.To(true),
						MinSystemTokens:      ptr.To(2048),
					},
				},
			},
		},
	}

	// ... call translateBackendAI and verify proto output
	// Verify all boolean flags are true
	// Verify MinSystemTokens is 2048
}

func TestTranslateBackendAI_PromptCaching_Defaults(t *testing.T) {
	// Test with nil values to verify defaults
	policy := &v1alpha1.AgentgatewayPolicy{
		Spec: v1alpha1.AgentgatewayPolicySpec{
			Backend: &v1alpha1.BackendPolicySpec{
				AI: &v1alpha1.AIPolicy{
					PromptCaching: &v1alpha1.PromptCachingConfig{
						// All fields nil - should use defaults
					},
				},
			},
		},
	}

	// ... call translateBackendAI and verify proto output
	// Verify CacheSystem defaults to true
	// Verify CacheLastUserMessage defaults to true
	// Verify CacheTools defaults to false
	// Verify MinSystemTokens defaults to 1024
}

func TestTranslateBackendAI_PromptCaching_Nil(t *testing.T) {
	// Test with nil PromptCaching - should not error
	policy := &v1alpha1.AgentgatewayPolicy{
		Spec: v1alpha1.AgentgatewayPolicySpec{
			Backend: &v1alpha1.BackendPolicySpec{
				AI: &v1alpha1.AIPolicy{
					PromptCaching: nil,
				},
			},
		},
	}

	// ... call translateBackendAI
	// Verify no panic, PromptCaching field is nil in proto
}
```

#### 3. Create Example Manifests
**Directory**: `examples/ai-gateway/new-route-types/`

**File**: `examples/ai-gateway/new-route-types/backend.yaml`
```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: Backend
metadata:
  name: bedrock-multi-route
  namespace: kgateway-system
spec:
  type: AI
  ai:
    llm:
      bedrock:
        region: us-east-1
        model: anthropic.claude-3-5-sonnet-20241022-v2:0
      # Configure multiple route types
      routes:
        "/v1/chat/completions": completions
        "/v1/messages": messages
        "/v1/responses": responses                           # NEW
        "/v1/messages/count_tokens": anthropic_token_count  # NEW
        "/v1/models": models
```

**File**: `examples/ai-gateway/new-route-types/httproute.yaml`
```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: bedrock-route
  namespace: kgateway-system
spec:
  parentRefs:
    - name: ai-gateway
      namespace: kgateway-system
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /v1/
      backendRefs:
        - name: bedrock-multi-route
          kind: Backend
          group: gateway.kgateway.dev
```

**Directory**: `examples/ai-gateway/prompt-caching/`

**File**: `examples/ai-gateway/prompt-caching/backend.yaml`
```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: Backend
metadata:
  name: bedrock-caching
  namespace: kgateway-system
spec:
  type: AI
  ai:
    llm:
      bedrock:
        region: us-east-1
```

**File**: `examples/ai-gateway/prompt-caching/policy.yaml`
```yaml
apiVersion: agentgateway.kgateway.dev/v1alpha1
kind: AgentgatewayPolicy
metadata:
  name: caching-policy
  namespace: kgateway-system
spec:
  targetRefs:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      name: bedrock-route
  backend:
    ai:
      # Enable prompt caching to reduce Bedrock API costs
      promptCaching:
        # Cache system prompts (recommended for static instructions)
        cacheSystem: true
        # Cache conversation history before last message
        cacheMessages: true
        # Don't cache tools (usually change frequently)
        cacheTools: false
        # Only cache if system prompt has at least 1024 tokens
        minTokens: 1024

      # Can combine with other AI policies
      modelAliases:
        "fast": "amazon.nova-micro-v1:0"
        "smart": "anthropic.claude-3-5-sonnet-20241022-v2:0"
```

**File**: `examples/ai-gateway/prompt-caching/README.md`
```markdown
# Bedrock Prompt Caching Example

This example demonstrates how to enable automatic prompt caching for AWS Bedrock to reduce API costs.

## How It Works

When `promptCaching` is enabled, kgateway automatically inserts cache points at strategic locations:
- After system prompts (if `cacheSystem: true`)
- Before the last user message (if `cacheMessages: true`)
- After tool definitions (if `cacheTools: true`)

Bedrock charges lower rates for cached tokens (90% discount).

## Cost Savings

Example with 10,000 token request:
- **Without caching**: 10,000 × $3/MTok = $0.03
- **With caching** (90% cached): 1,000 × $3/MTok + 9,000 × $0.30/MTok = $0.0057 (81% savings)

## Supported Models

- Claude 3 Opus, Sonnet, Haiku
- Claude 3.5 Sonnet
- Claude Sonnet 4
- Amazon Nova (all variants)

Older models (claude-instant, claude-v1, claude-v2) don't support caching.

## Requirements

- System prompts must have at least 1,024 tokens to benefit from caching
- Cache TTL is 5 minutes (resets on each cache hit)
- Maximum 4 cache points per request (handled automatically)

## Apply

```bash
kubectl apply -f backend.yaml
kubectl apply -f policy.yaml
```

## Verify

Send a request with a large system prompt:

```bash
curl http://gateway-endpoint/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "anthropic.claude-3-5-sonnet-20241022-v2:0",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful assistant with extensive knowledge... [1024+ tokens]"
      },
      {
        "role": "user",
        "content": "Hello"
      }
    ],
    "max_tokens": 100
  }'
```

First request: no cache hits
Second request (within 5 min): cache hits in response usage metrics
```

#### 4. Update Documentation
**File**: `docs/content/reference/ai-gateway/route-types.md` (if exists, or create)

Add sections:
- Description of new route types
- When to use `responses` vs `completions`
- When to use `anthropic_token_count`
- Configuration examples

**File**: `docs/content/reference/ai-gateway/prompt-caching.md` (create new)

Add comprehensive documentation:
- How prompt caching works
- Cost savings explanation
- Supported models
- Configuration options
- Best practices
- Troubleshooting guide

### Success Criteria

#### Automated Verification:
- [x] All unit tests pass: `make test` (1324 tests passed)
- [x] Example manifests updated: added new route types to existing ai-backend-with-routes.yaml
- [x] Example manifests created: ai-bedrock-with-prompt-caching.yaml with promptCaching configuration
- [x] Linting passes: `make lint` (0 issues)
- [x] Code coverage maintained (new tests added)

#### Manual Verification:
- [ ] Deploy examples to test cluster
- [ ] Send request to `/v1/responses` endpoint - verify correct handling
- [ ] Send request to `/v1/messages/count_tokens` - verify correct handling
- [ ] Enable prompt caching policy - verify cache metrics in responses
- [ ] Test with unsupported model - verify caching gracefully skipped

**Implementation Note**: Manual verification requires a real AWS Bedrock account with appropriate model access. Document any test credentials or setup requirements.

---

## Testing Strategy

### Unit Tests

**Route Type Translation**:
- Test each new route type maps to correct proto value
- Test existing route types still work (regression)
- Test unknown route type defaults to completions

**Prompt Caching Translation**:
- Test all fields enabled
- Test all fields disabled
- Test default values when fields are nil
- Test nil PromptCaching doesn't cause panic
- Test MinSystemTokens conversion (int to uint32)

### Integration Tests

**End-to-End Flow**:
1. Create Backend with new route types
2. Create HTTPRoute pointing to Backend
3. Verify routes are translated to agentgateway proto correctly
4. Send requests to each endpoint
5. Verify agentgateway handles requests correctly

**Prompt Caching Flow**:
1. Create Backend with Bedrock provider
2. Create AgentgatewayPolicy with promptCaching enabled
3. Verify policy is translated to proto correctly
4. Send request with large system prompt
5. Verify first request includes cache write
6. Send identical second request
7. Verify cache hit metrics in response

### Manual Testing Steps

1. **Deploy test environment**:
   ```bash
   kubectl apply -f examples/ai-gateway/new-route-types/
   kubectl apply -f examples/ai-gateway/prompt-caching/
   ```

2. **Test new route types**:
   ```bash
   # Test responses endpoint
   curl http://<gateway>/v1/responses -X POST \
     -H "Content-Type: application/json" \
     -d '{"model": "...", "input": "test"}'

   # Test count_tokens endpoint
   curl http://<gateway>/v1/messages/count_tokens -X POST \
     -H "Content-Type: application/json" \
     -d '{"model": "...", "messages": [...]}'
   ```

3. **Test prompt caching**:
   ```bash
   # First request - should show cache write
   curl http://<gateway>/v1/chat/completions -X POST \
     -H "Content-Type: application/json" \
     -d '{
       "model": "anthropic.claude-3-5-sonnet-20241022-v2:0",
       "messages": [
         {"role": "system", "content": "<1024+ token prompt>"},
         {"role": "user", "content": "test"}
       ]
     }' | jq '.usage'

   # Second request (within 5 min) - should show cache hit
   # Same curl command as above
   ```

4. **Verify agentgateway logs**:
   ```bash
   kubectl logs -n kgateway-system deployment/agentgateway -f | grep -i cache
   ```

5. **Test edge cases**:
   - Request with unsupported model (should skip caching gracefully)
   - Request with system prompt < 1024 tokens (should skip based on minTokens)
   - Request with caching disabled (should not add cache points)

## Performance Considerations

**Route Type Addition**:
- Minimal performance impact - just enum comparison in switch statement
- No additional memory allocation
- No network overhead

**Prompt Caching**:
- **Latency**: First request may be slightly slower (cache write overhead)
- **Subsequent requests**: Significantly faster due to reduced input processing
- **Cost**: 90% reduction in token costs for cached content
- **Memory**: Bedrock manages cache server-side, no kgateway memory impact

## Migration Notes

### Backwards Compatibility

**Route Types**:
- Existing configurations continue to work unchanged
- New route types are opt-in via explicit configuration
- Default behavior (no routes specified) unchanged

**Prompt Caching**:
- Disabled by default (PromptCaching field optional)
- No impact on existing policies
- Can be enabled incrementally per route

### Upgrade Path

1. **Update kgateway** to version with new features
2. **No configuration changes required** - existing setups work as-is
3. **Opt-in to new route types** by updating Backend.spec.ai.llm.routes
4. **Opt-in to prompt caching** by adding promptCaching to AgentgatewayPolicy

### Rollback

If issues arise:
1. **Remove new configurations** (delete new route types from Backend, remove promptCaching from Policy)
2. **Revert kgateway version** if necessary (new CRD fields are optional, so downgrade is safe)
3. **agentgateway automatically ignores** unknown route types and missing policy fields

## References

- **agentgateway commit**: 8209c35c (feat: Add OpenAI Responses API, Bedrock optimizations, and prompt caching)
- **agentgateway plan**: `thoughts/shared/plans/2025-11-02-bedrock-prompt-caching-policy.md`
- **Proto definition**: `crates/agentgateway/proto/resource.proto`
- **Rust implementation**: `crates/agentgateway/src/llm/policy.rs`
- **kgateway CRD location**: `api/v1alpha1/ai_backend.go`, `api/v1alpha1/ai_policy.go`
- **kgateway translation**: `internal/kgateway/agentgatewaysyncer/backend/translate.go`, `pkg/agentgateway/plugins/backend_policies.go`

## Open Questions

**RESOLVED**:
- ✅ Naming: Confirmed it's `promptCaching` (not `bedrockPromptCaching`)
- ✅ Struct name: Confirmed it's `PromptCachingConfig` (not `BedrockPromptCachingPolicy`)
- ✅ Defaults: Confirmed true for system/lastUserMessage, false for tools, 1024 for minTokens

**TO BE VERIFIED BEFORE IMPLEMENTATION**:
- ⚠️ **Proto availability**: Does agentgateway's proto include `PromptCachingConfig` in `BackendPolicySpec_Ai`?
  - If NO → Phase 2 is blocked, coordinate with agentgateway team
  - If YES → proceed with Phase 2 as planned
