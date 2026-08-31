# EP-13586: Referenced-only cluster discovery

Status: Proposed

- Issue: [#13586](https://github.com/kgateway-dev/kgateway/issues/13586)
- Related: [#10639](https://github.com/kgateway-dev/kgateway/issues/10639) (duplicate ask), [#14184](https://github.com/kgateway-dev/kgateway/issues/14184) (per-client xDS coherence)
- Prerequisites: **none**. This EP is self-contained against `main` at `bc57f656fc`. Every mechanism it relies on is either already merged or fully specified here.

## Background

By default kgateway emits an Envoy CDS cluster, and an EDS `ClusterLoadAssignment`, for **every** Service in its discovery scope, whether or not any route references it. A user inspecting `config_dump` sees a cluster for every Service in the cluster.

The cost is real and reported in #13586. One environment measured:

- ~279 Services discovered, of which only 16 are targeted by an `HTTPRoute`.
- 93,126 metrics carrying an `envoy_cluster_name` label on each Envoy instance, more than the entire `kube-state-metrics` deployment, multiplied per replica.

The cost scales multiplicatively: it is paid per proxy replica, per gateway, so fleets with many gateways carry the full unreferenced inventory in every proxy's memory and in every proxy's stats output.

The two existing mitigations are insufficient for common topologies:

- `statsMatcher` (GatewayParameters) trims stats but is capped at 16 expressions and is brittle as internal Service names churn.
- `discoveryNamespaceSelectors` scopes discovery by namespace, which does not help when public and internal workloads share namespaces.

### Why kgateway emits all clusters today

This is deliberate, not accidental. The maintainer rationale from the issue thread: there is no safe way to change a route's destination without dropping traffic unless the destination cluster already exists. Consider `/foo: service-a -> service-b`:

- The route update (RDS) and the cluster update (CDS) are applied to Envoy as separate events.
- If the route retargets to `service-b` before `service-b`'s cluster exists, `/foo` returns `503 NC` (no cluster) for the gap.

Pre-creating a cluster for every Service guarantees the destination always exists, so route changes never reference a missing cluster. **Emit-all is a make-before-break workaround for not having safe cluster and route transitions.** This EP builds those transitions, which removes the justification.

### What changed on main since this EP was first drafted

The first draft of this EP was written against an older `main` and was **wrong about its own foundation**. Correcting that is the primary reason for this rewrite, and it is why the prerequisite list above is now empty.

- **`714190cf85` reverted the per-client xDS readiness gates (#13868, #13958)** in favor of a first-connect grace period. That revert deleted `collectReferencedClusters`, `collectResourceClusterReferences`, `collectProtoClusterReferences`, `GatewayXdsResources.ReferencedClusters`, `findMissingReferencedClusters`, and `findMissingReferencedEndpointResources`. The earlier draft presented these as an existing foundation to extend. They are not on `main`.
- **`publishGate`, `resolveDeferredPerCluster`, `clientDeparted`, `filterEndpointResourcesForClusters`, `baseEnvoyCluster`, and `uccClusterDelta` do not exist on `main` and never did.** They are proposals in unmerged work (#14343, #14257, and the #14184 publication-engine PR). The earlier draft listed them under "already implemented" and made itself depend on all three landing first. This rewrite drops that dependency.
- **Ordered ADS already ships.** `Settings.EnableOrderedAds` (`api/settings/settings.go:272`, `KGW_ENABLE_ORDERED_ADS`, default `false`) wires `sotwv3.WithOrderedADS()` in `pkg/kgateway/setup/controlplane.go:114`. Its own doc comment already records that it changes neither ACK skew nor removal ordering. This EP does not need to propose it.
- **The ADS wire-order probe harness already exists in-tree**: `pkg/kgateway/setup/ads_delivery_order_test.go`, covering quiet-stream addition, ACK skew, and combined removal, each run in both server modes. The earlier draft proposed porting such a harness; it is already there, it passes, and it is the empirical authority cited below.
- **NACK observability already exists**, as `envoy_xds_rejects_total` and `envoy_xds_rejects_active` (`pkg/kgateway/setup/envoy_error.go`), fed by `logNackCallback.OnStreamRequest`, which already inspects every `DiscoveryRequest` and its `ErrorDetail`. The earlier draft cited a `kgateway_xds_nacks_total` that does not exist. That same callback is the per-stream observation point this EP needs.
- **A first-connect delay is the current convergence mitigation**: `KGW_XDS_FIRST_CONNECT_DELAY` (default 1s) in `pkg/krtcollections/uniqueclients.go`, slept on the first `DiscoveryRequest` of a new stream.
- **`bc57f656fc` scoped the local-cluster EDS resource** to clients that asked for it (`pkg/kgateway/proxy_syncer/local_cluster.go`). This creates a trap for naive EDS filtering, addressed under EDS alignment below.

What *is* on `main` and useful: `snapshotPerClient` (`pkg/kgateway/proxy_syncer/perclient.go:55`) assembles per-client snapshots; `PerClientEnvoyClusters.FetchClustersForClient` (`backends.go:58`) supplies the per-client backend clusters; `filterEndpointResourcesForStaticClusters` (`perclient.go:273`) is a working precedent for aligning EDS to CDS at assembly; and `wellknown.BlackholeClusterName` (`pkg/kgateway/wellknown/constants.go:45`) is the sentinel for unresolvable backends.

## Motivation

Users are asking for a control plane that tells each proxy about the backends it actually uses. Today's answer is a workaround (`statsMatcher`) that hides the symptom on the stats endpoint while the config, the memory, and the per-replica multiplication remain. The blocking reason has always been transition safety, not desirability, so the fix is to build the transitions rather than to keep paying for emit-all.

The key realization is that this is two independent problems, and the earlier draft conflated them:

1. **Which clusters belong in the snapshot.** A content question. Solved by a referenced-set filter, and its entire difficulty is *completeness*: missing one reference is a permanent outage, not a blip.
2. **When each change reaches Envoy.** A delivery question. Snapshot coherence does not imply application order, so this needs staged publication, in both directions.

```mermaid
flowchart LR
    A["Why emit-all exists:\na route retarget drops traffic\nif the new cluster is not already present"] --> B["Make the emitted set change safely:\npublish the cluster strictly before the route,\nretire the cluster strictly after"]
    B --> C["Transitions are drop-free,\nso emit-all is no longer load-bearing"]
    C --> D["Emit only referenced clusters,\nso 13586 is solved"]
```

## Goals

- Emit CDS and EDS only for clusters the generated Envoy configuration actually references, eliminating unreferenced-cluster bloat in `config_dump` and `/stats`.
- Preserve make-before-break in **both** directions, with no `503 NC` on a route retarget and no endpoint drop on a de-reference, using mechanisms specified in this EP.
- Make referenced-set completeness a **CI-enforced invariant** rather than a review obligation, because under-collection is a silent outage.
- Depend on no unmerged work, so this can land independently of #14184, #14343, and #14257 while composing cleanly with them.
- Keep the behavior opt-in until a soak proves it.

## Non-Goals

- Changing how Services are watched at the informer level. This EP filters what is *emitted*, not what is *watched*. Coarser watch-level scoping remains the job of `discoveryNamespaceSelectors` and of the proposed Service label selector (Alternatives, Option B).
- Shrinking **control-plane** per-client memory. Filtering at assembly shrinks what each Envoy holds and reports, which is what #13586 asks for. The per-client cluster rows still materialize every backend; collapsing that is #14343's job, and it composes with this EP (Alternatives, Option E).
- A user-facing `Backend` kube type for explicit cluster declaration (Alternatives, Option C).
- Re-litigating per-client publication freezes. That is #14184.

## Implementation Details

### Configuration

- `Settings.ClusterDiscoveryMode` (`KGW_CLUSTER_DISCOVERY_MODE`), enum `All` (default) and `Referenced`, following the typed-enum plus `Decode` pattern of `ValidationMode` (`api/settings/settings.go:15`).
- `Settings.ClusterReferenceAhead` (`KGW_CLUSTER_REFERENCE_AHEAD`, `metav1.Duration`, default a few seconds), the addition-side hold. `0` publishes cluster and retarget together, accepting the ACK-skew blip.
- `Settings.ClusterDereferenceGrace` (`KGW_CLUSTER_DEREFERENCE_GRACE`, `metav1.Duration`, default a few seconds), the removal-side retention. `0` prunes immediately, accepting the removal race.

Two knobs rather than one, because they trade different things: de-reference grace trades `config_dump` residency, reference-ahead trades route-edit latency. Both are separately bounded by the publication coordinator's deadline. `EnableOrderedAds` remains an independent, already-shipped hardening option.

### Translator and Proxy Syncer

#### Defining the referenced set

The emitted backend-cluster set for a gateway is the set of translated cluster names that appear anywhere in that gateway's **generated** RDS and LDS protos.

Derivation is a generic `protoreflect` walk over `GatewayXdsResources.Routes` and `.Listeners`, unmarshalling every `anypb.Any` so that names inside `typed_config` are visible, collecting **every string scalar** into a set:

```go
// collectReferencedNames walks generated RDS/LDS and returns every string scalar
// appearing anywhere in them, including inside typed_config extensions.
func collectReferencedNames(routes, listeners envoycache.Resources) map[string]struct{}
```

A cluster is emitted if and only if its name is a member. Because the filter only ever iterates clusters the translator actually produced, the intersection with "real cluster names" is implicit and no separate candidate set is needed.

`pkg/kgateway/proxy_syncer/xdswrapper.go` already contains the walker shape to follow: `visitFields` / `visitMessage` recurse through messages, lists, and maps and unmarshal nested `Any` payloads for redaction. The reference collector is the same traversal with collection instead of mutation.

#### Why an over-approximating collector rather than a typed allowlist

This is the load-bearing correctness decision, and it is the opposite of what the reverted code did.

The reverted `collectProtoClusterReferences` recursed generically but **extracted** cluster names from exactly two message types, `envoyroutev3.RouteAction` and `envoytcpv3.TcpProxy`, via their `Cluster` and `WeightedClusters` specifiers. That was correct for its purpose: it fed a route-reachability readiness gate, and its own comment explains that gating on ancillary references would starve a gateway forever on a plugin bug. It is **not** correct for emission.

Backend clusters are referenced by name from inside listener `typed_config`, nowhere near a `RouteAction`:

- JWKS: `pkg/kgateway/extensions2/plugins/trafficpolicy/jwt.go:308`
- OAuth2 token endpoint and JWKS: `trafficpolicy/oauth2.go:247,399`
- ext_authz gRPC and HTTP services: `trafficpolicy/gateway_extension.go:304,355`
- Access-log gRPC sinks: `pkg/kgateway/extensions2/plugins/listenerpolicy/common.go:22`

All of these call `BackendObjectIR.ClusterName()` (`pkg/pluginsdk/ir/backend.go:296`). They name a **per-client backend cluster**, the exact population this EP filters.

An existing golden fixture proves the failure mode. In `pkg/kgateway/translator/gateway/testutils/outputs/jwt/remote-jwks-async.yaml`, `kube_default_remote-jwks_8080` is an EDS backend cluster whose only reference is:

```text
Listener -> filter_chain -> HttpConnectionManager -> http_filters[jwt/default/jwt-ext]
  -> ExtensionWithMatcher -> ExecuteFilterAction -> JwtAuthentication
  -> providers[jwt-ext_default_async-provider].remoteJwks.httpUri.cluster
```

That is a `HttpUri.cluster` field nested several `Any` payloads deep. A `RouteAction` or `TcpProxy` extractor does not see it, so a typed-allowlist filter drops the cluster and JWT validation fails permanently, with no route-level symptom pointing at the cause.

The risk is asymmetric, and that asymmetry decides the design:

- **Under-collection** drops a cluster Envoy needs, giving a permanent `503 NC` or a silently broken filter.
- **Over-collection** emits one cluster that nothing routes to, which is exactly today's behavior for that one cluster. Harmless.

So the collector must be biased to over-collect. Collecting every string scalar achieves that: any reference to a cluster is a string, so no reference shape, present or future, can escape it. The cost is that a header value or regex that happens to equal a cluster name emits a spurious cluster. Given kgateway's generated names (`buildClusterName`, producing forms such as `kube_default_httpbin_8080`), collisions are vanishingly unlikely and benign when they occur.

The traversal scaffolding from `714190cf85` (`collectResourceClusterReferences`, `collectNestedProtoClusterReferences`, `collectProtoClusterReferencesFromValue`, including its `Any` unmarshalling and its debug-log path for type URLs not linked into the binary) should be resurrected as-is. Only the extraction step changes, from a two-type switch to "record every string".

#### Guard against unresolvable references

A cluster selected at request time cannot be predicted, so filtering must switch off rather than guess. If the walk encounters `RouteAction.cluster_header` or a `ClusterSpecifierPlugin`, the gateway falls back to emit-all, increments `kgateway_xds_cluster_filter_disabled`, and logs once. kgateway generates neither today (verified across `pkg/` and `internal/`), so this is future-proofing against a plugin that adds one, and it fails safe rather than silently.

#### Cost and placement of the walk

The walk is O(config size) and runs **once per gateway per translation**, stored on `GatewayXdsResources`, not per client. This is the same reasoning the reverted code carried in its own comment: `Any` unmarshalling on large LDS and RDS is not free, and all clients of a role share the result. `GatewayXdsResources` (`proxy_syncer.go:72`) gains:

```go
ReferencedNames     map[string]struct{} // +noKrtEquals (covered by the hash)
ReferencedNamesHash uint64
```

with `ReferencedNamesHash` added to `GatewayXdsResources.Equals` (`proxy_syncer.go:95`) so the set participates in change detection in its own right, rather than relying on it to co-vary with `Routes.Version`.

#### Emission filter placement

The filter is applied in the `xdsSnapshotsForUcc` transform of `snapshotPerClient` (`perclient.go`), where the gateway's routes and the client's clusters are **already both in hand**. It is not applied in the upstream `clusterSnapshot` transform.

This is a correctness requirement, not a style preference. `clusterSnapshot` keys on `ucc` and does not depend on `mostXdsSnapshots`; adding that dependency in order to filter there would let a cluster row be filtered against one generation of the referenced set while the routes published alongside it come from another. That is precisely the cross-collection skew documented by the `// HACK` comment at `perclient.go:133`. Filtering where both are present makes the invariant

> no published route names a backend cluster that this snapshot filtered out

true **by construction**, from a single `listenerRouteSnapshot`, with no timing assumption.

Mechanics:

- Iterate the fetched per-client clusters, emit those whose name is in `ReferencedNames`, and record the names skipped as `droppedClusterNames`.
- Fold `ReferencedNamesHash` into the cluster resources version, alongside the existing `clustersHash` XOR `ClustersHash` computation, so the CDS version moves when the emitted set changes.
- `listenerRouteSnapshot.Clusters` (`ExtraClusters`, contributed by plugins through `ResourcesToAdd()` in `pkg/kgateway/translator/irtranslator/gateway.go`) continues to be merged **unconditionally**. Those are self-contained ancillary clusters the config asked for, they are not part of the Service inventory that bloats `config_dump`, and excluding them from the filter removes an entire class of completeness risk.
- Blackhole semantics are unchanged. Errored clusters are already skipped at assembly, `BlackholeClusterName` is never a translated backend cluster, and routes that name it continue to fail visibly as intended. It is never a filter candidate, so it needs no special case.
- Cluster-name identity is compared on the name the translator produced. Names can be gateway-scoped (`gatewayBackendClientCertificateExtraKey` folds a per-gateway suffix into `extraKey`), which is consistent here because the referenced set is per-gateway and is compared only against that gateway's own translated clusters.

#### EDS alignment

EDS must shrink with CDS, and the rule must be stated as **drop the CLAs whose clusters this snapshot dropped**, never as "drop CLAs for clusters not in CDS":

```go
func filterEndpointResourcesForDroppedClusters(
    endpoints envoycache.Resources, dropped map[string]struct{},
) envoycache.Resources
```

The distinction matters because not every legitimate CLA has a CDS entry in the snapshot. Since `bc57f656fc`, the local-cluster CLA (`local_cluster.go`) is an EDS-only resource for a cluster defined in the Envoy **bootstrap**, offered only to clients that asked for it. A "not in CDS" rule deletes it and breaks whatever consumes it. Keying on the exact set of names this snapshot filtered out cannot touch any resource the filter did not cause, which makes the rule safe for local-cluster CLAs, bootstrap clusters, and any future EDS-only resource.

This composes with the existing `filterEndpointResourcesForStaticClusters` (`perclient.go:273`) rather than replacing it. The two filters are independent and both run.

#### Delivery ordering

A coherent snapshot does not make Envoy *apply* CDS before RDS. The in-tree probes in `pkg/kgateway/setup/ads_delivery_order_test.go` pin the mechanics against the real go-control-plane server, each scenario run in both server modes, and all three pass:

- `TestADSQuietStreamAdditionDeliversClusterBeforeRoute`. On an otherwise idle stream, the `ads=true` `SnapshotCache` type-sorts its pushes (`go-control-plane/pkg/cache/v3/order.go`) and CDS reaches the wire before RDS even without ordered ADS. The non-ordered server's `reflect.Select` drain randomizes only when several per-type channels are ready simultaneously, which happens when a response is stalled in gRPC flow control or when snapshots arrive back to back. `WithOrderedADS()` closes that residual window.
- `TestADSAckSkewDeliversRouteBeforeClusterEvenWhenOrdered`. **ACK skew defeats both modes.** After a CDS response is sent, its watch stays closed until Envoy ACKs. If the next snapshot, carrying a new cluster plus a route retarget, lands in that window, the only open watch is RDS, so the route reaches the wire before any CDS carrying its destination. SotW can only answer open watches, and no server option closes this. The window is reachable exactly when a route is retargeted while an earlier CDS-only update is un-ACKed, which is routine under churn.
- `TestADSCombinedRemovalDeliversClusterRemovalBeforeRouteDereferenceWhenOrdered`. The delivered type order is CDS before RDS, which is the **wrong** order for removals: the cluster would go away before the route stops using it.

Emit-all is immune to all three, because destinations were delivered and ACKed long before any route named them. Referenced-only removes that immunity, so it has to be rebuilt deliberately. `EnableOrderedAds` is useful busy-stream hardening and is orthogonal; it is not an addition-side fix, as its own doc comment states.

#### Publication coordinator

One small per-client component provides both transition guarantees. It is specified here rather than borrowed, because `publishGate` is not on `main`.

State per client key, under one mutex:

- `lastPublished`, the snapshot most recently written to the cache.
- `staged`, a held build awaiting reference-ahead release, plus its deadline.
- `dereferencedAt map[string]time.Time`, when each cluster left the referenced set.
- A `krt.RecomputeTrigger` to re-enter the transform when a timer fires. Precedents: `pkg/krtcollections/uniqueclients.go:180` and `pkg/kgateway/extensions2/plugins/backend/ec2.go:438`.

All cache mutations go through the coordinator, so a timer-driven publish can never interleave with a build-driven one.

Addition, reference-ahead. When a build both enlarges the emitted cluster set **and** changes RDS or LDS to name a newly-emitted cluster:

1. Publish the new CDS and EDS with the **previously published** RDS, LDS, and Secrets. The destination is now in Envoy, and no route names it yet.
2. Arm release with deadline `now + ClusterReferenceAhead`.
3. On release, publish the complete build. The retarget cannot `503 NC`, in either server mode, regardless of ACK skew.

This recreates, for exactly the transitional cluster, the property emit-all provided globally.

Removal, de-reference grace. When a cluster leaves the referenced set, record `dereferencedAt[name]`. The emitted set becomes referenced-now plus recently-de-referenced within the grace window. Re-referencing clears the entry, so a flapping route keeps its cluster present instead of oscillating. When the window elapses, the coordinator re-publishes without the cluster, under its own lock. This is the removal-side fix independent of delivery ordering, because the delivered order is structurally wrong for removals.

Release signals: observe to accelerate, window to bound. Both windows may be **shortened** by observation and must never be **gated** on it.

- The snapshot cache is keyed per unique client identity and multiple proxy replicas share a key, while ACKs arrive per stream. Releasing on the first stream's ACK reintroduces the race for the others. Releasing only on the slowest lets one wedged or NACKing replica freeze route retargets for every replica of the gateway, an unbounded withhold, which is the failure family #14184 exists to remove. A NACK counts as "will not ACK" and falls back to the window; `envoy_xds_rejects_total` makes that visible.
- **The accelerator signal should be the EDS subscription for the new cluster, not the CDS ACK.** A CDS ACK proves the response was accepted. It does not prove the cluster is routable, because an EDS cluster stays in warming until its `ClusterLoadAssignment` arrives, and a route to a warming cluster still fails. Envoy requests EDS for a cluster only *after* applying its CDS entry, so seeing a `DiscoveryRequest` name that cluster is strictly stronger evidence, and its subsequent ACK is stronger still. The observation point already exists: `logNackCallback.OnStreamRequest` (`pkg/kgateway/setup/envoy_error.go:84`) receives every request, with resource names on it.
- For non-EDS clusters (STATIC and similar) no EDS request is coming, so the CDS ACK is the best available signal and the window remains the bound.

In all cases the deadline fires unconditionally, so a hold is bounded by construction.

#### Worked example

```mermaid
sequenceDiagram
    participant R as Route /foo
    participant T as Assembly plus coordinator
    participant E as Envoy (ADS)
    Note over R: /foo: service-a -> service-b
    R->>T: routes now name b, not a
    T->>T: ReferencedNames gains b; dereferencedAt[a] = now
    T->>T: emitted = referenced plus graced = {..., a, b}
    T->>E: snapshot 1 {CDS: a,b ; EDS: a,b ; RDS: /foo->a}
    E->>T: EDS request naming b, then ACK (b applied and warmed)
    Note over T: observed early, or ClusterReferenceAhead elapsed
    T->>E: snapshot 2 {CDS: a,b ; RDS: /foo->b}
    Note over E: b is already applied and warmed, so the retarget cannot 503 NC\nin either server mode, regardless of ACK skew.\na is still present, so in-flight traffic to a is safe
    Note over T: de-reference grace elapses for a
    T->>E: snapshot 3 {CDS: b ; EDS: b}
    E->>E: a is removed only after no applied route uses it
```

### Reporting

- Status computation must stay independent of the emission filter. A backend with an attached policy but no route reference still needs its status reconciled. Emission decides what Envoy is told, not what kgateway reports.
- New metrics: `kgateway_xds_emitted_clusters` (gauge, per gateway, alongside the existing `snapshotResources` gauges in `perclient.go`); `kgateway_xds_reference_ahead_holds_total` and `kgateway_xds_reference_ahead_releases_total{reason="observed"|"window"}`; `kgateway_xds_dereference_grace_active` and `kgateway_xds_dereference_pruned_total`; `kgateway_xds_cluster_filter_disabled` for the unresolvable-reference fallback.
- Existing `envoy_xds_rejects_total` and `envoy_xds_rejects_active` remain the NACK signal. A NACK during a hold surfaces as a `reason="window"` release.

### Test Plan

The completeness risk must become an enforced invariant, because a review-time checklist cannot survive the next filter type that names a cluster.

- **Golden-corpus completeness property test, the key guard.** For every fixture under `pkg/kgateway/translator/gateway/testutils/outputs/`, derive the set of cluster names mentioned anywhere in `Listeners` and `Routes` **independently** of the production collector (the golden files store `typed_config` decoded, so a plain tree walk over the YAML reaches every reference), then assert the production `collectReferencedNames` result is a superset. Two independent derivations cross-checking each other over the whole corpus, so any new cluster-referencing filter shape fails CI the moment its fixture lands.

  The method is already validated. Sweeping all 401 golden outputs, 925 backend clusters resolve to 425 referenced and 500 unreferenced with **zero false drops**. Every drop inspected was genuinely unreferenced, including two that looked like bugs and were not: `jwt/cross-namespace.yaml` has two JWKS Services and only the `kube_remote_` one is referenced, and `delegation/traffic_policy_filter_override_merge.yaml` drops `extproc2` because the merge leaves only `extproc1` referenced. The 54% drop ratio is not a real-world estimate, since fixtures over-represent unreferenced backends; the zero-false-drops result is the meaningful part.
- **Unit, collector.** Route targets, weighted clusters, TCP and TLS targets, mirror backends, and each ancillary shape: `http_uri.cluster` for JWKS, `grpc_service.envoy_grpc.cluster_name` for ext_authz, ext_proc, rate limit, and access log, plus the OAuth2 token endpoint. `jwt/remote-jwks-async.yaml` is the regression pin for the typed-allowlist bug.
- **Unit, collector guard.** A synthetic `cluster_header` route and a `ClusterSpecifierPlugin` route each disable filtering for their gateway and bump the metric.
- **Unit, filter.** `Referenced` skips unreferenced backends; `All` is byte-identical to today; `ExtraClusters` always survive; a graced cluster survives; a local-cluster CLA survives, which is a direct regression pin for the EDS rule.
- **Unit, coordinator.** Retarget `a -> b` yields snapshot 1 with CDS `{a,b}` and RDS still naming `a`, snapshot 2 with RDS naming `b`, then a later snapshot with `a` pruned. A flapping route does not oscillate. Holds release on the window when no observation arrives, release early on EDS subscribe plus ACK, and a NACKing replica cannot extend a hold past its deadline.
- **Wire order.** Extend `pkg/kgateway/setup/ads_delivery_order_test.go`, whose harness already exists and passes, to drive the emission path through the ACK-skew and combined-removal scenarios, asserting that a retarget is never delivered before its cluster and a cluster removal never before its de-referencing RDS.
- **Referenced-but-empty.** A referenced backend that is legitimately empty (scale-to-zero, ExternalName) publishes as truth: never dropped, never defers publication, never holds a retarget.
- **Integration, envtest plus ADS.** Route destination changes produce no `503 NC` and no endpoint gap in `Referenced` mode, across HTTP, GRPC, TCP, and TLS, including delegated routes.
- **e2e and load.** The #13586 shape, hundreds of Services with a handful routed, yields cluster and metric counts proportional to referenced Services, with stable traffic through route churn.
- **Regression.** `All` mode byte-identical to today, asserted over the golden corpus.

### Implementation phases

Each phase is independently reviewable and revertible. Nothing user-visible ships before it is drop-free.

#### Phase 1: collector, dark

- Resurrect the traversal scaffolding from `714190cf85`, replace the two-type extraction with all-string collection, and add the unresolvable-reference guard.
- Store `ReferencedNames` and `ReferencedNamesHash` on `GatewayXdsResources` in `toResources` (`proxy_syncer.go:122`), and add the hash to `Equals`.
- Land the golden-corpus completeness property test and the collector unit tests.
- Nothing consumes the set yet, so there is no behavior change.

#### Phase 2: filter and EDS alignment, dark

- Apply the filter in `xdsSnapshotsForUcc`, and add `filterEndpointResourcesForDroppedClusters`.
- Add `ClusterDiscoveryMode` but **do not document `Referenced` as usable yet**. Without Phase 3 it is not drop-free, and shipping a knowingly-blippy mode is exactly the shortcut this EP exists to avoid.
- Tests: filter unit tests, the local-cluster CLA pin, and the `All`-mode byte-identical regression.

#### Phase 3: publication coordinator, mode becomes usable

- Add the coordinator (per-client state, mutex, `RecomputeTrigger`, deadlines) with reference-ahead and de-reference grace, plus `ClusterReferenceAhead` and `ClusterDereferenceGrace`.
- Add EDS-subscription and ACK observation as accelerators through `OnStreamRequest`, with the window as the unconditional bound.
- Tests: coordinator unit tests, the extended wire-order probes, and the integration churn matrix.
- Document `Referenced` as supported and opt-in.

#### Phase 4: observability, docs, default

- Add the metrics above so operators can see the reduction and the grace activity.
- Document the make-before-break guarantee, the two windows, and the behavior change: unreferenced Services no longer appear as clusters.
- Keep `All` as the default. Consider flipping only after a soak with the load matrix green.

## Alternatives

### Option A: monotone sticky emission, never remove

Emit referenced-now plus ever-referenced-since-process-start, and never remove a cluster once emitted. A cluster that has never been sent cannot be in use, so declining to add it is safe without any removal machinery: no de-reference grace, no prune timers, no removal-side observation, and no prune-versus-carry-forward question.

- Pros: deletes the entire removal half of this EP, ships far sooner, and degrades gracefully to today's behavior in the worst case.
- Cons: it does not fully solve #13586. The emitted set drifts monotonically toward emit-all under route churn, and clusters for deleted routes persist until the process restarts, so `config_dump` and `/stats` re-bloat over time, which is the actual complaint. It also still needs reference-ahead for additions, so it avoids only half the machinery. And it makes process restart semantically load-bearing, which is a bad property to design in.

Recorded because it is a genuinely attractive shortcut and was considered seriously. Rejected here because the ask is a lean steady state, not a lean cold start.

### Option B: Service label selector, complementary

Extend discovery scoping with a Service label selector, a sibling to `discoveryNamespaceSelectors`, wired into the kube backend plugin (`pkg/kgateway/extensions2/plugins/kubernetes/k8s.go`) so unmatched Services never become a backend and therefore never a cluster. This is the ask from the reporter with the 93k-metric environment.

- Pros: small; no coherence dependency at all, since the operator controls the set explicitly and there is no transition to make drop-free; finer than namespace scoping; ships independently of everything here.
- Cons: the operator must label workloads and keep labels current, and it is opt-in scoping rather than automatic.

Not mutually exclusive with this EP. Option B is the quick standalone win, referenced-only is the automatic model. Shipping Option B first is reasonable, and is an open question below.

### Option C: explicit Backend kube type plus disable-discovery

Add a kube-type `Backend` resource so users declare which Services become clusters, plus a setting to disable auto-discovery entirely (maintainer suggestion in the issue thread). Maximal control, largest UX change.

### Option D: generalized NACK-defensive staged publication, rejected

A broader version of this EP's staged publication was weighed and rejected: split *any* snapshot whose partial rejection could yield an incoherent applied state into prerequisite-first snapshots, advancing each step on acknowledgment.

- **Per-key cache versus per-stream ACK.** Replicas share cache keys, so ACK-gated advancement is either per-replica keys, undoing the identity collapse that controls per-client cost, or slowest-replica gating, where one wedged pod freezes updates for every replica of its gateway.
- **Ladders under churn.** Staged publication makes every build a multi-step state machine that must be reconciled against newer builds arriving mid-ladder. Under the churn rates that motivated #14184, ladders are routinely superseded before completing, and the supersession logic is the same readiness-gated shape `714190cf85` deleted.
- **Marginal benefit inverts the stale-versus-truth decision.** A partial NACK requires emitting a rejected proto, a translation bug class that strict validation pre-empts and `envoy_xds_rejects_total` makes loudly observable. During such a bug Envoy already holds last-accepted config per type; staging would keep serving stale targets while the bug is live, the opposite of fail-visible.

This EP's two windows are the deliberately-bounded slice: they stage publication only where transitions are *routine operation* rather than a bug class, they remain window-bounded with observation as an accelerator only, and they add no ladders.

### Option E: build on the #14184 engine and #14343, rejected as a prerequisite

The earlier draft made itself depend on #14343, #14257, and the #14184 engine PR landing first, and described their symbols as already implemented. They are not on `main`, and #13868 and #13958, which supplied the referenced-set walk, were reverted outright. Making a user-visible fix wait on three unmerged PRs, one of whose predecessors was already reverted, is unnecessary: everything this EP needs is either merged or small enough to specify here.

They still compose. If #14343's shared-base plus per-client-overlay translation lands, the same filter can additionally skip per-client materialization for unreferenced backends, shrinking control-plane memory as well as proxy memory. If the #14184 engine's `publishGate` lands, this EP's coordinator should be folded into it rather than kept alongside, since they are the same primitive with different release conditions.

### Status quo mitigations

`statsMatcher` and `discoveryNamespaceSelectors`, already shown insufficient for shared-namespace topologies and capped expression lists.

## Risks and trade-offs

- **Referenced-set completeness is the principal correctness risk**, which is why the collector over-approximates instead of allowlisting. The reverted two-type extractor would have dropped the JWKS cluster in an existing fixture. Mitigated structurally by collecting every string, by exclusion since `ExtraClusters` are never filtered, by the guard that disables filtering on unresolvable selectors, and by CI through the golden-corpus property test.
- **Delivery ordering is the sharpest behavioral risk.** Coherent snapshots do not imply application order, and ACK skew delivers a retarget before its cluster in **both** server modes, as the in-tree probes pin. Mitigated by reference-ahead. `EnableOrderedAds` is hardening, not a fix.
- **Removal correctness depends on the grace window**, which must exceed worst-case RDS propagation. Configurable for that reason.
- **Referenced-but-empty backends must remain publishable truth.** Scale-to-zero and ExternalName backends are referenced and legitimately empty, so they must never be dropped, defer publication, or hold a retarget. Pinned by test.
- **Two more windows to tune.** Defaults that are too short reintroduce blips, too long delays route edits and retains clusters. Both are bounded and observable.
- **Behavior change.** Unreferenced Services disappear from `config_dump` and `/stats`, so dashboards or scripts relying on their presence will notice. Intended, hence opt-in with a default of `All`.
- **Control-plane memory is unchanged** by this EP. Only proxy-side config, memory, and stats shrink. See Non-Goals and Option E.
- **Restart re-arms the addition path.** After a controller restart every reference is new again, so the first retarget onto each backend takes the reference-ahead path. `KGW_XDS_FIRST_CONNECT_DELAY` covers the initial convergence window and the coordinator covers the rest.

## Open Questions

- **Default durations for the two windows**, and whether to derive them from observed per-type ACK latency, which the server callbacks expose, rather than from static values. Large fleets with batched route churn are where measured windows would matter most.
- **Whether to ship Option B, the Service label selector, first** as an immediate mitigation for the reporter while this lands.
- **Whether reference-ahead should also cover first-ever emission on a cold start**, or whether `KGW_XDS_FIRST_CONNECT_DELAY` plus the coordinator already suffice.
- **Whether the EDS-subscription accelerator is worth its bookkeeping** in v1, or whether window-only release should ship first with observation as a follow-up.
- **Whether to fold the coordinator into `publishGate`** if the #14184 engine lands during implementation, versus shipping both and merging them later.
