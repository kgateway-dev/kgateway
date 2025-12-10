# EP-11937: Higher-Level Policy Validation and Replacement Strategy

* Issue: [#11937](https://github.com/kgateway-dev/kgateway/issues/11937)

## Background

The TrafficPolicy API supports attachment at various levels in the routing hierarchy, from individual HTTPRoute rules to Gateway-wide configuration. While the 2.x architecture includes route replacement for invalid policies at the route level, there is a gap in handling invalid policies attached at higher levels (e.g., gateway-wide and listener-wide). This represents an edge case in the overall 2.x route replacement approach but addresses a critical operational and security concern.

## Motivation

Higher-level policy attachment is a valid and valuable use case in Gateway API architectures. For example, org-wide authentication policies are commonly attached at the Gateway level to ensure consistent security posture across all routes and services managed by that Gateway. Similarly, global rate limiting or CORS policies may be applied at the Listener or xListenerSet level to enforce uniform behavior for all traffic on specific ports or protocols.

Currently, when invalid policies are attached at higher-levels, the following behavior occurs:

- Problematic TrafficPolicy status reports Accepted=false with a custom reason
- Malformed configuration is still translated and sent to Envoy, leading to Envoy NACKs
- Valid routes become unavailable due to unrelated policy failures. This makes troubleshooting difficult
- Inconsistent error handling may lead to fail-open scenarios where security policies are silently bypassed

Aligning on the right approach is critical to proactively avoid complete outage across all tenants, prevent breaking behavior changes for future 2.x minor versions, and provide a consistent UX early in the 2.x lifecycle.

## Goals

- Ensure invalid policies attached at higher-levels are gracefully handled
- Provide clear status reporting, logs, and metrics to enable quick resolution of the issue
- Prevent invalid security policies from creating fail-open scenarios

## Non-Goals

- Implementing any partial xDS updating mechanisms
- Support for any per-policy opt-out mechanisms for replacement behavior
- Support for non-Envoy data planes (i.e. agw)
- Any formal error classification framework that enables the route translator to make more informed decisions about how to handle different types of errors. This is a premature optimization and can be addressed in a future enhancement if it's needed
- Any migrations or backward compatibility with the initial 2.0 release. This is a breaking behavior change
- Adopting this paradigm for other APIs (e.g., HTTPListenerPolicy, BackendConfigPolicy). Those will be addressed in a future enhancement that mirrors the fallback semantics at LDS/CDS scopes

## Implementation Details

### Proposed Approach

The challenge at each entrypoint is balancing safety with impact. For invalid higher-level policies, we have three constraints:

- Cannot use the translated policy's Envoy config that leads to a NACK
- Cannot avoid stripping the problematic typed filter config if it leads to fail-open behavior
- Dropping entire listeners is very aggressive, but may be necessary in some cases

This proposal favors deterministic fallback in the translator rather than pausing xDS updates or dropping the entire listener. Fallback means replacing invalid policy configurations with synthetic configurations that return HTTP 500 responses. This provides clear failure semantics without halting all configuration updates to the proxy, which is especially important in dynamic environments like Kubernetes where endpoint sets change frequently.

#### Fallback Strategy

When invalid policies are detected at higher levels, the translator uses different fallback approaches based on the attachment scope. For route configuration failures (Gateway-wide or HTTPS listener attachment), the entire route configuration is replaced with a fail-closed synthetic response.

For virtual host failures (via HTTP listener attachment), only the affected virtual host is replaced while preserving its domain identity to maintain correct routing precedence. The latter is needed because Envoy rejects route configurations with multiple virtual hosts using the same domain (including wildcard domains like `*.example.com`), which would cause a NACK.

Overall, this strategy helps ensure fail-closed behavior while maintaining operational visibility and preventing Envoy NACKs. See the [Alternatives](#alternatives) section for more details on the different approaches considered.

### Observability

- Emit custom metrics using the framework introduced in the 2.1 cycle to track replacement events and affected scopes
- Generate structured logs that capture the root cause of the failure
- Surface status conditions on the problematic TrafficPolicy and the relevant Gateway or xListenerSet resources to ensure the scope of the failure is visible to operators and automation

## Alternatives

### Status Quo: 1.x Drop the entire listener

Adopt the 1.x behavior of dropping the entire listener when invalid policies are attached at the higher levels. This guaranteed fail-closed semantics but was very aggressive.

Pros:

- Ensures deterministic fail-closed behavior by removing any chance of invalid configuration being delivered to Envoy
- Simple to implement, since the entire listener is removed rather than attempting partial replacement
- Consistent with the historical behavior of the 1.x data plane, which some operators may already be familiar with

Cons:

- Removing listeners causes [Envoy connection draining](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/operations/draining). Attaching invalid policies Gateway-wide in this model forces everything behind it through a draining cycle
  - Draining is also triggered on LDS modifications that require listener re-creation (e.g., changing port bindings), so even non-removal changes can disrupt large numbers of active connections
- Extremely aggressive, as all routes and vhosts under the listener are discarded even if only a single policy is invalid
- Leads to large-scale outages that may affect unrelated tenants sharing the same listener
- Difficult for operators to debug, as the listener simply disappears from the proxy configuration without a clear indication of which policy triggered the drop
- Inconsistent with the 2.x model of route replacement, which favors preserving as much of the routing tree as possible while still failing closed

Overall, while the 1.x listener drop strategy provided a strong fail-closed guarantee, it created too much operational risk in multi-tenant environments. It is not recommended as the default for 2.x but is included here as a baseline for comparison.

### Status Quo: 2.x Preserve Routing Tree, Strip Invalid Policies

When invalid policies are detected at higher-level, strip the typed filter config while preserving the underlying routing tree.

Pros:

- Minimal impact on valid traffic
- More granular error handling
- Allows valid policies to be applied even if invalid policies are present

Cons:

- Significant impact on security posture: fail open behavior may lead to bypassing security policies and exposing insecure routes

Overall, this approach is too aggressive and creates a significant operational risk. Especially since the TrafficPolicy is a monolithic API, and having to classify each policy type under that API's umbrella as security-sensitive or not introduces a lot of complexity.

### Pause xDS at Last Known Good (Gateway-wide)

In this alternative, the control plane halts all xDS updates for a Gateway once it detects invalid global policy.

> Note: Listener-level and xListenerSet-level policies are potentially affected by this approach as well.

The proxy continues to serve the last known good configuration until the problem is resolved. This assumes that global policies at the Gateway scope are owned and managed by platform or gateway teams. These teams generally prioritize overall availability over strict correctness, and prefer to keep applications running with the most recent valid configuration rather than risk a full outage.

The control plane would bubble up the invalid policy state through Gateway and TrafficPolicy status, emit metrics that indicate the Gateway has been paused, and leave it to the owning team to fix the invalid configuration before updates resume.

Pros:

- Prevents an immediate outage for all applications served by the Gateway
- Preserves enforcement posture and stability if the LKG config contained the desired policy
- Provides a clear operational signal (e.g., status and metrics) that something is wrong without exposing all tenants to failures

Cons:

- Relies on Envoy holding onto its LKG config, which is not guaranteed across pod restarts. A restart of the underlying proxy may result in losing that state and no longer serving traffic correctly
- There is no first-class support in Envoy for fetching or persisting LKG configuration from the proxy. Prior work such as envoyproxy/xds-relay attempted to address this, but it is unmaintained
- The control plane currently lacks a proper mechanism to implement pause semantics while ensuring the proxy keeps serving valid config. This was an area where the 1.x project invested significant effort but never reached a fully reliable solution
- Creates the risk of hidden drift: applications continue serving traffic with stale configuration, and operators must rely entirely on metrics and conditions to detect the paused state

Additionally, freezing EDS updates is especially risky in Kubernetes or other dynamic environments where pods are frequently added or removed. When scaling events or rollouts occur, the proxy will not learn about new endpoints and may continue to send traffic to pods that are being terminated. This can result in uneven load balancing, retries, and degraded service availability even though the Gateway itself remains up.

While this approach avoids an outage across all applications, there's a distinct trade-off w.r.t. correctness and introduces operational risk around proxy restarts and state management. It may be a reasonable stopgap for platform-managed global policy enforcement, but a first-class solution for last known good persistence would be needed for it to be robust.

### Hybrid: Pause Config, Allow EDS to Update

This alternative builds on top of the previous one, but keeps LDS/RDS/CDS frozen, but let EDS refresh so rollout/scale changes still load-balance correctly. See the previous section for more details.

Pros:

- Avoids 500 storms, stale endpoints, allows for better app rollouts and scaling characteristics

Cons:

- Introduces complexity, more plumbing, etc. to implement this correctly

Overall, this approach shares the same cons as the previous approach, but avoids the risk of hidden drift. The additional complexity on the implementation side is not worth the trade-off.

## Appendix

### Historical Context: 1.x Implementation

In the legacy 1.x implementation, invalid policy applied at the vhost tier resulted in the entire listener being dropped from the resulting `Proxy` configuration. This was a more destructive action than route replacement, but it was necessary to prevent Envoy from rejecting the entire configuration.

When this scenario occurred in 1.x:

- An error was logged indicating the validation failure
- The VirtualService resource containing the invalid VirtualHostOption policy had the error reported on its status
- The status included detailed error information with reason codes like "ProcessingError" and specific validation failure details
- The behavior effectively prevented Envoy NACKs by proactively removing problematic configuration

This 1.x approach demonstrates that higher-level policy validation and replacement is both necessary and implementable, providing a foundation for the 2.x solution.

### Recap: Policy Attachment and xDS Mapping

It's important to understand how each level of policy attachment resolves into the underlying Envoy xDS configuration. This provides the context for why failures are handled differently depending on the attachment point.

> Note: See [PR #11272](https://github.com/kgateway-dev/kgateway/pull/11272) for more details on this change.

- Gateway attachment:
  - Policy applied at the Gateway level projects into the route configuration, affecting all listeners and routes under the Gateway
- Listener or xListenerSet attachment:
  - For HTTP listeners or xListenerSets, policy projects into the virtual host
  - For HTTPS listeners or xListenerSets, policy projects into the route configuration, even though it is attached at the listener level
- HTTPRoute or route rule attachment:
  - Policy applies directly to the Envoy route

This mapping is the reason the fallback strategy differs by scope.

### Recap: Current Validation & Replacement Model

Today, route-level policy is validated before being delivered to Envoy. Policies attached directly to `HTTPRoute` or route rules are checked against Envoy’s xDS validation. If invalid, those routes are replaced with direct response actions, ensuring the proxy never receives configuration that would NACK.

The route translator also validates the fully computed route configuration (i.e. RDS-style). For routes with invalid matchers, the route is dropped. Otherwise, routes that have invalid typed-per-filter configuration are replaced with a direct response action.

One current limitation is that the TrafficPolicy plugin does not classify errors. All errors are treated uniformly as fatal, and the route translator responds by replacing the affected routes. This means that referential errors (e.g., a missing `GatewayExtension` due to eventual consistency) are handled the same way as structural or semantic errors that would deterministically lead to an Envoy NACK. As a result, temporary conditions can trigger unnecessary replacements, rather than being surfaced as warnings or retried once the missing resource appears. Solving this in the control plane is out of scope for this proposal, but a formal error classification framework may be considered in the future.

Additionally, this pattern ensures deterministic, fail-closed behavior at the route scope. However, policies attached at higher levels in the routing hierarchy (Gateway, listener, xListenerSet) are not yet validated or replaced in the same way, which leaves gaps in consistency. Similarly, other APIs such as `HTTPListenerPolicy`, `BackendConfigPolicy`, and `Backend` do not yet adopt this paradigm (e.g., cluster-level validation analogous to CDS).
