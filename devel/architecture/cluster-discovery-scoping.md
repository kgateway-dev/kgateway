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
to an existing backend safe.

For `/foo: service-a -> service-b`, the route update (RDS) and the cluster
update (CDS) reach Envoy as separate events. If `/foo` starts naming `service-b`
before `service-b`'s cluster exists, `/foo` returns `503 NC` for the gap.
Pre-creating a cluster for every Service means a retarget's destination is
normally already there; only a backend created together with the route that
names it races. Emit-all is a make-before-break workaround for not having safe
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

A backend can also ship listeners with its cluster. A CONNECT tunnel's listener,
for one, proxies through the proxy backend's cluster, which no route names. The
listeners of each kept backend are walked the same way, and the clusters they
name are kept and followed in turn. A dropped backend's listeners keep nothing.
A request-time selector in one of them turns scoping off for that client, as one
in the gateway's own configuration does.

A golden-corpus sweep enforces this: for every translator fixture, every cluster
the configuration mentions must be in the emitted set, checked against an oracle
that walks the fixture YAML rather than production code.

## Transitions

**Adding a destination.** Publishing the new cluster and the route that names it
in one coherent snapshot is not enough, because Envoy does not necessarily apply
CDS before RDS within a snapshot, and after a CDS response is sent its watch
stays closed until Envoy ACKs — so a route update landing in that window reaches
the wire on the still-open RDS watch, ahead of its destination.
`KGW_CLUSTER_REFERENCE_AHEAD` holds routes, listeners and secrets at their
published versions while the new cluster goes out, then releases.

The window is per cluster. It starts when the controller first publishes a
snapshot carrying the cluster, just before installing it in its xDS cache.
Installation, the gRPC send, Envoy's ACK, and Envoy applying and warming the
cluster are separate later events, and the controller observes none of them. A
cluster counts as new when the published snapshot lacks it, whatever Envoy has,
so a rebuild inside the window (the new backend's own endpoints arriving, say)
keeps holding. If the CDS send waits behind an un-ACKed earlier response, or
the cluster warms slowly, the window can close before the cluster is active,
and the route can still briefly return 503 NC. Every cluster new to the proxy
counts, not only route targets: a listener that starts calling a new
ext_authz, ext_proc, rate limit or JWKS cluster waits for it too. Builds that
are deferred for an unrelated reason are held the same way. While held, the
published routes and listeners are what the client is being served, so every
cluster they name stays published with them, whatever the de-reference grace.

**Removing a destination.** With ordered ADS, the default, a snapshot's CDS
response is sent before its RDS response, and without it CDS can still arrive
first. Dropping a cluster in the same snapshot as the route that stopped naming
it then applies the removal first, and requests on the old route get a 503 NC
until the route update lands. `KGW_CLUSTER_DEREFERENCE_GRACE` keeps the cluster
published for a bounded period after the controller publishes the route update
that stops naming it, giving that update time to land first; it cannot make sure
the update does. A build held for a new or unready cluster does not start the
period: the hold keeps the cluster anyway, and the period starts at the release.
Set this above worst-case RDS propagation for your fleet.

Ordered ADS does not substitute for either window: it fixes CDS before RDS,
which is the wrong order for removals, and it does not close the ACK-skew
window for additions.

Both windows are elapsed time from publication, not acknowledgements: an update
Envoy rejects, or one parked behind a slow stream, can outlast them, so neither
makes a transition hitless. Size them above the fleet's worst-case propagation.

## When scoping turns itself off

A route can pick its destination at request time — from a header (on the route,
on a weighted-cluster entry, or on a mirror policy), or from a cluster specifier
plugin whose script composes a name — out of a set the configuration never
enumerates. A TcpProxy's cluster can be overridden through filter state (the
`sni_cluster` filter, or `set_filter_state` writing `envoy.tcp_proxy.cluster`).
No walk over the generated protos can find those candidates, nor the clusters
inside a `typed_config` the controller cannot decode, nor behind a
`cluster_specifier` kind it does not know.

The same holds for a kept backend's own listeners: a selector in one of them
reverts the client to every cluster too. A dropped backend's listeners are
never walked, so one there changes nothing.

