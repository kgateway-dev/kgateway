# ReferenceGrant Enforcement Modes

## Overview

The [Gateway API ReferenceGrant](https://gateway-api.sigs.k8s.io/api-types/referencegrant/)
mechanism controls whether a resource in one namespace may reference a resource
in another namespace. Without a ReferenceGrant in the target namespace, the
reference is denied.

kgateway supports three enforcement modes, configurable via the
`KGW_REFERENCE_GRANT_MODE` environment variable. The default is `PERMISSIVE`,
which preserves backward-compatible behavior.

## Modes

### Off

```
KGW_REFERENCE_GRANT_MODE=OFF
```

> **Warning: Off mode breaks Gateway API compliance and has serious security
> implications. Do not use in multi-tenant or production environments.**
>
> - **Gateway API non-compliance.** The Gateway API specification requires
>   ReferenceGrant for all cross-namespace references. Off mode violates this
>   requirement, meaning conformance tests will fail and behavior diverges from
>   the spec in ways that may surprise users and operators.
>
> - **Namespace isolation bypassed.** Any resource in any namespace can
>   reference backends, secrets, and GatewayExtensions in any other namespace
>   without restriction. A tenant with write access to one namespace can
>   silently route traffic through or read credentials from a completely
>   unrelated namespace.
>
> - **Secret exfiltration risk.** A TrafficPolicy referencing a GatewayExtension
>   that holds an OAuth2 `ClientSecretRef` gains indirect read access to that
>   secret. With Off mode there is no grant check at either hop, so any
>   namespace can access any OAuth2 client secret in the cluster.

Disables all ReferenceGrant validation. Every cross-namespace reference is
permitted unconditionally. Only appropriate when namespace isolation is enforced
entirely by an external mechanism (e.g., a service mesh or network policy layer)
and Gateway API conformance is not required.

### Permissive (default)

```
KGW_REFERENCE_GRANT_MODE=PERMISSIVE
```

Enforces ReferenceGrant for BackendRef and SecretRef cross-namespace references,
matching today's behavior. Cross-namespace `ExtensionRef` references (TrafficPolicy
-> GatewayExtension) are **not** checked and are always permitted.

### Strict

```
KGW_REFERENCE_GRANT_MODE=STRICT
```

Enforces ReferenceGrant for all cross-namespace references, including
`ExtensionRef` (TrafficPolicy -> GatewayExtension). A missing grant in Strict
mode causes the referencing policy to be rejected and its routes to report a
`RouteRuleReplaced` condition with reason `missing reference grant`.

## Reference type coverage by mode

| Reference | From | To | Off | Permissive | Strict |
|---|---|---|---|---|---|
| BackendRef | HTTPRoute / GRPCRoute | Service / Backend | allowed | checked | checked |
| SecretRef | TrafficPolicy (BasicAuth, APIKeyAuth) | Secret | allowed | checked | checked |
| BackendRef | GatewayExtension (ExtAuth, ExtProc, RateLimit, OAuth2) | Service | allowed | checked | checked |
| ExtensionRef | TrafficPolicy | GatewayExtension (same ns) | allowed | allowed | allowed |
| ExtensionRef | TrafficPolicy | GatewayExtension (cross-ns) | allowed | allowed | checked |

Same-namespace references always pass, regardless of mode.

## Why ExtensionRef matters in Strict mode

A TrafficPolicy in namespace A that references a GatewayExtension in namespace B
gains indirect read access to everything that GatewayExtension configures,
including its `ClientSecretRef` secret. Even though the GatewayExtension ->
Secret hop is same-namespace (and therefore never requires a grant), the
unchecked first hop creates a transitive exposure path:

```
TrafficPolicy (ns A) --[ExtensionRef, no grant]--> GatewayExtension (ns B)
                                                          |
                                                 [ClientSecretRef, same-ns]
                                                          |
                                                     Secret (ns B)
```

Strict mode closes this gap by requiring a ReferenceGrant for the first hop.
Among the GatewayExtension sub-types, only OAuth2 currently holds secrets
(`ClientSecretRef`). ExtAuth, ExtProc, and RateLimit only reference Service
backends, which are already checked by the BackendRef enforcement above.

## ReferenceGrant example for Strict mode

To allow a TrafficPolicy in `app-ns` to reference a GatewayExtension in
`infra-ns`, create a ReferenceGrant in the **target** namespace (`infra-ns`):

```yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: allow-trafficpolicy-to-gwext
  namespace: infra-ns
spec:
  from:
    - group: gateway.kgateway.dev
      kind: TrafficPolicy
      namespace: app-ns
  to:
    - group: gateway.kgateway.dev
      kind: GatewayExtension
```

## Code flow

### Off mode — all checks bypassed

The mode is stored on `RefGrantIndex` (created once at startup in
`pkg/pluginsdk/collections/collections.go`). `ReferenceAllowed()` returns
`true` immediately when the mode is `Off`, so all existing call sites
(BackendRef, SecretRef) are bypassed without any changes to those call sites.

