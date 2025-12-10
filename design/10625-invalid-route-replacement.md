# EP-10625: Invalid Route Replacement

*Issue*: [#10625](https://github.com/kgateway-dev/kgateway/issues/10625)

## Background

Today, when a user configures a route with invalid settings, e.g. an HTTPRoute with a path match containing invalid regex patterns, the system translates and pushes the configuration to Envoy. This can result in Envoy either NACKing the entire xDS snapshot or failing at request time, depending on when the error is detected. When Envoy NACKs an xDS snapshot, it freezes that snapshot at its last-acknowledged state and ignores every subsequent update until the offending config is fixed, effectively pausing all future changes for any tenant that relies on that snapshot. This can lead to multi-tenant isolation failure, configuration drift, operational blindness, and cascade failures.

In practice, this means that requests which would have matched the invalid rule instead fall through to the default listener's catch-all route, typically resulting in a 404 response. While technically compliant with Gateway API's allowance to drop invalid rules, this behavior can introduce significant correctness and security risks, especially in multi-tenant environments.

Silently removing a route may cause traffic to be rerouted to unintended backends or allow access to paths without their intended policy protections (e.g. authentication, rate limiting, etc.), producing fail-open behavior.

One notable exception is when a route references a non-existent backend (e.g. an `HTTPRoute.backendRef` pointing to a non-existent Service). In these cases, Envoy retains the route and responds with a 500 status code, and is aligned with Gateway API requirements. This is because Envoy treats missing clusters as a runtime failure rather than a structural omission, making it safer by default. This behavior is separate from the current proposal and will not be affected by it.

However, for all other forms of invalid configuration - especially those outside core Gateway API validation - there is currently no guardrail to ensure these failures degrade safely or visibly. This includes both critical errors (like invalid path matches that affect the entire route) and non-critical errors (like invalid retry configurations that only affect specific features). While non-critical errors might be better handled by silently dropping just the problematic configuration (while still reporting the issue via status and metrics), the current system lacks this granularity.

### Historical Context (1.x Behavior)

The legacy implementation ran [`envoy --mode validate`](https://www.envoyproxy.io/docs/envoy/latest/start/quick-start/run-envoy) against every xDS snapshot:

- Helped enforce that invalid configurations lead to the whole snapshot being rejected
- Lacked granularity that would allow for more fine-grained control over the replacement behavior
- Expensive (CPU/memory) for large snapshots
- Included webhook integration to reject invalid configurations at admission time, but this proved brittle due to performance, ordering, and availability issues

## Motivation

This proposal brings invalid route replacement to the 2.x version of the project, a feature that was previously available in the 1.x project. While not natively supported by the Kubernetes Gateway API specification currently, this feature has proven invaluable for gateway users over the years, preventing production outages and security incidents. The feature significantly improves the user experience and quality of life for gateway administrators and route creators, while providing stronger guarantees for route atomicity.

### Problem Scenarios

1. **Mis-routed requests**
   Example:
   **Route 1** matches `/path/bad` → Service A
   **Route 2** matches `/path` → Service B
   If Route 1 is invalid and dropped, requests to `/path/bad` erroneously match Route 2 and succeed (e.g., return 200), hiding routing errors and potentially misrouting traffic
2. **Accidental exposure without security policies**
   If a route relies on authentication (e.g., `/userInfo` protected by `TrafficPolicy`) and that policy fails to attach or is misconfigured, the route may be exposed without authentication. This creates a **fail-open** scenario where traffic to protected paths is allowed.
3. **Undetected invalid transformations**
   `TrafficPolicy` supports transformation filters using **Inja templates**. These templates can be valid at the CRD level but fail at the xDS or runtime level. Such failures are undetectable during admission and require deeper validation handling.
4. **Control plane produces invalid xDS due to unknown/unknowns**
   A bug or logic oversight may result in valid Kubernetes resources producing invalid xDS output. These cases are not caught at admission or schema-validation time and only manifest when Envoy NACKs the configuration.
5. **Dataplane loses valid config on restart**
   Even if Envoy initially NACKs a bad config and retains its last-known-good (LKG) config in memory, this state is not persisted across restarts. Upon restart, Envoy contacts the control plane with an empty cache, and if the current snapshot is still invalid, it will refuse to serve traffic entirely. There is no relay mechanism in Envoy to pull back its LKG from the control plane, making recovery fragile without upstream fallback logic.

### Goals

- Prevent invalid configurations from silently altering routing behavior, in addition to limiting the blast radius in multi-tenant environments
- Ensure security policies are never bypassed because of misconfiguration
- Provide clear, actionable feedback through status conditions and Prometheus metrics
- Maintain compliance with Gateway API specification for invalid route handling
- Add a global feature-gate (via Helm) to enable/disable the behavior
- Enable granular validation that isolates invalid configurations while preserving valid ones, providing better visibility into which resources cause validation failures

### Non-Goals

- Provide a validating webhook implementation to prevent misconfiguration at admission time
- Provide granular options to opt-out of route replacement per-route or per-Gateway
- Expose multiple independent toggles for route replacement and policy validation
- Allow user-defined custom response bodies or status codes for the replacement route
- Support invalid route replacement for non-Envoy data planes
- Handle the case where valid policies attached to routes have their `spec.targetRefs` field updated to break attachment, which currently results in policy-related Envoy config being dropped from the xDS snapshot and can lead to insecure routes being exposed
- Handle request-time errors that don't cause Envoy to NACK the xDS snapshot, such as valid transformation templates that reference non-existent templates. These errors don't prevent Envoy from accepting configuration updates and therefore don't require the route replacement mechanism proposed here

## Gateway-API Specification Compliance

- **Listener: `allowedRoutes`** (§ `Listener.allowedRoutes`)
  > *"Invalid Route rules can be ignored … If a Route rule transitions from valid to invalid, support for that Route rule should be dropped …"*
- **HTTPRouteRule: `matches`** (§ `HTTPRouteRule.matches`)
  > *"When no rules matching a request have been successfully attached … an HTTP 404 status code MUST be returned."*
- **ExtensionRef: `extensionRef`** (§ `HTTPRouteFilter.extensionRef`)
  > *"If the object referenced by an `ExtensionRef` cannot be found or is invalid, implementations MUST NOT skip the filter and SHOULD return an HTTP 500 status."*

These statements allow implementations to drop invalid rules. However, dropping the rule without installing a guard route can lead to the "mis-route" and "authn bypass" scenarios outlined above.

Additionally, for supported external policy types like authorization and global rate limiting, which defer behavior to a user-supplied GatewayExtension resource, we adopt the same approach as `ExtensionRef`. These BYO-style fields follow the same referential risk pattern and must result in HTTP 500 if the referenced resource is missing or invalid.

## Implementation Details

### Installation & Configuration

Rather than maintaining separate toggles for route replacement and xDS validation, which would create awkward UX edge cases (e.g. validation enabled but replacement disabled), we introduce a single enum-based configuration that clearly defines the behavior at each level.

This approach allows admins to disable xDS validation when performance is a concern, while still maintaining the same level of safety and visibility through route replacement.

```go
// ValidationMode determines how invalid routes and policies are handled during translation.
// Higher levels increase safety guarantees, but may have performance implications.
type ValidationMode string

const (
	// ValidationStandard rewrites invalid routes to direct responses
	// (typically HTTP 500), preserving a valid config while isolating failures.
	// This limits the blast radius of misconfigured routes or policies without
	// affecting unrelated tenants.
	ValidationStandard ValidationMode = "STANDARD"
	// ValidationStrict builds on standard by running targeted validation
	// (e.g. RDS, CDS, and security-related policies). Routes that fail these
	// checks are also replaced with direct responses, and helps prevent unsafe
	// config from reaching Envoy.
	ValidationStrict ValidationMode = "STRICT"
)
```

Default is `STANDARD` to provide safety guarantees by default. Users can opt in to `STRICT` mode for better safety guarantees.

Note: We deliberately avoid defining a `LEGACY`-esq mode, which would control the existing behavior where invalid routes are dropped entirely, due to the fail-open behavior and security risks outlined in the [Problem Scenarios](#problem-scenarios) section. If Gateway API conformance requirements or operational needs necessitate an opt-out mechanism, this can be added in a future iteration. See the [Open Questions](#open-questions) section for discussion on Gateway API compliance implications.

Users that wish to enable a validation level other than the default can add the following to their `values.yaml`:

```yaml
validation:
  level: strict
```

## Translator Changes

A tiered validation approach will be introduced to catch and handle invalid configurations at different stages:

1. **Admission Time**: CEL validation in extension CRDs prevents invalid configurations from being created, in addition to catching attachment related errors
2. **IR Construction**: Policy errors are checked during IR construction, influencing both status reporting and route replacement going forward
3. **xDS Generation**: Plugins validate their filter configurations using partial Envoy configs
4. **Final Validation**: Route translator validates the complete configuration before sending to Envoy, without validating the entire snapshot

This multi-layered pipeline approach ensures we catch invalid configurations as early as possible while maintaining consistent error handling across all stages, in addition to providing a more fine-grained control over which resources trigger route replacement.

### Current vs Proposed Behavior

#### Plugin Application (via `ApplyForX` methods)

- **Current**: Plugins return errors through `ApplyForX` methods, which the translator uses to set routes to `nil`. This approach was necessary to handle attachment-related issues at runtime, e.g. direct response plugin.
- **Proposed**: `ApplyForX` methods will become effectively pure functions that primarily mutate the output xDS configuration, but still require error handling to account for attachment edge cases where CEL validation cannot enforce attachment for GVKs we don't own (e.g., HTTPRoute, Gateway, etc.). This will remove most error handling from the route translator while preserving necessary attachment validation.

#### Spec to IR Construction

- **Current**: Policy errors encountered during IR construction are collected but not checked before applying the policy. The route translator's call site checks for non-nil errors and sets the output route to `nil` if any errors are found
- **Proposed**: Policy errors will be checked during IR construction before applying the policy. This will continue to influence status reporting while also triggering route replacement when policy IR reports errors, ensuring consistent error handling across all validation phases. Applies to both the proposed `STANDARD` and `STRICT` modes.

#### Structural Validation

- **Current**: Regex/path validation errors cause the route to be set to `nil`.
- **Proposed**: The same replacement logic will apply consistently across all validation failures. Applies to both the proposed `STANDARD` and `STRICT` modes.

#### xDS Validation

- **Current**: No validation of generated xDS configuration
- **Proposed**: Implement two-tiered xDS validation when `STRICT` mode is enabled.
  1. Plugin-level validation during IR construction
     - Relevant plugins first perform PGV validation against their filter types stored in the IR
     - If PGV passes, plugins then validate their filter configuration by converting their IR to a partial Envoy bootstrap config
     - Uses a fake listener containing the plugin's filters
     - Runs `envoy --mode validate` against this partial config
     - Optimize by comparing policy status's observed generation against current generation to short-circuit validation for metadata-only updates (and ignore updates to attached HTTPRoute status) and by only validating the plugin's filters
     - **Note**: For policies that attach to backends (e.g., Kubernetes Services), similar validation will be performed during backend translation rather than at the plugin layer. This ensures the output Envoy cluster has all required information (e.g., service discovery type) before validation, preventing partial validation that could mark config as valid but result in Envoy NACKs
  2. Route translator validation
     - Validates the complete output route configuration
     - Caches validation results for performance
     - Ensures the final xDS configuration is valid before being sent to Envoy

### Future Work: Error Categorization During Plugin IR Translation

During plugin IR translation, different types of errors may be encountered that require different handling strategies. For example, a `TrafficPolicy` that references a non-existent `GatewayExtension` (e.g. an ExtAuth provider) would return a referential error during IR construction. The route translator needs sufficient information to determine whether it should replace the entire route or just the specific policy configuration.

This proposal may require implementing sentinel error types or error categorization mechanisms to differentiate between:

- **Referential errors**: Missing resources that could be resolved through eventual consistency (e.g., missing `GatewayExtension` provider)
- **Structural errors**: Configuration issues that require user intervention (e.g., invalid Inja template syntax)
- **Semantic errors**: Logical issues that may require policy-specific handling

The implementation will rely on sentinel error reporting from plugins to make these decisions, as plugins are closest to the source and most informed on how to handle errors properly. While this error categorization is likely necessary for optimal route replacement behavior, it is considered out of scope for this initial proposal. Ex: should an invalid retry policy replace the entire route or do we silently drop it? The implementation PR will explore this requirement and the proposal may be updated based on findings.

### WIP - Observability

| Metric                             | Type    | Labels                                                    |
| ---------------------------------- | ------- | --------------------------------------------------------- |
| `invalid_route_replacements_total` | Counter | `gateway`, `route_namespace`, `route_name`, `error_class` |
| `policy_validation_failures_total` | Counter | `policy_kind`, `namespace`, `name`, `reason`              |

Status reporting:

- HTTPRoute: `PartiallyInvalid=True` if at least one rule was replaced
- Policy: `Accepted=False` / `Invalid=True`

### Test Plan

- Unit: add setup test case to exercise different route replacement modes
- Gateway Translator: N/A unless it supports configuring global settings (TODO: confirm)
- E2E: simulate misroutes, insecure paths, transformation failures
- Performance: no available suite for the 2.x codebase. Needs to be done to measure impact of xDS validation on performance at scale

## Alternatives

### Drop invalid routes (status quo)

When the route translator encounters an error, the output route is set to `nil`, and the rule is omitted from the xDS snapshot.

Pros:

- Already implemented
- Technically compliant with Gateway API ("invalid rules can be ignored")

Cons:

- Fail-open behavior: requests fall through to broader or default routes, potentially reaching unintended backends or bypassing security policies
- No visible signal to users that a route was dropped
- Operational ambiguity: manifests as 404s, which has operational overhead and can be misdiagnosed as a backend failure

### Admission-time rejection

Use validating admission webhooks or CEL-based CRD constraints to reject invalid configurations when resources are created or updated.

Pros:

- Immediate, early feedback loop for users
- Prevents invalid resources from entering the system

Cons:

- Incomplete: cross-resource and IR-level validation often requires logic beyond CEL's capabilities
- Adds availability and ordering concerns due to webhook dependencies
- Can be brittle or overly restrictive in dynamic environments

### Pause xDS updates

During error handling, the control plane halts propagation of the next xDS snapshot. This relies on Envoy's in-memory last-known-good (LKG) config.

Pros:

- Prevents invalid configuration from reaching the dataplane
- Retains security and correctness by preserving the last valid snapshot

Cons:

- Multi-tenancy impact: one invalid route blocks unrelated changes from other teams
- Lacks durability: LKG is not persisted—on Envoy restart, traffic serving may halt if errors persist
- Introduces complexity to manage rollback, durable state, and snapshot pinning, etc

## Glossary

| Term              | Definition | Potential Risk |
|-------------------|------------|-------------------------|
| Referential Error | A failure caused by referencing a non-existent or unresolved resource (e.g., missing Service, ReferenceGrant, ExtensionRef, GatewayExtension, or invalid policy targetRefs). | Invalid routes may be silently removed; requests fall through to a catch-all and return 404/503—or worse, reach an unintended backend—creating correctness and security gaps that are hard to trace from Kubernetes status alone. |
| Structural Error | A failure due to schema or config issues (e.g., bad regex, mutually exclusive fields, invalid Inja/CEL expressions). Detected via CRD validation, PGV, or xDS rejection. | If not rejected cleanly, the controller can program broken config that Envoy NACKs, causing reconcile loops, partial outages, or proxy crash-loops; operators see healthy status in Kubernetes while traffic drops or flaps. |
| Semantic Error | Configurations that are schema-valid and referentially correct but logically incorrect or unsafe. Examples include fail-open security behavior, conflicting path rewrites, or silent runtime errors in filters. | Unsafe logic ships to the data-plane: auth bypass, wildcard route hijacking, redirect loops, MTLS downgrades, etc. Traffic continues to flow, but in ways that violate intent or security posture, leading to incidents despite all status conditions showing green. |

### Error Classification

This section outlines the difference between error handling for **Gateway API core resources** and **extension APIs** like `TrafficPolicy`.

> See the [Glossary](#glossary) for definitions on the different error types.

#### Core Gateway API Resources

These include `xRoute`, `Gateway`, and related standard resources provided Kubernetes.

| Error Type  | Examples                                                                 |
| ----------- | ------------------------------------------------------------------------ |
| Referential | `backendRef` points to non-existent `Service`; missing `ReferenceGrant`  |
| Structural  | Invalid `match` config (bad regex), mutually exclusive filters           |
| Semantic    | Weight sum of 0; misconfig that passes validation but routes incorrectly |

These errors are governed by explicit spec guidance. For example, unmatched routes must 404, and invalid but matched routes must fail with 500.

#### Extension APIs (e.g. `TrafficPolicy`)

These are project-specific resources layered on top of core Gateway APIs.

| Error Type  | Examples                                                                         |
| ----------- | -------------------------------------------------------------------------------- |
| Referential | Missing `gatewayExtensionRef` for ExtAuth or RateLimit provider                  |
| Structural  | Inja templates with invalid syntax; bad CEL expressions; missing required fields |
| Semantic    | - Auth policy misconfiguration disables security (e.g., fail-open behavior)      |
|             | - Attachment misconfiguration: `spec.targetRefs` does not select expected `HTTPRoute`, leaving paths unprotected |
|             | - Policy silently ignored due to logic bugs despite `Accepted=True` |
|             | - Runtime transformation failure (e.g., invalid Inja templates evaluated in Envoy) |

Unlike the core APIs, these errors may only surface during IR construction, partial xDS validation, or even at runtime in Envoy. Therefore, handling must occur inside the translator's plugin and validation pipeline.

## Open Questions

- [ ] Does providing an opt-out mechanism (e.g., an `OFF` mode that drops invalid routes) break compliance with the Gateway-API specification? While the specification allows implementations to "ignore" invalid rules, the fail-open behavior and security risks of dropping routes entirely may not align with the intent of the specification. This question is particularly relevant if operational requirements necessitate providing users with the ability to disable route replacement entirely.
