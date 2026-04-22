# HTTP ACL

An Envoy HTTP filter that enforces IP-based allow/deny rules in `on_request_headers()`. This filter operates at Layer 7 on a per-HTTP-request basis.

## Overview

`http-acl` inspects the downstream client IP (Envoy's `source.address` attribute) on every request and compares it against a configured rule set. The filter uses longest-prefix matching over two separate tries (one for IPv4, one for IPv6), so a small `allow` CIDR can "punch a hole" inside a larger `deny` CIDR (or vice versa) regardless of rule order. If there are duplicated CIDR, the action and name for the last rule will win.

Behavior:

- On `allow`, the filter returns `Continue` and the request proceeds down the chain.
- On `deny`, the filter returns `StopIteration` and sends an immediate response. The status code and headers for that response come from the optional `denyResponse` block (defaults: `403`, no extra headers).
- On deny, the filter writes Envoy dynamic metadata under namespace `dev.kgateway.http.acl`, key `blocked-by`:
    - the matched rule's `name` if the rule had one,
    - the literal string `"rule"` if the matched rule was unnamed,
    - the literal string `"default"` if no rule matched and the deny came from `defaultAction`,
    - the literal string `"unknown-ip"` if the downstream `SourceAddress` is missing or unparseable.
- On every deny, the filter increments the Envoy counter `dev.kgateway.http.acl.blocked` so operators can monitor block volume.
- If the downstream client IP cannot be determined from Envoy's source address, the filter denies the request (failed closed).
- IPv4-mapped IPv6 addresses (`::ffff:a.b.c.d`) are unwrapped and evaluated against the IPv4 trie.
- If no rule matches the client IP, the configured `defaultAction` is used.
- Bare IPs in rules (no `/` prefix) are treated as `/32` for IPv4 and `/128` for IPv6.

The ACL decision engine is implemented in the reusable [`acl`](../../lib/acl/) library crate so it can be unit-tested without the Envoy SDK.

## Json Config Schema

Top-level fields:

| Field           | Type                      | Required | Description                                                                          |
| --------------- | ------------------------- | -------- | ------------------------------------------------------------------------------------ |
| `defaultAction` | `"allow"` \| `"deny"`     | yes      | Action when no rule matches the client IP.                                           |
| `rules`         | array of rule objects     | no       | IP/CIDR rules. Longest-prefix match wins; order doesn't matter. For duplicated IP/CIDR, the action and name of the last one inserted will be used |
| `denyResponse`  | deny response object      | no       | Customizes the response sent on deny. Defaults to `{ "statusCode": 403 }`.           |

Rule object:

| Field    | Type                       | Required | Description                                                                |
| -------- | -------------------------- | -------- | -------------------------------------------------------------------------- |
| `name`   | string                     | no       | Optional rule name. Emitted as `blocked-by` dynamic metadata on deny.      |
| `cidr`   | string                     | yes      | CIDR (`10.0.0.0/8`, `2001:db8::/32`) or bare IP (treated as a single host).|
| `action` | `"allow"` \| `"deny"`      | yes      | Action to apply when a client IP falls in this prefix.                     |

Deny response object:

| Field                 | Type                  | Required | Description                                                                                                                                                                          |
| --------------------- | --------------------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `statusCode`          | integer (HTTP status) | no       | HTTP status code returned on deny. Defaults to `403`.                                                                                                                                |
| `headers`             | array of header pairs | no       | Extra response headers to attach on every deny. Each entry is `{"name","value"}`.                                                                                                    |
| `addBlockedByHeader`  | string                | no       | When set, the filter adds a response header with this value as the header **name** on every deny. The header **value** mirrors the `blocked-by` dynamic metadata: the matched rule's `name`, `"rule"` for an unnamed rule, or `"default"` for a default-action deny. |

### Default allow, deny one CIDR

```json
{
  "defaultAction": "allow",
  "rules": [
    { "cidr": "192.168.0.0/16", "action": "deny" }
  ]
}
```

### Default deny, allow a specific subnet

```json
{
  "defaultAction": "deny",
  "rules": [
    { "cidr": "10.0.0.0/8", "action": "allow" }
  ]
}
```

Here any client outside `10.0.0.0/8` is denied by `defaultAction`, which emits `dev.kgateway.http.acl/blocked-by = "default"` to dynamic metadata.

### Hole-punch with named rules

```json
{
  "defaultAction": "allow",
  "rules": [
    { "name": "block-internal-range",  "cidr": "10.0.0.0/8",  "action": "deny"  },
    { "name": "allow-trusted-subnet",  "cidr": "10.1.0.0/16", "action": "allow" },
    { "name": "block-rogue-host",      "cidr": "10.1.2.3",    "action": "deny"  }
  ]
}
```

With this config, client IP `10.1.2.3` is denied and `dev.kgateway.http.acl/blocked-by = "block-rogue-host"` is written to dynamic metadata. `10.1.2.4` is allowed (matches `allow-trusted-subnet`). `10.2.0.1` is denied and tagged `block-internal-range`. `8.8.8.8` falls through to the default `allow` with no metadata.

### Custom deny status code and headers

```json
{
  "defaultAction": "deny",
  "denyResponse": {
    "statusCode": 451,
    "headers": [
      { "name": "X-Blocked-Reason", "value": "geo-policy" },
      { "name": "Retry-After",      "value": "3600" }
    ]
  },
  "rules": []
}
```

Denied requests get HTTP 451 and the two extra response headers.

### Surface the block reason in a response header

```json
{
  "defaultAction": "deny",
  "denyResponse": {
    "addBlockedByHeader": "X-Blocked-By"
  },
  "rules": [
    { "name": "block-internal-range", "cidr": "10.0.0.0/8",     "action": "deny"  },
    {                                 "cidr": "192.168.0.0/16", "action": "deny"  },
    {                                 "cidr": "203.0.113.0/24", "action": "allow" }
  ]
}
```

With this config every deny carries `X-Blocked-By`, mirroring the `blocked-by` dynamic metadata:

- A request from `10.5.5.5` is denied with `X-Blocked-By: block-internal-range` (named rule).
- A request from `192.168.1.1` is denied with `X-Blocked-By: rule` (unnamed rule).
- A request from `8.8.8.8` is denied with `X-Blocked-By: default` (no rule matched, default-action deny).
- A request from `203.0.113.5` is allowed — no header is added (no deny).

### IPv6 rule

```json
{
  "defaultAction": "deny",
  "rules": [
    { "cidr": "2001:db8::/32", "action": "allow" }
  ]
}
```

## Dynamic metadata

On deny, the filter always emits metadata under namespace `dev.kgateway.http.acl`, key `blocked-by`, so access logs and downstream filters can correlate the block:

| Deny came from ...                           | `blocked-by` value   |
| -------------------------------------------- | -------------------- |
| a **named** rule                             | the rule's `name`    |
| an **unnamed** rule                          | `"rule"`             |
| the `defaultAction` (no rule matched)        | `"default"`          |

Access log format string: `%DYNAMIC_METADATA(dev.kgateway.http.acl:blocked-by)%`.

## Stats

The filter defines a single Envoy counter:

| Counter name                       | Type    | Incremented when ...                                   |
| ---------------------------------- | ------- | ------------------------------------------------------ |
| `dev.kgateway.http.acl.blocked`    | counter | the filter denies a request (for any `blocked-by` reason) |

The counter is defined once per filter config (via the SDK's `define_counter`) and incremented by 1 on each deny. It is exported through Envoy's normal stats pipeline (admin `/stats` endpoint, Prometheus, stats sinks, etc.); the exact surface name may be prefixed by Envoy's dynamic-modules stats scope.