Such a gateway reverts to emitting every cluster, a warning is logged when it
does, naming the selectors and, for a backend's listener, the backend and
listener, and `kgateway_xds_cluster_scoping_disabled_gateways` goes to 1 for it. Detection
matters more than it looks: a plugin like that usually falls back when its
computed cluster is absent, so pruning a candidate does not produce a visible
503 — every affected request quietly goes somewhere else instead. The metric is
what turns that into something you can alert on.

## Settings

| Setting | Default | Effect |
| --- | --- | --- |
| `KGW_CLUSTER_DISCOVERY_MODE` | `ALL` | `REFERENCED` scopes CDS/EDS to referenced clusters. `ALL` is the pre-existing behavior. |
| `KGW_CLUSTER_DEREFERENCE_GRACE` | `5s` | How long a de-referenced cluster stays published. `0` or negative removes immediately and accepts the race. |
| `KGW_CLUSTER_REFERENCE_AHEAD` | `2s` | How long configuration naming a cluster new to a proxy is held after the controller first publishes that cluster. `0` or negative publishes both together and accepts the blip. |

Both windows are off in `ALL` mode, where every discovered backend is emitted,
so a cluster enters or leaves CDS only with its backend, never because a route
started or stopped naming it. With `ALL` set, nothing in this feature runs: the
emitted set is not computed, no filtering happens, and the publish path is the
one that existed before the feature.

When `KGW_PER_CLIENT_PUBLISH_BUDGET` is positive, a hold also ends one budget
after it began, so neither a misconfigured window nor a stream of new clusters
can pin a client's routing updates. With the budget at 0 or negative, a hold
still ends when its windows close, but new clusters that keep arriving can
extend it, and a hold that also waits for an unready cluster waits for it
indefinitely. A hold the budget cuts short is counted as
`kgateway_xds_snapshot_perclient_bounded_publishes_total{mode="reference_ahead"}`;
one whose windows close first is not a bounded publish.

## Metrics

| Metric | Type | Meaning |
| --- | --- | --- |
| `kgateway_xds_cluster_scoping_dropped_clusters` | gauge | The most clusters any connected proxy group of a gateway has withheld because nothing references them. |
| `kgateway_xds_cluster_scoping_disabled_gateways` | gauge | 1 while any connected proxy group of a gateway reverted to emit-all. Worth an alert. |
| `kgateway_xds_cluster_scoping_transitions_total` | counter | Clusters whose emitted-set change needed a window, by `transition`; one per cluster per window it enters, for each proxy group. |

A proxy group is the proxies of a gateway that share labels and locality; they
receive one snapshot, and each group keeps its own windows. A gateway whose
proxies span three zones therefore counts every transition three times, while
any number of replicas in one zone count once.

How many clusters a gateway's proxies do receive is
`kgateway_xds_snapshot_resources{resource="Cluster"}`, which counts the published
snapshot after scoping.

`disabled_gateways` is kept per proxy group and reported for the gateway as
whether any group is unscoped: groups can differ, since a backend can fail
translation for one group's proxies alone, and a healthy group must not clear
another's fallback. `dropped_clusters` is kept per group the same way and
reported as the most any connected group drops, so a group that reverted to
emit-all does not hide what the others save. The warning fires when the
gateway turns unscoped, not on every rebuild. A departing group's state goes
with it, and both gauges are
removed when a gateway's last connected proxy goes away, so a deleted gateway
does not keep a `disabled_gateways` alert firing.

`dropped_clusters` at zero with scoping on means no connected proxy group
drops anything: the deployment routes to everything it discovers and has
nothing to gain here. A steadily rising
`transitions_total{transition="reference_ahead_held"}` means route edits keep
landing on clusters missing from the proxy's published snapshot, each paying
the hold once,
which is a reason to shorten the window rather than a fault.

## What changes for users

Unreferenced Services stop appearing as clusters in `config_dump` and stop
producing `/stats` series. That is the point, and it is also a visible behavior
change for anyone whose dashboards assume those clusters exist — which is why
the feature is opt-in.
