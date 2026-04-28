# EP-2027: Secret-backed Header Values in TrafficPolicy

- Issue: [#2027](https://github.com/solo-io/gloo-gateway/issues/2027)


## Background

`TrafficPolicy.headerModifiers` currently wraps `gwv1.HTTPHeaderFilter`, which only supports static
string values. There is no way to inject a header whose value comes from a Kubernetes Secret without
embedding that value in the policy manifest itself.

In Gloo Edge v1 this was possible via `headerSecretRef` in `headerManipulation.requestHeadersToAdd`.
This EP proposes a first-class equivalent for kgateway using a unified header type that supports
both inline literals and secret references.

## Motivation

Teams that follow GitOps practices commit gateway configuration to source-controlled repositories.
Embedding credentials (API keys, auth tokens) as plaintext in those manifests is a security
anti-pattern. The feature allows a policy author to inject sensitive values at the gateway level
without ever writing them into a policy YAML.

## Goals

- Allow `TrafficPolicy.headerModifiers` to source individual header values from Kubernetes Secrets
- Keep add/set/remove semantics in one place (no parallel `*FromSecret` fields)
- Make `Set` vs `Add` semantics explicit and consistent between inline and secret-backed values
- Require explicit per-key selection (no key-less "inject all secret keys" behavior)
- No breaking change for existing users of `headerModifiers.request`/`response`

## Non-Goals

- Sourcing values from ConfigMaps or other reference types
- "Inject all keys" behavior (key-less secret injection)
- Header removal via secret reference (existing `Remove` field handles all removal use cases)

## Implementation Details

### Configuration

Replace the existing `gwv1.HTTPHeaderFilter` wrapper with a kgateway-specific type. Each header
entry carries either an inline `value` string or a `secretRef` as sibling fields with a `oneOf`
constraint. This keeps `value` as a plain string, preserving backward compatibility.

```go
// HTTPHeaderFilter defines header mutation rules for a TrafficPolicy, supporting both
// inline string values and Kubernetes Secret-backed values.
// +kubebuilder:validation:AtLeastOneOf=set;add;remove
type HTTPHeaderFilter struct {
    // +optional
    // +listType=atomic
    // +kubebuilder:validation:MaxItems=16
    Set    []HTTPHeader `json:"set,omitempty"`
    // +optional
    // +listType=atomic
    // +kubebuilder:validation:MaxItems=16
    Add    []HTTPHeader `json:"add,omitempty"`
    // +optional
    // +listType=set
    // +kubebuilder:validation:MaxItems=16
    Remove []string             `json:"remove,omitempty"`
}

// +kubebuilder:validation:ExactlyOneOf=value;secretRef
// +kubebuilder:validation:XValidation:rule="has(self.value) ? has(self.name) : true",message="name is required when using an inline value"
type HTTPHeader struct {
    // Name is the header field name. Required when value is set.
    // When secretRef is used and name is omitted, the secret key is used as the header name.
    // +optional
    Name      *gwv1.HTTPHeaderName `json:"name,omitempty"`

    // Value is an inline string value. Mutually exclusive with secretRef.
    // +optional
    Value     *string              `json:"value,omitempty"`

    // SecretRef sources the header value from a key in a Kubernetes Secret.
    // The namespace must be specified explicitly. Mutually exclusive with value.
    // +optional
    SecretRef *SecretKeyRef        `json:"secretRef,omitempty"`
}

type SecretKeyRef struct {
    // Name is the name of the Kubernetes Secret.
    // +required
    Name      gwv1.ObjectName `json:"name"`
    // Key is the key within the Secret's data map whose value will be used as the header value.
    // +required
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=253
    Key       string          `json:"key"`
    // Namespace is the namespace of the Secret. Cross-namespace references require a ReferenceGrant
    // in the target namespace permitting access from the policy's namespace.
    // +required
    Namespace gwv1.Namespace  `json:"namespace"`
}
```

`HeaderModifiers` becomes:

```go
// +kubebuilder:validation:AtLeastOneOf=request;response
type HeaderModifiers struct {
    Request  *HTTPHeaderFilter `json:"request,omitempty"`
    Response *HTTPHeaderFilter `json:"response,omitempty"`
}
```

Example policy:

```yaml
apiVersion: gateway.kgateway.dev/v1alpha1
kind: TrafficPolicy
metadata:
  name: inject-backend-creds
spec:
  targetRefs:
    - kind: HTTPRoute
      name: my-route
  headerModifiers:
    request:
      set:
        # Inline literal — unchanged from today, backward compatible
        - name: X-Static-Header
          value: "static-value"

        # Secret-backed with explicit header name
        - name: X-Api-Key
          secretRef:
            name: backend-creds
            key: api-key
            namespace: default

        # Secret-backed with name omitted — "tenant-id" becomes the header name
        - secretRef:
            name: backend-creds
            key: tenant-id
            namespace: default
      remove:
        - X-Request-Id
```

### Plugin

The `trafficpolicy` plugin resolves secret references at control-plane translation time (not at
request time). The resolved plaintext value is embedded in the Envoy `header_mutation` filter
config, consistent with how Gloo Edge v1 worked. This keeps the data plane simple and avoids any
runtime secret-lookup latency.

Secret access uses the existing `SecretIndex.GetSecret` path, which enforces ReferenceGrant rules
for cross-namespace references automatically.

**Security implications:** once a secret value is resolved at translation time, it is embedded as
plaintext in the Envoy `header_mutation` filter config and distributed via xDS. It is no longer
protected solely by Kubernetes Secret RBAC. Potential exposure paths include:

- xDS snapshots and any control-plane caching or persistence
- Envoy admin and `/config_dump` endpoints
- Control-plane logs, debug output, and support bundles that capture effective configuration

Deployments using this feature should restrict RBAC access to the control plane and xDS
distribution path, disable or tightly scope Envoy admin access where not required, and ensure logs
and support bundles redact embedded header values. This feature improves GitOps ergonomics by
keeping secrets out of policy manifests; it does not provide end-to-end confidentiality once the
value has been translated into proxy configuration.

### Translator and Proxy Syncer

`Set` entries translate to `OVERWRITE_IF_EXISTS_OR_ADD` mutations; `Add` entries translate to
`APPEND_IF_EXISTS_OR_ADD`. This mapping is applied uniformly regardless of whether the value came
from an inline string or a secret reference.

### Reporting

If a referenced secret does not exist, does not contain the specified key, or a cross-namespace
reference is made without a valid ReferenceGrant, the `TrafficPolicy` status should reflect a
`ResolvedRefs=False` condition with `Reason=InvalidCertificateRef` and a message identifying the
cause. `Accepted=False` with `Reason=Invalid` should be used only when the policy itself is
structurally invalid (e.g., duplicate header names).

### Test Plan

- Unit tests for the translation layer covering: inline values, secret-backed values, missing
  secret, missing key, name omitted with secretRef
- Translator golden-file tests covering: same-namespace secret, name omitted (key becomes header),
  cross-namespace secret with ReferenceGrant (allowed), cross-namespace secret without ReferenceGrant
  (Accepted=False), policy+secret in different namespace from Gateway (allowed)
- E2E test injecting an API key from a Secret and verifying it arrives at the upstream

## Design Decisions

### Replacing `gwv1.HTTPHeaderFilter` rather than extending it

`HeaderModifiers` previously used `gwv1.HTTPHeaderFilter` directly, imported from the upstream
Gateway API. This EP replaces it with `HTTPHeaderFilter` rather than running the two types
in parallel.

The replacement is necessary because we need to change the per-header value type — from a plain
`string` to a union of `string` or `secretRef`. Go does not allow modifying field types from
external packages by embedding or extension, so a custom type is the only option.

The risk of diverging from the upstream type is low: `gwv1.HTTPHeaderFilter` is one of the
simplest and most stable types in Gateway API (set/add/remove string headers), and Gateway API is
unlikely to add secret-backed values since those are an implementation concern rather than a
portability one. If Gateway API does add new fields to `HTTPHeaderFilter`, we add them to
`HTTPHeaderFilter` manually — a known maintenance cost, not a structural problem.

Existing user configs that use inline `value` strings continue to work without any changes.

**Note:** Gateway API issue [#4689](https://github.com/kubernetes-sigs/gateway-api/issues/4689)
proposes extending `HTTPHeaderFilter` upstream with a `valueFrom` field supporting `secretKeyRef`
and `configMapKeyRef`. This is open with no milestone. If it progresses to a GEP and merges,
aligning with the upstream shape at that point would mean revisiting the flat-siblings approach in
favor of `valueFrom` — a potential future breaking change worth monitoring.

### Flat `value`/`secretRef` siblings — no nested wrapper type

The header entry has `value` (plain string) and `secretRef` as sibling fields with a `oneOf`
constraint, rather than nesting them inside a union wrapper struct. This preserves backward
compatibility: existing configs that use `value: "my-string"` continue to work without any
migration. The `oneOf` constraint ensures exactly one is set.

### Header name is optional when using `secretRef`; defaults to the secret key

When `secretRef` is specified without a `name`, the secret key is used as the header name. This
allows a policy author to specify just the secret reference without having to restate the key as
both a lookup value and a header name. When a different header name is desired (e.g., the secret
key is `api-key` but the header should be `X-Api-Key`), `name` can be set explicitly to override
it.

When `value` (literal) is used, `name` is always required — there is no key to derive it from.

Because `name` is optional, `Set`/`Add` cannot use `listMapKey=name` for strategic merge patch.
These lists use `listType=atomic`, meaning the whole list is replaced on update rather than merged
element by element.

### Header name and secret key are separate fields when name is specified

When `name` is provided alongside `secretRef`, the header name and the secret key are intentionally
distinct. The secret owner and the policy author are often different people on different teams: a
platform team manages the Secret and controls its key names; an application team authors the
TrafficPolicy and decides which header to inject. Keeping them separate means either side can rename
their field without requiring the other to change.

### `key` is always required

`key` is always required in `SecretKeyRef`. Making it optional to mean "inject all secret key-value
pairs as headers" is out of scope for the initial implementation and listed as a Non-Goal.

### `SecretKeyRef.namespace` is required; cross-namespace supported via ReferenceGrant

`namespace` is a required field on `SecretKeyRef` — there is no defaulting to the policy namespace.
Requiring the user to state the namespace explicitly forces them to think about where the Secret
lives, and ensures that moving the control plane to a different namespace never silently changes
which Secret is resolved.

Cross-namespace references are supported: the existing `SecretIndex.GetSecret` path already enforces
ReferenceGrant rules, so no additional rejection logic is needed. A ReferenceGrant in the target
namespace must exist for cross-namespace access to succeed.

### Duplicate entries for the same header name are a user error

A policy with two entries that resolve to the same header name — whether from an explicit `name`
field or from two `secretRef` entries whose keys are identical — is a user error. Duplicate explicit
`name` entries can be caught via CEL validation at admission time. Duplicates where the header name
is derived from a secret key can only be detected at translation time and are surfaced via
`Accepted=False` with `Reason=Invalid`. Allowing duplicates would produce non-deterministic header
values and undermine the predictability that `Set` semantics are meant to provide.

## Migration

### No breaking change for existing `headerModifiers.request`/`response` users

The `value` field remains a plain string on the new `HTTPHeader` type, exactly as it was on
`gwv1.HTTPHeader`. Existing configs continue to work without any changes:

```yaml
# This still works after the migration
headerModifiers:
  request:
    set:
    - name: X-My-Header
      value: "my-value"
```

### `requestHeadersFromSecret` / `responseHeadersFromSecret` — no existing users

These fields were introduced in PR #13880, which has not yet shipped. There are no users to
migrate; the old parallel-field shape is simply replaced by this EP's unified type.

## Alternatives

### Option A: Parallel `requestHeadersFromSecret` fields (current PR #13880)

Add `requestHeadersFromSecret []SecretHeaderMapping` and `responseHeadersFromSecret
[]SecretHeaderMapping` alongside the existing `Request`/`Response` fields.

**Rejected because:** splitting inline and secret-backed values across separate fields creates
ambiguity when both target the same header name, and duplicates the add/set/remove decision across
two parallel APIs.

### Option B: Nested union wrapper (`HTTPHeaderValue`)

Wrap the value in a union struct with `literal` and `secretRef` fields, requiring existing inline
configs to migrate from `value: "string"` to `value: {literal: "string"}`.

**Rejected because:** it introduces a breaking change for existing users of
`headerModifiers.request`/`response`.

### Option C: This EP (flat siblings, non-breaking)

`value` (plain string) and `secretRef` sit as siblings on the header entry with a `oneOf`
constraint. Existing configs work unchanged. This is the approach proposed here.

### Option D: `valueFrom` wrapper

Add a `valueFrom *HTTPHeaderValue` field alongside `value`, where `HTTPHeaderValue`
holds a `secretRef` (and potentially other source types later).

**Rejected because:** `value` already names the header value, so `valueFrom` reads as a second way
to set the same thing — a confusing collision. Additionally, the extensibility argument does not
hold: when ConfigMap support is added, a discriminator between source types (`secretRef` vs
`configMapRef`) is required inside `valueFrom` just as it would be as flat siblings. The nesting
adds no structural benefit.

## Open Questions

`SecretKeyRef` is defined locally in `TrafficPolicy` for now. If a concrete need to reuse it across
other policy types arises, it can be promoted to a shared type at that point.