```
ReferenceAllowed()                     pkg/krtcollections/policy.go
  if mode == Off -> return true        (short-circuits all callers)
```

### Permissive mode — BackendRef and SecretRef checked

The normal check flow applies for BackendRef and SecretRef. ExtensionRef
resolution in `FetchGatewayExtension` does a raw `krt.FetchOne` without calling
`ReferenceAllowed`, so cross-namespace ExtensionRefs pass through.

```
HTTPRoute translation (KRT collection evaluation)
  transformHttpRoute()                 pkg/krtcollections/policy.go
    getBackends()
      GetBackendFromRef()
        ReferenceAllowed()             -> grant required

TrafficPolicy -> IR (KRT collection evaluation)
  ConstructIR()
    constructExtProc/ExtAuth/...
      FetchGatewayExtension()          pkg/.../trafficpolicy/constructor.go
        krt.FetchOne()                 -> no grant check
```

### Strict mode — ExtensionRef also checked

`FetchGatewayExtension` calls `ReferenceAllowed` before `krt.FetchOne` when the
mode is `Strict` and the target namespace differs from the source namespace:

```
FetchGatewayExtension()                pkg/.../trafficpolicy/constructor.go
  if mode == Strict:
    RefGrants.ReferenceAllowed(
      from: TrafficPolicy GK, fromNs,
      to:   GatewayExtension GK, targetNs, name
    )
    -> ErrMissingReferenceGrant if denied
  krt.FetchOne(gatewayExtensions, ...)
```

Because `FetchGatewayExtension` runs inside a KRT collection evaluation, the
`ReferenceAllowed` call registers a KRT dependency on the matching
ReferenceGrant objects. Adding or removing a ReferenceGrant automatically
triggers recomputation of any affected TrafficPolicy IR — no manual cache
invalidation is needed.

### Key files

| File | Role |
|---|---|
| `api/settings/settings.go` | `ReferenceGrantMode` type and `Settings.ReferenceGrantMode` field |
| `pkg/krtcollections/policy.go` | `RefGrantIndex`, `NewRefGrantIndex`, `ReferenceAllowed` |
| `pkg/pluginsdk/collections/collections.go` | Wires mode from settings into `NewRefGrantIndex` |
| `pkg/kgateway/extensions2/plugins/trafficpolicy/constructor.go` | `FetchGatewayExtension` — Strict-mode ExtensionRef check |
| `pkg/krtcollections/secrets.go` | SecretRef enforcement via `GetSecret` -> `ReferenceAllowed` |

## Tests

Translator golden tests covering each mode live under:

```
pkg/kgateway/translator/gateway/testutils/inputs/reference-grant-mode/
pkg/kgateway/translator/gateway/testutils/outputs/reference-grant-mode/
```

| Test name | Mode | Resource | Expected outcome |
|---|---|---|---|
| `off-backendref-no-grant` | Off | HTTPRoute BackendRef to Service in `other` ns, no grant | Route accepted; cluster `kube_other_backend-svc_80` present |
| `permissive-extensionref-no-grant` | Permissive | TrafficPolicy ExtProc ExtensionRef to GatewayExtension in `other-ns`, no grant | Policy accepted; `ext_proc/other-ns/extproc-ext` filter applied |
| `strict-extensionref-no-grant` | Strict | TrafficPolicy JWT ExtensionRef to GatewayExtension in `other-ns`, no grant | Policy rejected; route reports `jwt: missing reference grant` |
| `strict-extensionref-with-grant` | Strict | TrafficPolicy ExtAuth ExtensionRef to GatewayExtension in `other-ns`, with grant | Policy accepted; `ext_auth/other-ns/extauth-ext` filter applied |

Run a specific test:

```bash
go test -run "^TestBasic$/^ReferenceGrantMode" ./pkg/kgateway/translator/gateway/
```

Regenerate golden files after changing translation output:

```bash
REFRESH_GOLDEN=true go test -run "^TestBasic$/^ReferenceGrantMode" ./pkg/kgateway/translator/gateway/
```

## Audit: reference types not covered by any mode

The following references are same-namespace only by type and therefore never
require a ReferenceGrant regardless of mode:

| CRD | Field | Type | Reason |
|---|---|---|---|
| `Backend` | `Aws.Auth.SecretRef` | `corev1.LocalObjectReference` | Name only, no namespace field |
| `BackendConfigPolicy` | `TLS.SecretRef` | `corev1.LocalObjectReference` | Name only, no namespace field |
| `GatewayExtension` | `OAuth2.Credentials.ClientSecretRef` | `corev1.LocalObjectReference` | Always same-namespace as the GatewayExtension |
| `GatewayExtension` | `JWT.Providers[].JWKS.LocalJWKS.ConfigMapRef` | `corev1.LocalObjectReference` | Always same-namespace as the GatewayExtension |
