# EP-2027: Secret-backed Header Values in TrafficPolicy

- Issue: [#2027](https://github.com/solo-io/gloo-gateway/issues/2027)

<!-- toc -->
- [Background](#background)
- [Motivation](#motivation)
- [Goals](#goals)
- [Non-Goals](#non-goals)
- [Implementation Details](#implementation-details)
  - [Configuration](#configuration)
  - [Plugin](#plugin)
  - [Translator and Proxy Syncer](#translator-and-proxy-syncer)
  - [Reporting](#reporting)
  - [Test Plan](#test-plan)
- [Design Decisions](#design-decisions)
- [Migration](#migration)
- [Alternatives](#alternatives)
- [Open Questions](#open-questions)
<!-- /toc -->

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
- Support optional `key` and `namespace` on `secretRef`, with well-defined name/key mutual
  defaulting and all-keys injection when both are absent
- No breaking change for existing users of `headerModifiers.request`/`response`

## Non-Goals

- Sourcing values from ConfigMaps or other reference types
- Header removal via secret reference (existing `Remove` field handles all removal use cases)
- Runtime-lookup / SDS-based secret injection (secret is resolved at translation time)

## Implementation Details

### Configuration

Replace the existing `gwv1.HTTPHeaderFilter` wrapper with a kgateway-specific type. Each header
entry carries either an inline `value` string or a `secretRef` as sibling fields with a `oneOf`
constraint. This keeps `value` as a plain string, preserving backward compatibility.

```go
// HTTPHeaderFilter defines a filter that modifies the headers of an HTTP request or response.
// Only one action for a given header name is permitted. Filters specifying multiple actions of
// the same or different type for any one header name are invalid and will be rejected by CRD
// validation. Configuration to set or add multiple values for a header must use RFC 7230 header
// value formatting, separating each value with a comma.
// Unlike the Gateway API HTTPHeaderFilter, each entry also supports sourcing the value from a
// Kubernetes Secret via secretRef.
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
    Remove []string     `json:"remove,omitempty"`
}

// HTTPHeader represents a single header name/value pair. Exactly one of value or secretRef must
// be set. When using secretRef, name and key interact as follows:
//   - Both present: name is the header name, key is the Secret data key.
//   - name absent, key present: the key is also used as the header name.
//   - name present, key absent: the name is also used as the Secret data key.
//   - Both absent: every entry in the Secret is injected as a header (data key -> header name).
//
// +kubebuilder:validation:ExactlyOneOf=value;secretRef
// +kubebuilder:validation:XValidation:rule="has(self.value) ? has(self.name) : true",message="name is required when using an inline value"
type HTTPHeader struct {
    // Name is the HTTP header field name. Name matching is case-insensitive.
    // Required when value is set. When secretRef is used, if omitted the Secret data key is
    // used as the header name; if both name and key are omitted every Secret entry is injected
    // as a header.
    // +optional
    Name      *gwv1.HTTPHeaderName `json:"name,omitempty"`

    // Value is an inline string value for the header. Mutually exclusive with secretRef.
    // Must consist of printable US-ASCII characters.
    // +optional
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=4096
    // +kubebuilder:validation:Pattern=`^[!-~]+([\t ]?[!-~]+)*$`
    Value     *string              `json:"value,omitempty"`

    // SecretRef sources the header value from a key in a Kubernetes Secret.
    // Mutually exclusive with value.
    // +optional
    SecretRef *SecretRefWithKey    `json:"secretRef,omitempty"`
}

// SecretRefWithKey identifies a Kubernetes Secret and optionally a specific key within it.
type SecretRefWithKey struct {
    // Name is the name of the Kubernetes Secret.
    // +required
    Name      gwv1.ObjectName  `json:"name"`

    // Key is the key within the Secret's data map to use as the header value. When omitted and
    // the parent HTTPHeader.name is set, that name is used as the key. When both key and name are
    // omitted, all entries in the Secret are injected as headers.
    // +optional
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=253
    // +kubebuilder:validation:Pattern=`^[-._a-zA-Z0-9]+$`
    Key       *string          `json:"key,omitempty"`

    // Namespace is the namespace of the Secret. If omitted, defaults to the namespace of the
    // referencing policy. Cross-namespace references require a ReferenceGrant in the target
    // namespace permitting access from the policy's namespace.
    // +optional
    Namespace *gwv1.Namespace  `json:"namespace,omitempty"`
}
```

`HeaderModifiers` becomes:

```go
// +kubebuilder:validation:AtLeastOneOf=request;response
type HeaderModifiers struct {
    // Request modifies request headers.
    // +optional
    Request  *HTTPHeaderFilter `json:"request,omitempty"`
    // Response modifies response headers.
    // +optional
    Response *HTTPHeaderFilter `json:"response,omitempty"`
}
```

Example policy showing all four `secretRef` resolution cases:

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
        # 1. Inline literal - unchanged from today, backward compatible
        - name: X-Static-Header
          value: "static-value"

        # 2. Secret-backed: explicit name and key are independent
        #    header name = "X-Api-Key"; secret data key = "api-key"
        - name: X-Api-Key
          secretRef:
            name: backend-creds
            key: api-key

        # 3. name omitted: the key is also used as the header name
        #    header name = "tenant-id"; secret data key = "tenant-id"
        - secretRef:
            name: backend-creds
            key: tenant-id

        # 4. Both name and key omitted: every Secret entry is injected as a header.
        #    Cross-namespace reference requires a ReferenceGrant.
        - secretRef:
            name: extra-creds
            namespace: other-ns
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

Internally, `SecretIndex.GetSecret` calls `krt.FetchOne` with `FilterKey` to register the specific
secret as a KRT dependency for each policy that references it. When a Secret is updated in
Kubernetes, only the `TrafficPolicy` objects that reference that specific secret are recomputed,
not all policies in the cluster. The resulting xDS diff is also minimal, so only the Envoy
instances affected by the changed policy receive an update.

**Security implications:** once a secret value is resolved at translation time, it is embedded as
plaintext in the Envoy `header_mutation` filter config and distributed via xDS. See the expanded
treatment in the [Security implications](#security-implications) section below, which includes a
discussion of exposure paths, mitigations, and the relationship to existing kgateway features with
the same trust model. Product sign-off on the security trade-offs is required before this feature
ships.

### Security implications

Secret values are resolved at control-plane translation time, not at request time. The resolved
plaintext value is embedded directly into the Envoy `header_mutation` filter configuration and
distributed to proxies via xDS. Once resolved, the value is no longer protected solely by
Kubernetes Secret RBAC. The following explains the exposure model, its scope, and the mitigations
available.

**Exposure paths**

- **Envoy admin API (`/config_dump`):** Envoy exposes all effective filter configuration at the
  `/config_dump` admin endpoint. Any process or principal with access to the Envoy admin port can
  retrieve the embedded header values in plaintext. The admin port is typically bound to localhost
  or a restricted network interface, but this varies by deployment configuration.

- **xDS transport:** Header values are transmitted from the control plane to Envoy over the xDS
  channel (typically gRPC). The values are unencrypted at the application layer; mTLS protects
  them in transit but they are visible to any component that can intercept or inspect the xDS
  stream.

- **Control-plane memory and cache:** The resolved IR is held in memory by the KRT collection while
  the control plane is running. A heap dump, core dump, or OOM kill artifact from the control-plane
  process could expose these values.

- **Logs and debug output:** If any control-plane logging, debug endpoint, or tracing pipeline
  captures effective filter configuration or xDS snapshot contents, embedded header values will
  appear in plaintext. Secret values should be redacted from logs and debug artifacts where
  technically feasible.

- **Support bundles:** Any tooling that captures gateway configuration for debugging - including
  `kubectl describe`, distributed tracing payloads that include filter config, or automated support
  bundle collectors - would include the plaintext header values.

**Comparison to existing practice in kgateway and Gloo Edge v1**

This is not a new pattern. The following existing kgateway features use the same model: secret
resolved at translation time, plaintext value embedded in xDS:

- **AWS request signing:** AWS access key ID and secret access key are embedded in the AWS
  credentials filter config distributed via xDS.
- **OAuth2/OIDC:** Client secrets are embedded in the OAuth2 filter config.
- **Basic auth:** Credentials (base64-encoded user:password pairs) are embedded in Envoy filter
  config.
- **API key auth:** Valid API key values are embedded in Envoy filter config for matching.

In Gloo Edge v1, `headerSecretRef` in `headerManipulation.requestHeadersToAdd` used the same
approach.

This EP is therefore consistent with the security posture already established across the product.
Approving this EP does not introduce a new security model; it extends an existing one to header
injection.

**What this feature does and does not provide**

- Does: Keep sensitive header values (API keys, auth tokens, credentials) out of policy manifests
  and source-controlled YAML
- Does: Leverage Kubernetes Secret RBAC for access control at the time of policy authoring
- Does not: Encrypt the header value in Envoy's runtime configuration or xDS payload
- Does not: Prevent exposure via the Envoy admin API
- Does not: Provide end-to-end confidentiality once the value has been translated into proxy
  configuration

**Recommended mitigations**

- Restrict RBAC on the kgateway control plane service account to the minimum set of secrets it
  needs to read (namespace-scoped ReferenceGrant already enforces cross-namespace boundaries at the
  policy level)
- Disable or restrict access to the Envoy admin port in production environments (bind to localhost,
  use NetworkPolicy, or disable entirely if `/config_dump` is not needed for operations)
- Ensure control-plane logs and debug output redact or exclude embedded header values; treat any
  support bundle or observability artifact that captures effective proxy configuration as containing
  potentially sensitive data
- Secure the xDS channel with mTLS (this is the default in kgateway)

**Open question for product sign-off**

The core question for product is: is it acceptable to adopt the same control-plane-resolved,
plaintext-in-xDS model that already governs AWS credentials, OAuth secrets, and basic auth
credentials - for header values sourced from Secrets? If not, the alternative (Envoy SDS-based
runtime secret injection) is substantially more complex to implement and would represent a departure
from the existing pattern for all of those features.

### Translator and Proxy Syncer

`Set` entries translate to `OVERWRITE_IF_EXISTS_OR_ADD` mutations; `Add` entries translate to
`APPEND_IF_EXISTS_OR_ADD`. This mapping is applied uniformly regardless of whether the value came
from an inline string or a secret reference.

### Reporting

If a referenced secret does not exist, does not contain the specified key, or a cross-namespace
reference is made without a valid ReferenceGrant, the `TrafficPolicy` status reflects an
`Accepted=False` condition with `Reason=Invalid` and a message identifying the cause. The same
condition is used for structural errors detected at translation time (e.g., two entries resolving
to the same header name). There is no separate `ResolvedRefs` condition on `TrafficPolicy` today;
a future revision could introduce that distinction to align more closely with Gateway API semantics.

When any policy error occurs the affected route rule is replaced with a 500 direct response rather
than silently forwarding the request without the intended header. This behavior is consistent with
how kgateway handles other policy errors, and is especially important here because the missing
header may be a security control (an auth token, API key, or credential). Forwarding silently
without it could allow requests to reach upstreams in an unauthenticated or under-scoped state.

### Test Plan

- Unit tests for the translation layer covering: inline values, secret-backed values, missing
  secret, missing key, name omitted with secretRef (key becomes header name), both name and key
  omitted (all-keys injection)
- Translator golden-file tests covering: same-namespace secret, name omitted (key becomes header),
  cross-namespace secret with ReferenceGrant (allowed), cross-namespace secret without ReferenceGrant
  (Accepted=False + 500 direct response), policy and secret in different namespace from Gateway
  (allowed)
- E2E test injecting an API key from a Secret and verifying it arrives at the upstream

## Design Decisions

### Replacing `gwv1.HTTPHeaderFilter` rather than extending it

`HeaderModifiers` previously used `gwv1.HTTPHeaderFilter` directly, imported from the upstream
Gateway API. This EP replaces it with `HTTPHeaderFilter` rather than running the two types
in parallel.

The replacement is necessary because we need to change the per-header value type - from a plain
`string` to a union of `string` or `secretRef`. Go does not allow modifying field types from
external packages by embedding or extension, so a custom type is the only option.

The risk of diverging from the upstream type is low: `gwv1.HTTPHeaderFilter` is one of the
simplest and most stable types in Gateway API (set/add/remove string headers), and Gateway API is
unlikely to add secret-backed values since those are an implementation concern rather than a
portability one. If Gateway API does add new fields to `HTTPHeaderFilter`, we add them to our
`HTTPHeaderFilter` manually - a known maintenance cost, not a structural problem.

Existing user configs that use inline `value` strings continue to work without any changes.

**Note:** Gateway API issue [#4689](https://github.com/kubernetes-sigs/gateway-api/issues/4689)
proposes extending `HTTPHeaderFilter` upstream with a `valueFrom` field supporting `secretKeyRef`
and `configMapKeyRef`. This is open with no milestone. If it progresses to a GEP and merges,
aligning with the upstream shape at that point would mean revisiting the flat-siblings approach in
favor of `valueFrom` - a potential future breaking change worth monitoring.

### Flat `value`/`secretRef` siblings - no nested wrapper type

The header entry has `value` (plain string) and `secretRef` as sibling fields with a `oneOf`
constraint, rather than nesting them inside a union wrapper struct. This preserves backward
compatibility: existing configs that use `value: "my-string"` continue to work without any
migration. The `oneOf` constraint ensures exactly one is set.

### Optional `name` with key/name mutual defaulting

`name` is optional on `HTTPHeader` when `secretRef` is used. The four resolution cases are:

| `name`  | `key`   | Resolution |
|---------|---------|------------|
| set     | set     | header name = `name`; secret data key = `key` |
| set     | absent  | secret data key = `name`; header name = `name` |
| absent  | set     | header name = `key`; secret data key = `key` |
| absent  | absent  | inject every Secret entry: data key -> header name |

This design was discussed with reviewers. An earlier proposal required `name` on every entry to
keep the API closer to upstream (`listMapKey=name` as in `gwv1.HTTPHeaderFilter`) and to simplify
the defaulting rules. The optional approach was adopted instead because making both `name` and
`key` optional is the only way to cleanly support the all-keys injection case (case 4), which was
explicitly requested to achieve feature parity with Gloo Edge v1.

The platform-team / application-team separation-of-concerns argument also supports optional `name`:
a platform team can own Secret key names and an application team can own header names independently.
When they happen to be the same string, one can be omitted as a shorthand.

When `value` (literal) is used, `name` is always required - there is no key to derive it from. This
is enforced by a CEL validation rule.

### `listType=atomic` rather than `listMapKey=name`

The upstream `gwv1.HTTPHeaderFilter` uses `+listType=map` with `+listMapKey=name`, which allows
strategic merge patch to update individual entries by header name. Our `Set` and `Add` lists use
`+listType=atomic` instead, meaning the whole list is replaced on update rather than merged
element by element.

This is a direct consequence of `name` being optional: `listMapKey=name` requires the key field to
be present on every list entry, which is not the case here (entries using the all-keys injection
case have neither `name` nor `key`). If a future revision requires `name`, it would be possible to
adopt `listMapKey=name` at that point - but that would be a breaking API change.

### Optional `namespace` defaulting to policy namespace

`namespace` is optional on `SecretRefWithKey`. When omitted, it defaults to the namespace of the
referencing `TrafficPolicy`. This makes the common same-namespace case less verbose without
sacrificing cross-namespace capability.

Cross-namespace references are supported via the existing `SecretIndex.GetSecret` path, which
enforces ReferenceGrant rules automatically. A ReferenceGrant in the target namespace must exist
for cross-namespace access to succeed. There is no difference in runtime behavior between an
explicit same-namespace reference and an omitted namespace - the defaulting is purely a convenience
for policy authors.

### All-keys injection

When both `name` and `key` are absent on a `secretRef` entry, all key-value pairs from the Secret
are injected as headers using each data key directly as the header name. Secret authors should
therefore name keys using valid HTTP header name characters when they intend the Secret to be used
this way.

The resulting header list is sorted deterministically (alphabetically by data key) to ensure stable
xDS output and avoid spurious config reloads when the Secret is updated and re-read.

### `SecretRefWithKey` is defined in `api/v1alpha1/shared`

The type was promoted to the `shared` package (as `SecretRefWithKey`) rather than kept local to
the `trafficpolicy` plugin. `HeaderModifiers` itself lives in `shared` and is referenced by
multiple policy types; keeping `SecretRefWithKey` alongside it in the same package avoids an
import from a higher-level package into `shared`.

### Duplicate entries for the same header name are a user error

A policy with two entries that resolve to the same header name - whether from an explicit `name`
field or from two `secretRef` entries whose keys are identical - is invalid. Duplicates are
detected at translation time and surfaced via `Accepted=False` with `Reason=Invalid`. Admission-
time rejection via CEL rules is not currently implemented for this case: the list uses
`listType=atomic` and `name` is optional, which rules out `listMapKey=name`-based uniqueness
enforcement at the CRD level. Allowing duplicates would produce non-deterministic header values
and undermine the predictability that `Set` semantics are meant to provide.

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

### `requestHeadersFromSecret` / `responseHeadersFromSecret` - no existing users

These fields were introduced in PR #13880, which has not yet shipped. There are no users to
migrate; the old parallel-field shape is simply replaced by this EP's unified type.

## Alternatives

### Option A: Parallel `requestHeadersFromSecret` fields (original PR #13880)

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
to set the same thing - a confusing collision. Additionally, the extensibility argument does not
hold: when ConfigMap support is added, a discriminator between source types (`secretRef` vs
`configMapRef`) is required inside `valueFrom` just as it would be as flat siblings. The nesting
adds no structural benefit.

## Open Questions

`SecretRefWithKey` was promoted to `api/v1alpha1/shared` so that it could be used alongside
`HeaderModifiers`, which is also defined there and referenced by multiple policy types. If the type
turns out not to be needed outside `TrafficPolicy`, it can be moved back to the plugin package in a
future cleanup.

Gateway API issue [#4689](https://github.com/kubernetes-sigs/gateway-api/issues/4689) proposes
upstream support for secret-backed header values via a `valueFrom` field. This is open with no
milestone. It is worth tracking and potentially engaging upstream to help shape it; if it lands, an
alignment migration would be required.
