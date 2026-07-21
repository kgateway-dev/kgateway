# HTTP ACL policy merging

This document describes how `TrafficPolicy` ACL fields are merged. Read
[`overview.md`](overview.md) first for the generic framework; this document
covers only the ACL-specific behavior that differs from the defaults.

## What is different about ACL merging

Most `TrafficPolicy` fields follow the generic framework default: same-hierarchy
merges use `AugmentedShallowMerge` (first policy with a non-nil value wins).

ACL is different. For same-hierarchy merges, ACL **defaults to
`AugmentedDeepMerge`** even without any `Settings.PolicyMerge` override. The
rationale is that multiple ACL policies on the same route are usually intended
to be additive (union the rule sets), not mutually exclusive.

The `trafficPolicy.acl` settings key can override this default:

```json
{
  "trafficPolicy": {
    "acl": "ShallowMerge"
  }
}
```

Cross-hierarchy merges (delegated parent -> child) still use the strategy
derived from `kgateway.dev/inherited-policy-priority`, the same as every other
field.

## Deep merge semantics

The ACL IR is stored as a JSON blob inside a `DynamicModuleFilterPerRoute`
proto. Deep merge operates directly on the parsed JSON maps.

Given two ACL configs `p1` (higher priority / accumulator) and `p2`:

| Field | Merge behavior |
| --- | --- |
| `defaultAction` | `p1` wins; conflict detected\* if values differ |
| `rules` | unioned — `p1` rules first, then `p2` rules appended |
| `denyResponse.statusCode` | `p1` wins; conflict detected\* if values differ |
| `denyResponse.blockedByHeaderName` | `p1` wins if set; otherwise taken from `p2` |
| `denyResponse.headers` | unioned — `p1` headers first, then `p2` headers appended |

\* See [Conflict detection and fallback](#conflict-detection-and-fallback).

For `OverridableDeepMerge` (parent preferred over child), `p2` takes the `p1`
role in the table above.

## Conflict detection and fallback

Before merging, `detectHttpACLMergeConflict` checks the singleton fields
(`defaultAction`, `denyResponse.statusCode`, `denyResponse.blockedByHeaderName`)
for conflicting values between `p1` and `p2`.

If any conflict is found:

1. A warning is logged for each conflicting field.
2. The merge falls back to the corresponding shallow strategy (`AugmentedShallowMerge`
   or `OverridableShallowMerge`) so the whole winning ACL config is taken rather than
   producing a partially merged result. For `AugmentedShallowMerge`, `p1` (higher-priority
   policy) wins; for `OverridableShallowMerge`, `p2` (parent policy) wins.

Fields that are absent from either policy do not count as conflicts; a missing
`denyResponse` in one policy is simply filled from the other.

## MergeOrigins

ACL uses `mergeOrigins.Append("httpACL", ...)` after a successful deep merge,
meaning both contributing policies are recorded as origins. On a shallow merge
or conflict fallback, `defaultMerge` uses `SetOne`, so only the winning policy
appears as the origin.

## Example: two ACL policies on the same route

```yaml
# policy-a (higher weight, p1)
spec:
  acl:
    defaultAction: deny
    rules:
      - action: allow
        cidrs: ["10.0.0.0/8"]

# policy-b (lower weight, p2)
spec:
  acl:
    defaultAction: deny
    rules:
      - action: allow
        cidrs: ["192.168.1.0/24"]
```

Result after deep merge:

```json
{
  "defaultAction": "deny",
  "rules": [
    { "action": "allow", "cidrs": ["10.0.0.0/8"] },
    { "action": "allow", "cidrs": ["192.168.1.0/24"] }
  ]
}
```

Both policies appear as origins in `MergeOrigins["httpACL"]`.

## Gateway-level vs route-level ACL policies

ACL policies attached at the **gateway (listener) level** and ACL policies attached at the **route level** are kept in separate merge groups and are **never cross-merged** with each other.

Each group merges independently:

- All gateway-level ACL policies merge together, producing one merged ACL config. This config is written to the `typedPerFilterConfig` on the virtual host or route config.
- All route-level ACL policies on a given route merge together, producing a separate merged ACL config. This config is written to the `typedPerFilterConfig` on that specific route entry.

Envoy evaluates `typedPerFilterConfig` with route-level config taking precedence over route-config-level (gateway) config. So when a route carries its own merged ACL, it **completely replaces** the gateway-level merged ACL for that route — the two configs are not combined.

When a route has **no route-level ACL policy**, there is no per-route override and the gateway-level merged ACL applies to that route by default.

This separation is distinct from route delegation, where a parent route's policies are inherited and merged into the child route's policies across hierarchy levels using `HierarchicalPriority` and the `kgateway.dev/inherited-policy-priority` annotation.

### Example

Given:
- Two gateway-level policies (`gw-acl-1`, `gw-acl-2`) that merge to: `{"defaultAction":"deny","rules":[{"action":"allow","cidrs":["192.168.0.0/16"]},{"action":"allow","cidrs":["10.0.0.0/8"]}]}`
- Two route-level policies on `/route-0` (`route-acl-1`, `route-acl-2`) that merge to: `{"defaultAction":"allow","rules":[{"action":"deny","cidrs":["10.10.0.0/16"]},{"action":"deny","cidrs":["172.16.0.0/12"]}]}`
- `/route-1` with no route-level ACL

Result:
- `/route-0` uses its own merged ACL — the gateway-level merged ACL is **not applied**
- `/route-1` falls through to the gateway-level merged ACL

## Invalid CIDR behavior

CIDR validation runs at IR construction time, inside `constructHttpACL`, before
the policy is added to the KRT collection. A CIDR is invalid when it has host
bits set (e.g. `172.18.0.0/12` is rejected because the network address for that
prefix is `172.16.0.0/12`). Bare IP addresses without a prefix length are
always accepted.

When validation fails, the error is attached to the `PolicyWrapper.Errors`
field. The `httpACL` field on the IR is left nil for that policy.

During merge (`pkg/pluginsdk/policy/merge.go`), any policy that carries errors
is **skipped** — its IR is not folded into the accumulator. However, the error
is **propagated to the merged result's `Errors` field**.

During route translation (`pkg/kgateway/translator/irtranslator/route.go`),
when the translator processes the merged policy it checks whether the merged
result has any errors. If it does, the plugin is not applied and the error
becomes a `routeProcessingErr`. That error flows into `routeReplacementErr`,
which **replaces the entire route with an HTTP 500 direct response**. This
holds even when other policies that were successfully merged would have produced
a valid config.

### Effect on the route

- The route is **replaced with a 500 direct response** — all traffic to that
  route receives 500 regardless of source IP.
- This applies even if other valid `TrafficPolicy` objects on the same route
  had a valid ACL — their merged config is discarded along with the errored
  policy's contribution.
- The HTTPRoute's status condition is set to
  `Accepted=False, Reason=RouteRuleReplaced`.
- The invalid policy's status condition is set to `Accepted=False, Reason=Invalid`.
