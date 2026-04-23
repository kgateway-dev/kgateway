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

type KgatewayHTTPHeader struct {
    Name  gwv1.HTTPHeaderName     `json:"name"`
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
        - name: X-Api-Key
          value:
            secretRef:
              name: backend-creds
              key: api-key
        - name: X-Tenant-Id
          value:
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

### Header name and secret key are separate fields

The header name (`KgatewayHTTPHeader.Name`) and the secret key (`SecretKeyRef.Key`) are intentionally
distinct fields rather than implicitly coupled (e.g., "use the secret key as the header name"). The
secret owner and the policy author are often different people on different teams: a platform team
manages the Secret and controls its key names; an application team authors the TrafficPolicy and
decides which header to inject. Keeping the two fields separate means either side can rename their
field without requiring the other to change. It also allows a single Secret to back multiple
differently-named headers.

### Duplicate entries for the same header name are a user error

Because inline and secret-backed values share the same `Set`/`Add` lists, it is possible to write a
policy with two entries for the same header name — one inline, one secret-backed, or two
secret-backed entries pointing to different keys. This is a user error and must be rejected at
admission time with a clear validation message. Allowing it would produce non-deterministic header
values and undermine the predictability that `Set` semantics are meant to provide.

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
- Should initial scope include a `namespace` field on `SecretKeyRef` (defaulting to
  TrafficPolicy namespace, rejected at admission if set to a different namespace) to make
  future cross-namespace support a non-breaking addition?
