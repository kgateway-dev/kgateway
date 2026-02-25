# EP-11703: GatewayParameters Accepted Status

- Issue: [#11703](https://github.com/kgateway-dev/kgateway/issues/11703)

<!-- toc -->
- [EP-11703: GatewayParameters Accepted Status](#ep-11703-gatewayparameters-accepted-status)
  - [Background](#background)
  - [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
  - [Implementation Details](#implementation-details)
    - [Configuration](#configuration)
    - [Plugin](#plugin)
    - [Controllers](#controllers)
    - [Deployer](#deployer)
    - [Translator and Proxy Syncer](#translator-and-proxy-syncer)
    - [Reporting](#reporting)
    - [Test Plan](#test-plan)
  - [Alternatives](#alternatives)
  - [Open Questions](#open-questions)
<!-- /toc -->

## Background

`GatewayParameters` currently has no object-level status that tells operators whether the object itself is valid.
Today, invalid `GatewayParameters` values surface indirectly through controller logs and consumer resources (for example, `Gateway` status),
which makes troubleshooting slower and less explicit when many resources reference the same parameters object.

This EP adds a resource-owned status model to `GatewayParameters` so each object reports whether it is accepted.
The initial implementation scopes status to object validity only, independent of consumer topology.

## Motivation

Operators need direct feedback on whether a `GatewayParameters` object is valid. Object-level status improves day-2 operations by:

- reducing time to diagnose invalid deployer inputs
- making `kubectl get gatewayparameters` immediately actionable

## Goals

- Add `GatewayParameters.status.conditions` with a primary `Accepted` condition.
- Report `Accepted=True` for valid specs and `Accepted=False` for invalid specs.
- Use reason values `Accepted` and `Invalid`, with concise error messages.
- Implement a dedicated `GatewayParameters` reconciler to own status updates.
- Keep status updates idempotent and conflict-safe.

## Non-Goals

- Aggregating per-consumer outcomes (which `Gateway`/`GatewayClass` references are healthy/unhealthy).
- Changing existing `Gateway` invalid-parameter status behavior.
- Adding reference summaries (for example, "used by N Gateways").
- Adding runtime/environment-dependent validation beyond object-level checks.

## Implementation Details

### Configuration

API updates:

- Extend `GatewayParametersStatus` in `api/v1alpha1/kgateway/gateway_parameters_types.go` with `Conditions []metav1.Condition`

### Plugin

No plugin framework changes are required. 

### Controllers

Add a dedicated reconciler for `GatewayParameters` in `pkg/kgateway/controller/gateway_parameters.go` and register it from
`pkg/kgateway/controller/controller.go` in `NewBaseGatewayController`.

Reconciler flow:

1. Watch `GatewayParameters` add/update/delete and reconcile by namespaced name.
2. Fetch the latest object; return successfully when not found/deleted.
3. Validate object-level correctness via shared deployer validation helper.
4. Build desired `Accepted` condition:
   - valid -> `status=True`, `reason=Accepted`
   - invalid -> `status=False`, `reason=Invalid`, message includes error details
5. Update `status.conditions` with retry-on-conflict logic and `observedGeneration`.
6. Skip status writes when condition state is unchanged.

### Deployer

Introduce a deployer-side validation helper for object-level checks in
`pkg/kgateway/deployer/gateway_parameters_validation.go`.

Validation scope for this phase:

- branch/spec coherence (`kube` vs `selfManaged`)
- deployer-relevant value resolvability checks
- overlay applicability shape checks that can be evaluated without mutating cluster resources

### Translator and Proxy Syncer

No translator or proxy syncer behavior changes are required.

### Reporting

`GatewayParameters` status model:

- `status.conditions` uses Kubernetes `metav1.Condition`
- primary condition type: `Accepted`
- reason values:
  - `Accepted`
  - `Invalid`
- message contains concise validation detail (single error verbatim; multiple errors summarized)
- `observedGeneration` is set to the current object generation

Status updates use standard condition helpers (`meta.SetStatusCondition`) and conflict retries.

### Test Plan

Unit tests:

- reconciler sets `Accepted=True` for valid `GatewayParameters`
- reconciler sets `Accepted=False`, `reason=Invalid`, and expected message for invalid specs
- reconciler updates `observedGeneration` when spec generation changes
- status update path remains idempotent and resilient to update conflicts

Optional follow-up e2e:

- verify accepted/invalid transitions for representative valid and invalid `GatewayParameters` examples

## Alternatives

- **Only update status during `Gateway` reconciliation**
  - Rejected: can leave status stale or missing when no consumer reconcile occurs.
- **Aggregate consumer outcomes into `GatewayParameters` status**
  - Deferred: adds coupling and complexity; object validity is a clearer first increment.
- **Introduce multiple detailed condition types in v1**
  - Deferred: one primary `Accepted` condition is enough for immediate operator feedback.

## Open Questions
- Should we add a "Policy Attachment" condition? 
  - would it be a summary of number of attached gateways / gatewayclasses or a detailed per-reference list?
- Should a later phase add a summary of referencing resources (`Gateway`/`GatewayClass`) on status?
- Is `Accepted|Invalid` sufficient long-term, or should reason taxonomy expand in a follow-up EP?
