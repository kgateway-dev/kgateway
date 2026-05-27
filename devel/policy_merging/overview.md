# Policy merging overview

This document describes the generic policy merge framework implemented in [`pkg/pluginsdk/policy/merge.go`](/pkg/pluginsdk/policy/merge.go) and how it is used today. It is meant for developers adding or updating policy plugins.

For route delegation background, see [`design/10943-route-delegation.md`](/design/10943-route-delegation.md), but treat the code as the source of truth for current behavior.

## What "policy merging" means in kgateway

Policy merging is **plugin opt-in**. A policy plugin participates by setting [`PolicyPlugin.MergePolicies`](/pkg/pluginsdk/types.go).

If a plugin does **not** provide `MergePolicies`, kgateway applies each attached policy one by one.

This does **not** create a generic winner-selection rule by itself. kgateway still preserves the policy ordering described below and invokes the plugin once per attached policy in that order, but the final outcome is owned by the plugin's apply logic:

- if the plugin simply writes the same output field repeatedly, the later application wins
- if the plugin does its own incremental merge or priority-aware apply logic, that plugin-specific logic decides the winner

The builtin route policy path is the main current example of the second case: it does not implement `PolicyPlugin.MergePolicies`, but it still uses merge-style logic during `ApplyForRoute` so inherited lower-priority policies do not automatically overwrite higher-priority values.

If a plugin **does** provide `MergePolicies`, kgateway calls that function first and then applies the single merged result.

Today, the generic framework in [`pkg/pluginsdk/policy/merge.go`](/pkg/pluginsdk/policy/merge.go) is used by:

- `TrafficPolicy`
- `ListenerPolicy`
- `HTTPListenerPolicy`

Notable non-users:

- builtin route policies do their own incremental merge/apply logic
- `BackendTLSPolicy` does not use the generic framework; it picks one winner instead

## When the generic merge framework runs

The framework runs whenever translation has **multiple attached policies of the same `GroupKind`** and the plugin opted into `MergePolicies`.

Current scenarios include:

1. **Multiple policies at the same effective attachment level**
   - multiple policies of the same kind attached to one HTTPRoute rule or route
   - multiple policies of the same kind attached to one listener / HCM scope
   - multiple policies of the same kind attached to one backend
2. **HTTPRoute delegation inheritance**
   - a child route receives policies from parent routes in the delegation chain

There are a few important current nuances:

- Listener translation currently merges **listener-attached** and **gateway-attached HTTP policies** together as the same hierarchy.
- Route translation currently merges **rule-level extension refs**, **rule-level attached policies**, and **route-level attached policies** together as the same hierarchy.
- The only place that currently assigns different hierarchy levels for generic policy merging is the **delegated parent route chain**.

So, in practice, "same hierarchy" means "same `HierarchicalPriority` value", not necessarily "same Kubernetes object kind".

## Ordering before merge

Attached policies are ordered before merging:

1. higher `kgateway.dev/policy-weight` first
2. for equal weight, older creation timestamp first

That ordering is established in [`pkg/krtcollections/policy.go`](/pkg/krtcollections/policy.go) before the plugin merge code runs. Earlier policies in the list are treated as higher priority during merge.

## The generic merge algorithm

`policy.MergePolicies(...)` does two passes in the `MergePolicies[T any]()` function in [`pkg/pluginsdk/policy/merge.go`](/pkg/pluginsdk/policy/merge.go) :

1. **Merge within each hierarchy level**
   - policies are grouped by `HierarchicalPriority`
   - each group is merged in priority order
   - this happens in the loop that calls `merge(..., true, ...)`
2. **Merge across hierarchy levels**
   - the merged result of each hierarchy is merged from highest hierarchy to lowest hierarchy
   - this happens in the final call to `merge(..., false, ...)`

Only the first pass receives the plugin `mergeSettingsJSON`. The second pass always receives an empty settings string.

## Merge strategies

The generic framework works with four internal strategies:

| Strategy | Meaning |
| --- | --- |
| `AugmentedShallowMerge` | keep existing values in `p1`; only fill unset fields from `p2` |
| `OverridableShallowMerge` | let `p2` replace set fields in `p1` |
| `AugmentedDeepMerge` | deep merge, but prefer `p1` on conflicts |
| `OverridableDeepMerge` | deep merge, but prefer `p2` on conflicts |

Here, `p1` is the current merged accumulator and `p2` is the next policy being folded in.

### Same hierarchy

For `sameHierarchy=true`, [`GetMergeStrategy(...)`](/pkg/pluginsdk/policy/merge.go) always returns `AugmentedShallowMerge`.

