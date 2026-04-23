# EP-2027: Secret-backed Header Values in TrafficPolicy

- Issue: [#2027](https://github.com/solo-io/gloo-gateway/issues/2027)

<!-- toc -->
<!-- /toc -->

## Background

`TrafficPolicy.headerModifiers` currently wraps `gwv1.HTTPHeaderFilter`, which only supports static
string values. There is no way to inject a header whose value comes from a Kubernetes Secret without
embedding that value in the policy manifest itself.

In Gloo Edge v1 this was possible via `headerSecretRef` in `headerManipulation.requestHeadersToAdd`.
This EP proposes a first-class equivalent for kgateway using a unified header value type that
supports both inline literals and secret references.

## Motivation

Teams that follow GitOps practices commit gateway configuration to source-controlled repositories.
Embedding credentials (API keys, auth tokens) as plaintext in those manifests is a security
anti-pattern. The feature allows a policy author to inject sensitive values at the gateway level
without ever writing them into a policy YAML.

## Goals

- Allow `TrafficPolicy.headerModifiers` to source individual header values from Kubernetes Secrets
- Keep add/set/remove semantics in one place (no parallel `*FromSecret` fields)
- Make `Set` vs `Add` semantics explicit and consistent between inline and secret-backed values
- Require explicit per-key selection (no "inject all keys" magic)

## Non-Goals

- Cross-namespace secret references (out of scope for initial implementation; can follow with ReferenceGrant support)
- Sourcing values from ConfigMaps or other reference types
- "Inject all keys" behavior (key-less secret injection)
- Header removal via secret reference (existing `Remove` field handles all removal use cases)

## Implementation Details

### Configuration

Replace the existing `gwv1.HTTPHeaderFilter` wrapper with a kgateway-specific type that adds a
union value field:

```go
// KgatewayHTTPHeaderFilter extends gwv1.HTTPHeaderFilter to support
// secret-backed header values in addition to inline literals.
type KgatewayHTTPHeaderFilter struct {
    Set    []KgatewayHTTPHeader `json:"set,omitempty"`
    Add    []KgatewayHTTPHeader `json:"add,omitempty"`
    Remove []string             `json:"remove,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="has(self.value.literal) ? has(self.name) : true",message="name is required when using a literal value"
type KgatewayHTTPHeader struct {
    // Name is the header field name. Required when value is a literal string.
    // When secretRef is used and name is omitted, the secret key is used as the header name.
    // +optional
    Name  *gwv1.HTTPHeaderName    `json:"name,omitempty"`
    Value KgatewayHTTPHeaderValue `json:"value"`
}

// KgatewayHTTPHeaderValue is a union; exactly one field must be set.
// +kubebuilder:validation:ExactlyOneOf=value,secretRef
type KgatewayHTTPHeaderValue struct {
    // Inline string value.
    // +optional
    Literal *string `json:"value,omitempty"`

    // SecretRef sources the header value from a Kubernetes Secret.
    // +optional
    SecretRef *SecretKeyRef `json:"secretRef,omitempty"`
}

type SecretKeyRef struct {
    // Name of the Secret.
    Name gwv1.ObjectName `json:"name"`
    // Key within the Secret's data map.
    Key string `json:"key"`
}
```

`HeaderModifiers` becomes:

```go
type HeaderModifiers struct {
    Request  *KgatewayHTTPHeaderFilter `json:"request,omitempty"`
    Response *KgatewayHTTPHeaderFilter `json:"response,omitempty"`
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
        # Inline literal — name required
        - name: X-Static-Header
          value:
            literal: "static-value"
        # Secret-backed with explicit header name override
        - name: X-Api-Key
          value:
            secretRef:
              name: backend-creds
              key: api-key
        # Secret-backed with name omitted — "tenant-id" becomes the header name
        - value:
            secretRef:
              name: backend-creds
              key: tenant-id
      remove:
        - X-Request-Id
```

### Plugin

The `trafficpolicy` plugin resolves secret references at control-plane translation time (not at
request time). The resolved plaintext value is embedded in the Envoy `header_mutation` filter
config, consistent with how Gloo Edge v1 worked. This keeps the data plane simple and avoids any
runtime secret-lookup latency.

Secret access uses the existing `SecretIndex.GetSecret` path, which enforces same-namespace
constraints and can be extended to support ReferenceGrant later without API changes.

### Translator and Proxy Syncer

`Set` entries translate to `OVERWRITE_IF_EXISTS_OR_ADD` mutations; `Add` entries translate to
`APPEND_IF_EXISTS_OR_ADD`. This is the same mapping used for inline values today, applied uniformly
regardless of whether the value came from a literal or a secret reference.

### Reporting

If a referenced secret does not exist or does not contain the specified key, the `TrafficPolicy`
status should reflect `Accepted=False` with `Reason=Invalid` and a message identifying the missing
secret or key.

### Test Plan

- Unit tests for the translation layer covering: inline values, secret-backed values, missing
  secret, missing key
- Translator golden-file tests covering: same-namespace secret, cross-namespace without
  ReferenceGrant (rejected), policy+secret in different namespace from Gateway (allowed)
- E2E test injecting an API key from a Secret and verifying it arrives at the upstream

## Design Decisions

### Header name is optional when using `secretRef`; defaults to the secret key

When `secretRef` is specified without a `name`, the secret key is used as the header name. This
allows a policy author to specify just the secret reference without having to restate the key as
both a lookup value and a header name. When a different header name is desired (e.g., the secret
key is `api-key` but the header should be `X-Api-Key`), `name` can be set explicitly to override
it.

When `value` (literal) is used, `name` is always required — there is no key to derive it from.

Note: because `name` is optional, `Set`/`Add` cannot use `listMapKey=name` for strategic merge
patch. These lists use `listType=atomic`, meaning the whole list is replaced on update rather than
merged element by element.

### Header name and secret key are separate fields when name is specified

When `name` is provided alongside `secretRef`, the header name and the secret key are intentionally
distinct. The secret owner and the policy author are often different people on different teams: a
platform team manages the Secret and controls its key names; an application team authors the
TrafficPolicy and decides which header to inject. Keeping them separate means either side can rename
their field without requiring the other to change.

### `key` is always required; it does not default to the header name

An alternative considered was making `key` optional: if omitted, it would default to the header
name, reducing verbosity in the common case where the secret key and header name happen to match.
We rejected this because it re-introduces the coupling the separate-field design is meant to avoid.
If the platform team renames a secret key, or the application team renames a header, an implicit
default silently breaks without a validation error. Requiring `key` keeps both sides explicit and
independently changeable.

### `SecretKeyRef` includes an optional `namespace` field, validated to match the policy namespace

`SecretKeyRef` includes an optional `namespace` field that defaults to the policy's own namespace
if omitted. If a user specifies a namespace that differs from the policy namespace, the controller
rejects the config with a clear error rather than silently resolving it in an unexpected namespace.

This has two practical benefits: it forces the policy author to state explicitly where the Secret
lives (catching copy-paste bugs or stale configs), and it ensures that moving the control plane to a
different namespace does not silently change which Secret is resolved. When cross-namespace support
is added later, relaxing this validation becomes a non-breaking change rather than a new field.

### Duplicate entries for the same header name are a user error

Because inline and secret-backed values share the same `Set`/`Add` lists, it is possible to write a
policy with two entries for the same header name — one inline, one secret-backed, or two
secret-backed entries pointing to different keys. This is a user error and must be rejected at
admission time with a clear validation message. Allowing it would produce non-deterministic header
values and undermine the predictability that `Set` semantics are meant to provide.

## Breaking Changes and Migration

### `headerModifiers.request` and `headerModifiers.response` — breaking value type change

The `Request` and `Response` fields on `HeaderModifiers` currently wrap `gwv1.HTTPHeaderFilter`,
where each header's `value` is a plain string. This EP replaces that type with
`KgatewayHTTPHeaderFilter`, where `value` is a union struct. This is a **breaking change** for any
user currently using `headerModifiers.request` or `headerModifiers.response` with inline values.

Before:

```yaml
headerModifiers:
  request:
    set:
    - name: X-My-Header
      value: "my-value"
```

After:

```yaml
headerModifiers:
  request:
    set:
    - name: X-My-Header
      value:
        literal: "my-value"
```

All existing configs using inline header values must update the `value` field to the `literal` form.
This should be called out in the release notes for the version that ships this change.

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

### Option B: This EP (unified type)

Secret-backed and inline values share the same `Set`/`Add`/`Remove` lists. The value field is a
union. This is the approach proposed here.

## Open Questions

- Should `SecretKeyRef` be a shared type reusable by other policies, or defined locally in
  `TrafficPolicy`?
