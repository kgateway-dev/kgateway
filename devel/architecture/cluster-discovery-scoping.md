# Scoping cluster discovery to what routes reference

By default kgateway sends every proxy a CDS cluster, and an EDS
`ClusterLoadAssignment`, for **every** backend in its discovery scope, whether or
not any route targets it. A cluster with 300 Services and 16 routed ones puts all
300 in every proxy's `config_dump`, in every proxy replica's memory, and in every
proxy's `/stats` — where each cluster contributes a large family of series
carrying an `envoy_cluster_name` label.

`KGW_CLUSTER_DISCOVERY_MODE=REFERENCED` scopes CDS and EDS to the clusters the
generated configuration actually references. It is off by default.

## Why the default is emit-all

This is not an oversight. Emitting everything is what makes a route retarget
safe.

For `/foo: service-a -> service-b`, the route update (RDS) and the cluster
update (CDS) reach Envoy as separate events. If `/foo` starts naming `service-b`
before `service-b`'s cluster exists, `/foo` returns `503 NC` for the gap.
Pre-creating a cluster for every Service guarantees the destination is always
already there. Emit-all is a make-before-break workaround for not having safe
cluster and route transitions.

Scoping therefore has to supply those transitions, in both directions.

## What counts as referenced

The emitted set is derived from the **generated Envoy configuration**, not from
route `backendRefs`. That is correct by construction for every destination
translation actually chose, and it picks up delegation, non-HTTP routes and
mirrors for free, because they are already in the output.

It deliberately over-approximates. The collector takes every string of every
message it reaches, including inside `typed_config`, rather than reading named
fields of message types it knows about. Emitting one cluster nothing uses costs
a little memory; failing to emit one Envoy needs is a permanent outage with no
route-level symptom. Ancillary clusters — ext_authz and ext_proc servers, rate
limit services, access-log gRPC sinks, JWKS sources — are real clusters reached
only from filter configuration, and they are exactly the population a narrower
collector would drop.

A golden-corpus sweep enforces this: for every translator fixture, every cluster
the configuration mentions must be in the emitted set, checked against an oracle
that walks the fixture YAML rather than production code.

## Transitions

**Adding a destination.** Publishing the new cluster and the route that names it
in one coherent snapshot is not enough, because Envoy does not necessarily apply
CDS before RDS within a snapshot, and after a CDS response is sent its watch
stays closed until Envoy ACKs — so a route update landing in that window reaches
the wire on the still-open RDS watch, ahead of its destination.
`KGW_CLUSTER_REFERENCE_AHEAD` holds routes, listeners and secrets at the
versions the client already has while the new cluster goes out, then releases.

**Removing a destination.** Envoy is delivered CDS before RDS, so dropping a
cluster in the same snapshot as the route that stopped naming it sends the
removal first. `KGW_CLUSTER_DEREFERENCE_GRACE` keeps the cluster published for a
bounded period after its last reference goes away, so the route update always
lands first. Set this above worst-case RDS propagation for your fleet.

Ordered ADS does not substitute for either window: it fixes CDS before RDS,
which is the wrong order for removals, and it does not close the ACK-skew
window for additions.

## When scoping turns itself off

A route can pick its destination at request time — from a header, or from a
cluster specifier plugin whose script composes a name — out of a set the
configuration never enumerates. No walk over the generated protos can find those
candidates.

Such a gateway reverts to emitting every cluster, and
`kgateway_xds_cluster_scoping_disabled_gateways` goes to 1 for it. Detection
matters more than it looks: a plugin like that usually falls back when its
computed cluster is absent, so pruning a candidate does not produce a visible
503 — every affected request quietly goes somewhere else instead. The metric is
what turns that into something you can alert on.

### Claiming, so one plugin does not cost the whole gateway

A plugin can declare what it may route to, through `ClusterEmissionClaim` in
`pkg/pluginsdk`. A gateway whose every request-time selector is claimed keeps
scoping for every other backend, instead of reverting wholesale.

A claim says three things: which selectors it accounts for (matched against the
extension name the plugin puts in `cluster_specifier_plugin`, in either its
referenced or inline form), which exact cluster names to keep, and which name
prefixes to keep for candidates that cannot be enumerated ahead of time.

Two rules are worth knowing. An **empty prefix is rejected**: it would re-admit
the entire inventory while still reporting the gateway as scoped, which is worse
than reverting because it is invisible. And a **cluster-header selector can
never be claimed** — the destination is whatever the client sends, so nothing a
plugin declares can bound it, and such a gateway stays unscoped.

A claim should also cover the plugin's own resources. A plugin of this shape
commonly emits an endpointless placeholder cluster so an unmatched request fails
closed; once no route names that placeholder, scoping would otherwise prune the
very cluster that makes the feature safe.

## Settings

| Setting | Default | Effect |
| --- | --- | --- |
| `KGW_CLUSTER_DISCOVERY_MODE` | `ALL` | `REFERENCED` scopes CDS/EDS to referenced clusters. `ALL` is the pre-existing behavior. |
| `KGW_CLUSTER_DEREFERENCE_GRACE` | `5s` | How long a de-referenced cluster stays published. `0` removes immediately and accepts the race. |
| `KGW_CLUSTER_REFERENCE_AHEAD` | `2s` | How long a route update onto a newly-emitted cluster is held. `0` publishes both together and accepts the blip. |

Both windows are ignored in `ALL` mode, where no cluster is ever de-referenced
and none is ever new to a client. With `ALL` set, nothing in this feature runs:
the emitted set is not computed, no filtering happens, and the publish path is
the one that existed before the feature.

The reference-ahead hold is additionally bounded by
`KGW_PER_CLIENT_PUBLISH_BUDGET`, so a misconfigured window cannot pin a client's
routing updates.

## Metrics

| Metric | Type | Meaning |
| --- | --- | --- |
| `kgateway_xds_cluster_scoping_emitted_clusters` | gauge | Backend clusters published to a client after scoping. |
| `kgateway_xds_cluster_scoping_dropped_clusters` | gauge | Clusters withheld because nothing references them. |
| `kgateway_xds_cluster_scoping_disabled_gateways` | gauge | 1 when a gateway reverted to emit-all. Worth an alert. |
| `kgateway_xds_cluster_scoping_transitions_total` | counter | Emitted-set changes that needed a window, by `transition`. |

`dropped_clusters` at zero with scoping on means the deployment routes to
everything it discovers and has nothing to gain here. A high ratio of
`transitions_total{transition="reference_ahead_held"}` to route updates means
most route edits are paying the hold, which is a reason to shorten the window
rather than a fault.

## What changes for users

Unreferenced Services stop appearing as clusters in `config_dump` and stop
producing `/stats` series. That is the point, and it is also a visible behavior
change for anyone whose dashboards assume those clusters exist — which is why
the feature is opt-in.