That means:

- same-hierarchy merge is shallow by default
- `kgateway.dev/inherited-policy-priority` is **not** consulted for same-hierarchy merge
- a plugin that wants different same-hierarchy behavior must override it itself inside its merge function

`TrafficPolicy` is the current example of such an override.

### Different hierarchies

For `sameHierarchy=false`, the strategy comes from `kgateway.dev/inherited-policy-priority` on the route that owns the inherited policy:

| Annotation value | Internal strategy |
| --- | --- |
| `ShallowMergePreferParent` | `OverridableShallowMerge` |
| `ShallowMergePreferChild` | `AugmentedShallowMerge` |
| `DeepMergePreferParent` | `OverridableDeepMerge` |
| `DeepMergePreferChild` | `AugmentedDeepMerge` |

If the annotation is missing or invalid, the default is `ShallowMergePreferChild`.

In current code, this matters primarily for **delegated HTTPRoute parent -> child inheritance**.

## Merge status and origins

The merge framework is not just about producing one IR object. It also tracks where merged fields came from.

`ir.MergeOrigins` is used to record which source policy contributed to which merged field:

- use `SetOne(...)` for shallow replacement semantics
- use `Append(...)` for deep merge semantics

This data drives two things:

1. policy attachment status (`Attached`, `Merged`, `Overridden`)
2. filter metadata written as `merge.<groupKind>` on generated Envoy objects

If a merge function does not maintain `MergeOrigins` correctly, policy status reporting becomes misleading.

Because merging always starts with an empty PolicyIr and merge existing policies into it, so even there is only one policy, the merge status and origins are always filled. This is also why you will always see the merge metadata in the envoy config dump.

## `Settings.PolicyMerge`

[`api/settings/settings.go`](/api/settings/settings.go) exposes `Settings.PolicyMerge` as a raw JSON string.

Today it is wired only into the `TrafficPolicy` plugin:

- controller settings -> plugin registry -> `trafficpolicy.NewPlugin(..., globalSettings.PolicyMerge, ...)`
- `ListenerPolicy` and `HTTPListenerPolicy` ignore it
- cross-hierarchy merge ignores it because the generic framework only passes settings during the same-hierarchy pass

Current supported `TrafficPolicy` keys are:

- `trafficPolicy.extAuth`
- `trafficPolicy.extProc`
- `trafficPolicy.transformation`
- `trafficPolicy.acl`

The recognized override values are `DeepMerge` and `ShallowMerge`. Any other value logs an error and falls back to shallow merge.

Example:

```json
{
  "trafficPolicy": {
    "extProc": "DeepMerge",
    "extAuth": "DeepMerge"
  }
}
```

## How to implement merging for a new policy plugin

### 1. Opt into merge at the plugin level

Set `PolicyPlugin.MergePolicies` and call the shared helper:

```go
MergePolicies: func(pols []ir.PolicyAtt) ir.PolicyAtt {
	return policy.MergePolicies(pols, mergeMyPolicies, "" /* or merge settings JSON */)
},
```

### 2. Write a merge function over your IR type

Your merge function receives:

- the accumulator (`p1`)
- the next policy (`p2`)
- the source policy ref
- the source policy merge origins
- merge options / strategy
- the output merge origins map
- the plugin settings JSON string

The function should mutate only the accumulator and treat `p2` as input.

### 3. Respect the active strategy

For simple fields, use `policy.IsMergeable(...)` and then either:

- assign with shallow semantics, or
- perform a policy-specific deep merge

If your plugin needs same-hierarchy overrides, do what `TrafficPolicy` does: inspect the settings JSON and replace `opts.Strategy` inside the relevant field merge function.

### 4. Record origins as you merge

Use `mergeOrigins.SetOne(...)` when one policy becomes the sole owner of a field, and `mergeOrigins.Append(...)` when a field is composed from multiple policies.

### 5. Test both merge dimensions

At minimum, add tests for:

1. multiple policies at the same hierarchy level
2. inherited merge across a delegated parent/child route hierarchy, if your policy can participate there
3. merge-origin-sensitive behavior when status or metadata matters

## Current mental model

When adding a new mergeable policy, think about these as separate decisions:

1. **Should this plugin merge at all, or should one policy win?**
2. **What should happen when two policies target the same effective attachment point?**
3. **What should happen when a parent policy is inherited by a child route?**
4. **For deep merge fields, how should origins and precedence be reported?**

The generic framework gives you the hierarchy-aware orchestration. The plugin still owns the field-level semantics.
