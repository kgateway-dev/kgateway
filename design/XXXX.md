# EP-XXXX: Orphaned Resource Status Reporting

* Issue: [#XXXX](https://github.com/kgateway-dev/kgateway/issues/XXXX)

## Background

Currently, status for Kgateway CRDs (HTTPRoutes, TrafficPolicies, etc.) are only reported at the end
of translation phase. Resources are processed through a reverse lookup mechanism where parent resources trigger
the discovery and processing of child resources. This approach works well for correctly configured resources but
leaves stale status for "orphaned" resources—resources that reference non-existent or invalid parent/target
references.

When a resource is orphaned (e.g., an HTTPRoute with all invalid `parentRefs`, or a TrafficPolicy with all
invalid `targetRefs`), it is never picked up during translation. As a result, the status from previous
configurations remains stale and is never cleared. Users can technically detect orphaned resources by observing
a mismatch between `observedGeneration` and the current generation in the status, but this requires subtle
prior knowledge of Kubernetes status semantics and is not the best user experience.

This problem affects user experience in several ways:
- Users see stale status from previous valid configurations, making it unclear the resource is now orphaned
- Troubleshooting becomes difficult as stale status can be misleading
- The behavior deviates from Kubernetes best practices

## Motivation

Ensuring status accurately reflects the current state by clearing stale status for orphaned resources

### Goals

- Clear stale status for resources that become orphaned
- Maintain consistency with existing status reporting patterns
- Handle partial validity scenarios (some refs valid, some invalid) correctly
- Align with Gateway API and Kubernetes ecosystem best practices
- Minimize performance overhead of status management
- Only manage status owned by kgateway controller (respect controllerName)

### Non-Goals

- Change the core translation mechanism or reverse lookup approach
- Implement "pending" states that might confuse transient controller operations with user errors
- Validation of references
- Modify status owned by other controllers (respect controllerName boundaries)
- Modify the status syncer architecture significantly

## Implementation Details

### High-Level Design

The solution introduces **status clearing** using status collections during the intermediate representation (IR) construction phase. Each
resource with existing status condition gets an empty status entry in the status collection at construction time, essentially marking them "dirty". After translation completes,
the status collection is merged with the translation ReportMap. This ensures orphaned resources have their stale
status updated and cleared, while valid resources have their status updated from translation.

#### Current Flow (Problem)

```mermaid
graph TD
    A[Resources in K8s] --> B[Translation Phase]
    B --> C{Reverse Lookup}
    C -->|Valid Parent Refs| D[Process Child Resources]
    C -->|Invalid/Missing Parent Refs| E[Resource Ignored]
    D --> F[Generate IR]
    F --> G[Generate xDS]
    G --> H[Status Reporter]
    H --> I[Update Status in K8s]
    
    style E fill:#ff9999
```

#### Proposed Flow (Solution)

```mermaid
graph TD
    A[Resources in K8s] --> C[IR Construction Phase]
    C --> D[All Resources Get Empty Status Entry]
    D --> E[Translation Phase]
    E -->|Reverse Lookup| F[Process Valid Resources]
    F --> G[Generate xDS]
    G --> H[Translation ReportMap]
    D --> I[Status Collection - Empty Status]
    I --> J[Merge Status Collection into ReportMap]
    H --> J
    J --> K[Update Status in K8s]
    
    style I fill:#0000ff
    style J fill:#009900
```

### Implementation Approach

The solution leverages the existing `ReportMap` infrastructure and introduces status clearing during IR
construction phase using a status collection pattern. This approach is consistent with agentgateway status
reporting and ensuring orphaned resources have their stale status cleared.

#### 1. Empty Status Creation at IR Construction Phase

During the IR construction phase (when building intermediate representation from CRDs), create empty status
entries for all resources with existing status conditions:

**For HTTPRoute:**
- Create an empty status entry in the HTTPRoute status collection for each HTTPRoute that has existing status condition with kgateway as controller name
- Empty status means no parent status conditions to begin with, which will clear any stale parent refs
- All routes proceed to translation phase via reverse lookup

**For TrafficPolicy:**
- Create an empty status entry in the TrafficPolicy status collection for each TrafficPolicy with existing status condition
- Empty status means no ancestor status conditions to begin with, which will clear any stale target refs
- All policies proceed to translation phase via reverse lookup

**For other CRDs with references:**
- Create empty status entries during IR construction
- Empty status clears all stale conditions
- Be careful of Gateway API resources like HTTPRoute, as those can be owned by other controllers simultaneously

#### 2. Status Collection Pattern

Each resource type has its own status collection that captures status entries:

```go
// Status collection for HTTPRoutes (created during IR construction)
httpRouteStatusCollection, irCollection := krt.NewStatusCollection(
    func(ctx krt.HandlerContext, route *gwv1.HTTPRoute) (*gwv1.RouteStatus, *ir.HttpRouteIR) {
        status := &gwv1.RouteStatus{
            Parents: []gwv1.RouteParentStatus{},  // Empty - clears all stale parent status
        }
        
        // TODO: add empty status if there is existing status condition, check by condition parent length not 0
        for _, parentRef := range route.Spec.ParentRefs {
            controllerName := ptr.OrEmpty(parentRef.ControllerName)
            if controllerName == "" || controllerName == wellknown.GatewayController {
                // This parent belongs to kgateway, include empty status entry
                status.Parents = append(status.Parents, gwv1.RouteParentStatus{
                    ParentRef:  parentRef,
                    Conditions: []metav1.Condition{},  // Empty conditions
                })
            }
        }
        
        // Build IR (normal IR construction logic)
        routeIR := constructIR(ctx, route)
        
        return status, routeIR
    },
)
```

**Key characteristics:**
- Status collection contains empty status entries for all resources with existing status conditions
- Empty status will overwrite/clear any stale status
- For HTTPRoute, only check status for parentRefs controlled by kgateway (controllerName check)
- Translation will populate ReportMap with actual status for valid refs (existing behavior)

#### 3. Status Merging

At the end of translation, merge status collection entries into the existing ReportMap:

**Merging Logic**:
1. If resource has entries in ReportMap (actually translated), overlay those statuses
    - ReportMap with actual translation status entries merge corresponding empty status entries
2. Final result:
    - **Fully orphaned resources**: Only empty status (clears all stale conditions)
    - **Fully valid resources**: ReportMap status merged with all empty entries (normal path)
    - **Partially orphaned**: ReportMap merged with some refs, others remain empty (cleared)

Merge will be specific to each resource, and following agentgateway's pattern, each resource will register themselves
and corresponding merging function to the status syncer. Status syncer will be added with calling all registered merge
functions.

#### 4. Concrete Example: Partially Orphaned HTTPRoute

Consider an HTTPRoute with a parent ref that previously had valid status, but now is changed to an invalid ref:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-route
  namespace: default
spec:
  parentRefs:
  # changed from valid to invalid
  # - name: valid-gateway
  - name: missing-gateway
  rules:
  - matches:
    - path: {type: PathPrefix, value: /app}
    backendRefs:
    - name: my-service
      port: 80
```

**Previous Status (Stale)**:
```yaml
status:
  parents:
  - parentRef: {name: valid-gateway}
    conditions: [{type: Accepted, status: "True", ...}] # Wrong - should be cleared
```

**Processing Flow:**

1. **IR Construction Phase**:
    - Create empty status collection entry:
      ```go
      httpRouteStatusCollection["default/my-route"] = {}
      ```

2. **Translation Phase**:
    - `missing-gateway` is NOT picked up during translation and not in ReportMap

3. **Merge Phase**:
    - Empty status from status collection merge with ReportMap from translation (empty report map in this example)
    - Final status:
        - `missing-gateway`: Empty conditions (cleared the stale status)

4. **User sees**:
   ```yaml
   status:
     parents:
     # CLEARED - no stale status remains
   ```

### Status Condition Format

Following Gateway API conventions:

**For Orphaned Resources:**
```yaml
status:
  parents:
  - parentRef:
  # Empty - stale status cleared
```

**For Valid References:**
```yaml
status:
  parents:
  - parentRef:
      name: valid-gateway
    conditions:
    - type: Accepted
      status: "True"
      reason: Accepted
    - type: ResolvedRefs
      status: "True"
```

**For Partial Validity:**
```yaml
status:
  parents:
  - parentRef:
      name: valid-gateway
    conditions:
    - type: Accepted
      status: "True"
    - type: ResolvedRefs
      status: "True"
  # Only valid shown
```

### Performance Considerations

- Creating empty status is lightweight
- No additional API calls to Kubernetes
- Uses existing collection infrastructure

### Integration with Existing Systems

The proposed solution maintains compatibility with:
- **AgentGateway status reporting pattern**: Similar status collection approach
- **Translation reporter**: Existing translation status reporting unchanged

### Test Plan

TBD

## Alternatives

### Alternative 1: Clear Missing Status at End of Translation

**Approach:** At the end of translation, iterate through all resources and clear status for any
resources that have existing status but not reported.

**Pros:**
- Minimal changes, straightforward

**Cons:**
- Cannot add validation or other things in the future

### Alternative 2: Pending State

**Approach:** Introduce a "Pending" status state for resources being processed but not reported at the end.

**Pros:**
- Explicitly shows resources are not processed

**Cons:**
- Users cannot distinguish between controller operations and configuration errors
- Misleading for permanently orphaned resources

### Alternative 3: Mark Orphaned / Invalid Only

**Approach:** Mark resources as "orphaned" or "invalid".

**Pros:**
- Simple implementation
- Low overhead

**Cons:**
- Might overwrite other controller status, if the parentRef is changed to Gateways owned by other controllers

## Open Questions

1. **ControllerName handling**: For HTTPRoute parentRefs, should we clear status for refs with unspecified
   controllerName, or only those explicitly set to kgateway's controller?
