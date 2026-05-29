# Auth Succeeded Metadata

## Overview

Kgateway can propagate authentication outcomes as Envoy dynamic metadata, allowing downstream
filters and policies to make routing or transformation decisions based on whether a request was
successfully authenticated. When enabled, a successfully-authenticated route has the key
`auth_succeeded=true` written to the `dev.kgateway.auth_policy` metadata namespace.

This feature is disabled by default and must be opted in via the `KGW_ENABLE_AUTH_METADATA`
environment variable.

## Motivation

Multiple auth mechanisms can be active simultaneously on a gateway (ExtAuth, JWT, OAuth2, Basic Auth,
API Key Auth). Downstream filters — such as transformations or response mutation
rules — may need to know whether authentication has already been verified for a given request. Envoy
dynamic metadata is the standard mechanism for passing such cross-filter state.

## How It Works

When `EnableAuthSucceededMetadata` is `true`, the trafficpolicy plugin configures two things for
each auth mechanism on a route:

1. **Chain-level "enabled" filter** — A globally-disabled [auth enabled] filter (#auth_enabled_filter) is
   added to the filter chain. Its name identifies the auth type (e.g., `ext_auth_enabled`,
   `jwt_enabled`). Being disabled by default means it has no effect unless explicitly overridden at
   the route level.

2. **Per-route override** — For routes where authentication is active, the plugin adds a per-route
   typed filter config that overrides the disabled chain-level filter and runs the Rustformation,
   which writes `dev.kgateway.auth_policy:auth_succeeded=true` into Envoy dynamic metadata.

For routes where authentication is explicitly **disabled** (e.g., via `extAuth.disable: true`), a
blank (no-op) per-route Rustformation config is applied instead. This prevents the chain-level
filter from being inherited and ensures the metadata key is never set on routes that bypass auth.

```
filter chain
  [disabled] ext_auth_enabled  <- chain-level, no-op unless route overrides
  [disabled] ext_auth/<name>   <- actual extauth filter

route with extAuth enabled
  ext_auth/<name>: ExtAuthzPerRoute  <- activates extauth
  ext_auth_enabled: <rustformation>  <- writes auth_succeeded=true

route with extAuth disabled
  ext_auth/<name>: {disabled: true}  <- disables extauth for this route
  ext_auth_enabled: {blank}          <- suppresses chain-level inheritance; no metadata set
```

## Auth Enabled Filter

The transformation filter is used to add the dynamic metadata to the request.

The metadata write is expressed as a `TransformationPolicy` JSON payload:

```json
{
  "request":  {"dynamicMetadata": [{"namespace": "dev.kgateway.auth_policy", "key": "auth_succeeded", "value": {"stringValue": "true"}}]},
  "response": {"dynamicMetadata": [{"namespace": "dev.kgateway.auth_policy", "key": "auth_succeeded", "value": {"stringValue": "true"}}]}
}
```

This payload is serialized as a JSON string and passed as the `FilterConfig` field of the
`DynamicModuleFilterPerRoute` proto.

## Supported Auth Mechanisms

The feature applies uniformly to all five auth mechanisms in the trafficpolicy plugin:

| Mechanism | Chain filter name | Enabled/disabled via |
|-----------|-------------------|----------------------|
| ExtAuth | `ext_auth_enabled` | `extAuth.extensionRef` / `extAuth.disable` |
| JWT | `jwt_enabled` | `jwt.extensionRef` / `jwt.disable` |
| OAuth2 | `oauth2_enabled` | `oauth2.extensionRef` |
| Basic Auth | `basic_auth_enabled` | `basicAuth.htpasswd` / `basicAuth.disable` |
| API Key Auth | `api_key_auth_enabled` | `apiKeyAuth.extensionRef` / `apiKeyAuth.disable` |

## Configuration

### Enabling the Feature

Set the environment variable on the kgateway controller:

```bash
KGW_ENABLE_AUTH_METADATA=true
```

Or configure it in the Helm chart values under `settings.enableAuthSucceededMetadata`.

### Reading the Metadata

Downstream filters can read the metadata using the Envoy dynamic metadata path:

- Metadata namespace: `dev.kgateway.auth_policy`
- Key: `auth_succeeded`
- Value: `"true"` (string)

For example, a `MetadataMatcher` that checks whether auth succeeded:

```yaml
filter: dev.kgateway.auth_policy
path:
  - key: auth_succeeded
value:
  stringMatch:
    exact: "true"
```
