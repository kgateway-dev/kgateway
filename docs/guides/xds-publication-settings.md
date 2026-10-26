# xDS publication and scoping settings

The kgateway controller builds an xDS snapshot for each connected client and
installs it in its xDS cache, from which the xDS server answers the proxy's
requests. A client is the group of a gateway's proxies that share one snapshot
key: the same labels and, when pod locality is tracked, the same locality.

The settings on this page control when the controller installs a client's
snapshot, which clusters it carries, and how the xDS server answers rejections
and reconnects.

The durations here time events in the controller. None of them observes the
gRPC send, Envoy's ACK, or Envoy applying and warming configuration, and adding
them up does not give a bound on proxy startup or on when a change takes effect.

## Setting a value

Each setting is an environment variable of the controller container. The
xDS scoping settings have their own Helm values under
`controller.xdsScoping`. Set the others through `controller.extraEnv`:

```yaml
controller:
  xdsScoping:
    mode: REFERENCED     # KGW_XDS_SCOPING_MODE
    referenceAhead: 3s   # KGW_CLUSTER_REFERENCE_AHEAD
    dereferenceGrace: 5s # KGW_CLUSTER_DEREFERENCE_GRACE
  extraEnv:
    KGW_PER_CLIENT_PUBLISH_BUDGET: "20s"
```

or on the command line:

```sh
helm upgrade <release> <chart> --namespace <namespace> --reuse-values \
  --set controller.xdsScoping.mode=REFERENCED
```

A `controller.xdsScoping` value left empty, as it is by default, sets
nothing, so the controller uses its own default. A variable set in
`controller.extraEnv` takes precedence over the matching
`controller.xdsScoping` value. For example, an explicit
`controller.extraEnv.KGW_XDS_SCOPING_MODE` overrides
`controller.xdsScoping.mode`.

- **When values are read.** The controller reads these variables once, at
  startup. `KGW_XDS_FIRST_CONNECT_DELAY` is read once too, when the first xDS
  stream needs it. Nothing is reloaded while the controller runs. Changing
  `controller.extraEnv` or `controller.xdsScoping` changes the
  controller's pod template, so a Helm upgrade rolls the controller and the new
  values apply to the new pods.
- **Accepted values.** Durations are Go duration strings, such as `500ms`, `15s`
  or `1m30s`. A bare number other than `0` is invalid. Booleans accept what Go's
  `strconv.ParseBool` does: `true` and `false`, `True`, `TRUE`, `t`, `1`, and so
  on.
- **Invalid values.** An invalid value stops the controller from starting: it
  logs `error loading settings from env` and exits. The exception is
  `KGW_XDS_FIRST_CONNECT_DELAY`, which logs a warning and uses its default. An
  invalid xDS scoping value, in `controller.xdsScoping` or in
  `controller.extraEnv` under `KGW_XDS_SCOPING_MODE`,
  `KGW_CLUSTER_REFERENCE_AHEAD` or `KGW_CLUSTER_DEREFERENCE_GRACE`, fails the
  Helm install or upgrade instead.
- **Negative durations.** Nothing rejects a negative duration. Every duration
  here treats a negative value exactly like `0`, so a stray minus sign silently
  turns that bound or window off.
- **Scope.** Each setting applies to every gateway the controller serves.
  Publication timers and windows run separately for each client.

| Variable | Default | Values | `0`, negative, or off |
| --- | --- | --- | --- |
| `KGW_PER_CLIENT_PUBLISH_BUDGET` | `15s` | duration | No first-publish or flip-release deadline. |
| `KGW_PER_CLIENT_WARM_WITHHOLD_BOUND` | `10m` | duration | Warm withholds and missing-cluster carries have no limit. |
| `KGW_XDS_SNAPSHOT_CONSISTENCY_CHECK` | `false` | boolean | `false`: no check. |
| `KGW_XDS_RESPOND_ON_RECONNECT` | `true` | boolean | `false`: held-version requests wait for a version change. |
| `KGW_XDS_SCOPING_MODE` | `ALL` | `ALL`, `REFERENCED`, any case | `ALL`: no scoping and no transition windows. |
| `KGW_CLUSTER_DEREFERENCE_GRACE` | `5s` | duration | A de-referenced cluster is removed at once. |
| `KGW_CLUSTER_REFERENCE_AHEAD` | `2s` | duration | A new cluster and the configuration naming it publish together. |
| `KGW_XDS_FIRST_CONNECT_DELAY` | `1s` | duration | No sleep on a new stream. |

## Publication

Each time a client's inputs change, the controller builds a new snapshot for
it. A build is *coherent* when every cluster its routes and listeners reference
is in its CDS and has derived endpoints. Derived endpoints can be empty, for
example when a Service has no ready Pods; that is the backend's real state, and
the build is still coherent. A referenced cluster missing from CDS, or one whose
endpoints are not derived yet, is a gap, and a build with a gap is *deferred*.

A coherent build is published at once. In `REFERENCED` mode its routes can
still wait for a reference-ahead window (see below). A deferred build is
resolved against the snapshot last published on the client's current
connection:

- a missing cluster that was published is carried from it;
- a cluster whose endpoints are not derived yet keeps the endpoints the client
  has;
- a route change onto a newly referenced, unready cluster is held, while new
  clusters and endpoints still go out.

A client is *warm* when its first listener request on a connection carries a
previously accepted version, and *cold* otherwise. This is the controller's
only signal of warmth. It means Envoy accepted listener configuration before;
it does not prove that configuration is active or complete.

### `KGW_PER_CLIENT_PUBLISH_BUDGET` (default `15s`)

Bounds two waits.

- **First publish.** A client with nothing published on its current connection
  is withheld while its builds are deferred, because publishing routes to
  unready clusters would fail them.
  - The budget starts when the controller receives the client's first deferred
    build, after per-client translation produces one, not when the proxy
    connects.
  - A client whose withhold ended because it disconnected less than a budget
    earlier resumes that start rather than restarting it.
  - At expiry the latest deferred build is published, so the proxy gets
    listeners. Routes to unready clusters return 503 until they are ready.
  - A warm client follows different rules; see the next setting.
- **Flip release.** A route change held for a newly referenced, unready cluster
  is published one budget after the hold began, so it cannot pin the client's
  route, listener and secret updates. Later holds in the same episode do not
  extend the deadline, and a publish without a hold ends the episode.

**With `0` or a negative value, neither deadline exists.** A cold client whose
referenced cluster never becomes ready is never published, so its proxy never
goes Ready, and a held route change waits for its cluster indefinitely. The
warm withhold bound and the carry bound depend on the budget, so they stop
applying too. The xDS scoping windows below keep their own timers.

Keep the budget well below the proxy's startup probe window (60 one-second
probes by default), or a proxy whose first publish waits the full budget
restarts in a loop. The budget is not a startup deadline. Time before the first
deferred build, the first-connect delay, delivery, Envoy's fetch timeouts and
cluster warming all add to it.

Metrics:

- `kgateway_xds_snapshot_perclient_defers_total{reason}`;
- `kgateway_xds_snapshot_perclient_withheld_clients{state="within_budget"}`, which
  counts nothing when the budget is `0` or negative;
- `kgateway_xds_snapshot_perclient_bounded_publishes_total` with `mode` set to
  `first_publish`, `warm_truth` or `flip_release`.

### `KGW_PER_CLIENT_WARM_WITHHOLD_BOUND` (default `10m`)

A warm client may still serve routes to a cluster that a new build lacks. So
when its first-publish budget expires, it stays withheld while any referenced
cluster is missing from CDS.

- The withhold is a separate episode. It starts when the first-publish budget
  expires and lasts at most this bound, so a warm client can wait for the budget
  plus the bound.
- A client that disconnects and reconnects within the bound resumes the episode.
- While withheld, the client receives no updates at all, including certificate
  rotations.
- At the bound, the latest deferred build is published, and routes to the
  missing clusters fail until those clusters appear.
- If a warm client's only gaps are endpoints not yet derived, it is published
  when the budget expires and keeps the endpoints it has.
- When proxies share a client (pod locality off), a cold proxy that has waited a
  budget lifts the withhold for all of them.

The bound also limits *carry*. A published client whose builds lack a cluster
it was sent keeps being sent that cluster's last configuration
(make-before-break) for at most this bound. Each carried cluster has its own
clock, separate from the warm withhold episode. The clock starts with the first
build that lacks the cluster and stops when the cluster returns.

The bound applies only while the publish budget is positive. With `0` or a
negative value, warm clients stay withheld, and missing clusters are carried,
indefinitely. Keep the bound well above how long translation takes to catch up
after a controller restart, or warm proxies lose routes on every restart.

Metrics:

- `kgateway_xds_snapshot_perclient_withheld_clients{state="warm_withheld"}`;
- `kgateway_xds_snapshot_perclient_bounded_publishes_total` with `mode` set to
  `warm_release` or `carry_expired`.

### `KGW_XDS_SNAPSHOT_CONSISTENCY_CHECK` (default `false`)

Checks every per-client snapshot, just before it is published, for these
invariants:

- every ClusterLoadAssignment and RouteConfiguration is referenced by a cluster
  or listener in the same snapshot, except the assignment for the bootstrap's
  local cluster;
- every RouteConfiguration a listener names is present.

These are structural checks. Envoy never requests an unreferenced assignment
or route, and the xDS cache answers EDS and RDS requests by the names Envoy
asks for, so such a resource no longer holds back updates of its type. A
violation still means the snapshot is not the one kgateway meant to build. It
is logged as an error and counted in
`kgateway_xds_snapshot_perclient_inconsistent_snapshots_total`.

The check only monitors: the snapshot is still published. It checks only these
invariants, so a clean result does not mean a snapshot is correct in other
ways. It is meant for tests and CI. Any increment is a kgateway bug.

### `KGW_XDS_FIRST_CONNECT_DELAY` (default `1s`)

This setting predates the publish budget and still applies; the budget did not
replace it. Each new xDS stream, including one from a reconnecting proxy,
sleeps this long on its first request:

- after the controller registers the client, which starts per-client
  translation, and runs the connection-start hooks of the client's first
  stream;
- before go-control-plane creates the stream's first watch.

The sleep gives translation a head start, so the first snapshot a stream sees is
more often complete. It blocks only that stream. A reconnecting Envoy keeps
serving its existing configuration meanwhile.

- It is a Go duration string, read once, on first use, through a `sync.Once`.
- `0` or a negative value disables the sleep.
- An invalid value logs a warning and uses `1s`.
- It is independent of the publication timers and of Envoy's timers. The
  first-publish budget starts when a deferred build reaches the controller's
  gate, which can happen during the sleep. Envoy's `initial_fetch_timeout`
  clocks run in the proxy.

### What zero means

The same value means different things in different places:

- **`KGW_XDS_FIRST_CONNECT_DELAY=0`** removes a sleep. A new stream gets its
  first watch sooner.
- **`KGW_PER_CLIENT_PUBLISH_BUDGET=0`** removes publication deadlines. Withheld
  clients and held route changes wait without limit.
- **An Envoy `initial_fetch_timeout` of `0`** disables that initialization
  deadline. Envoy waits for the resource instead of giving up and continuing
  without it.
  - kgateway's proxy bootstrap sets `0s` for CDS and LDS, and `1s` for its local
    cluster's endpoints.
  - Resources requested through generated configuration use Envoy's default of
    `15s`.

## xDS server behavior

### NACK retry throttling

go-control-plane answers a NACK by re-sending the rejected response at once, so
both sides spin at the rejection rate.

kgateway always throttles NACK retries. A NACK of the response last sent on its
stream, while the snapshot still has that version, waits for a snapshot change
or a retry delay:

- The delay starts at 1s and doubles for each consecutive rejection of the same
  version, up to 1m.
- An ACK, or a rejection of a different version, resets the delay.
- When the delay passes, the same version is sent again. Suppression is never
  permanent, so a transient failure, such as a secret that is not mounted yet,
  recovers without a configuration change.
- A NACK of an older version is answered at once.
- Rejections are still logged and counted.

Metrics:

- `kgateway_envoy_xds_nack_resends_suppressed_total{type_url}` counts each
  parked NACK, including repeat rejections after a retry;
- `kgateway_envoy_xds_rejects_total`;
- `kgateway_envoy_xds_rejects_active`.

### `KGW_XDS_RESPOND_ON_RECONNECT` (default `true`)

A proxy's first request for a resource type on a new stream carries the version
it holds and no nonce. go-control-plane answers it only when the snapshot's
version differs. After a controller restart or a cache reclaim, a rebuilt
snapshot at the same version would leave such a request unanswered.

This setting covers endpoint (EDS), route (RDS) and secret (SDS) requests. For
these, it clears the held version on a copy of the request, so the current or
next snapshot answers it. Without that, a cluster or listener warming on them
waits for Envoy's fetch timeout, with CDS paused meanwhile.

Cluster (CDS) and listener (LDS) requests are left alone. The cost is one EDS,
RDS and SDS response per reconnect.

With `false`, these requests wait for a version change. The metric is
`kgateway_envoy_xds_reconnect_responses_total{type_url}`.

### Re-defined clusters (always on)

When a CDS response changes a cluster the proxy already has (a policy change,
for example), Envoy builds a new cluster under the same name and warms it on
endpoints. CDS stays paused until it is warm. Envoy re-requests the endpoints at
the version it holds, which go-control-plane does not answer. Unless the
endpoint version changes after that CDS response, the cluster waits for Envoy's
EDS fetch timeout (15 seconds by default), and no other cluster change reaches
the proxy meanwhile.

So for each stream the xDS server notes which clusters a CDS response changed.
The stream's next endpoint request is answered with their assignments, even at
the held version. There is no setting. The cost is one hash per cluster per CDS
response, and one endpoint response carrying only the changed clusters. A
changed cluster that still waits shows up in Envoy's log as `initial fetch timed
out` for `ClusterLoadAssignment`.

## xDS scoping

### `KGW_XDS_SCOPING_MODE` (default `ALL`)

- **`ALL`** emits a CDS cluster and an EDS assignment for every backend in
  discovery scope, along with its backend-owned LDS listeners and SDS secrets.
- **`REFERENCED`** emits only the clusters the generated configuration
  references, including clusters that only filters call, such as ext_authz,
  ext_proc, rate limit, access log and JWKS clusters. EDS assignments and
  backend-owned LDS listeners and SDS secrets follow their clusters: dropping
  a backend's cluster drops those resources too. Gateway-level listeners and
  secrets are unaffected. `REFERENCED` is experimental.

Helm value: `controller.xdsScoping.mode`.

A backend can ship listeners with its cluster, as a CONNECT tunnel does. A
kept backend's listeners keep the clusters they name, such as the tunnel's
proxy, and those clusters' own listeners are followed in turn.

A gateway whose routes can pick a destination at request time reverts to
emitting every cluster, and so does one with a request-time selector in a kept
backend's listener. A warning names the selectors, and the backend and listener
for one in a backend's listener.
`kgateway_xds_cluster_scoping_disabled_gateways` is 1 for the gateway while any
of its connected proxy groups is reverted.

The exception is a cluster specifier plugin whose selector a valid plugin claim
(`ClusterEmissionClaim` in `pkg/pluginsdk`) accounts for:

- A static claim must keep at least one cluster name or name prefix. An empty
  prefix is rejected, and a claim with a rejected prefix accounts for nothing.
- A claim can instead derive each use's destinations from that use's
  `typed_config` (`DestinationsFor`). A use it declines, or fails on, keeps the
  gateway unscoped, and the warning names the reason.
- A claim's names and prefixes are kept only in gateways that use one of its
  selectors.
- Claims are collected once, when the controller starts and before its caches
  sync.
- A claim matches a selector by its extension name alone, so a plugin should
  use a name it owns. Two plugins claiming one name is logged as an error.

Claims cover the gateway's routes only. A selector in a backend's listener
always reverts the gateway, even one a claim covers in routes.

Some selectors cannot be claimed and always revert the gateway:

- cluster headers;
- filter-state overrides, such as `sni_cluster` and `set_filter_state`;
- typed configs the controller cannot decode;
- unknown cluster specifier kinds.

A claim is trusted as complete. A cluster the plugin can pick that the claim
does not keep is pruned, and the plugin's fallback then takes the traffic
without an error.

Metrics:

- `kgateway_xds_cluster_scoping_dropped_clusters`;
- `kgateway_xds_cluster_scoping_disabled_gateways`;
- `kgateway_xds_snapshot_resources{resource="Cluster"}`.

[xDS scoping](../../devel/architecture/xds-scoping.md)
explains the design.

### Transition windows

Two windows apply only in `REFERENCED` mode. With `ALL` they are off.

Each starts when the controller first publishes a build carrying the change,
just before installing it in its xDS cache. Neither observes the send, Envoy's
ACK, or Envoy applying the change, so neither guarantees a hitless transition.
Size both above your fleet's worst-case propagation.

#### `KGW_CLUSTER_REFERENCE_AHEAD` (default `2s`)

Helm value: `controller.xdsScoping.referenceAhead`.

When a build names a cluster that the client's published snapshot lacks, the
cluster and its endpoints are published at once. Routes, listeners and secrets
stay at their published versions until the cluster's window closes.

One snapshot carrying both is not enough. After a CDS response is sent, its
watch stays closed until Envoy ACKs, so a route update in that interval goes out
first on the open RDS watch. The window has a limit of its own: if the CDS send
waits behind an earlier response that has not been ACKed, or the cluster warms
slowly, the window can close first, and the route can briefly fail with
`503 NC`.

- **Bounded by the publish budget.** When the budget is positive, a hold ends
  one budget after it began. Cutting a hold short this way is counted as
  `kgateway_xds_snapshot_perclient_bounded_publishes_total{mode="reference_ahead"}`.
- **Budget `0` or negative.** A hold still ends when its windows close, but new
  clusters that keep arriving can extend it. A hold that also waits for an
  unready cluster follows the flip-release rules above.
- **`0` or negative.** The cluster and the configuration naming it publish
  together.

The metric is
`kgateway_xds_cluster_scoping_transitions_total{transition="reference_ahead_held"}`.

#### `KGW_CLUSTER_DEREFERENCE_GRACE` (default `5s`)

Helm value: `controller.xdsScoping.dereferenceGrace`.

When a build stops naming a published cluster, the cluster stays published for
this long. That gives the route update that dropped it time to land before CDS
removes the cluster. The clock starts when the controller publishes the route
update that stops naming the cluster. If that update is held for a new or
unready cluster, the clock starts at the release, so the hold does not shorten
the grace. If the route update is rejected, or delayed past the grace, the
removal can still land first.

Only clusters that scoping stops emitting are kept. A cluster whose backend is
deleted, or fails translation, is removed in the same publication, as in `ALL`
mode. Otherwise a destination selector would keep routing to the deleted
backend for the whole grace.

With `0` or a negative value, the cluster is removed in the same publication,
which accepts that race. The metric is
`kgateway_xds_cluster_scoping_transitions_total` with `transition` set to
`dereference_graced` or `dereference_pruned`.

## Interactions

- **Budget off.** With `KGW_PER_CLIENT_PUBLISH_BUDGET` at `0` or negative, the
  warm withhold bound and the carry bound stop applying. The two transition
  windows keep their timers.
- **`ALL` mode.** The transition windows are off.
- **Held configuration.** While a reference-ahead hold is in place, every
  cluster the held configuration names stays published, whatever the
  de-reference grace.
- **xDS server settings.** `KGW_XDS_FIRST_CONNECT_DELAY`,
  `KGW_XDS_RESPOND_ON_RECONNECT`, NACK retry throttling and the
  re-defined-cluster rule act in the xDS server, independently of publication.
- **Consistency check.** `KGW_XDS_SNAPSHOT_CONSISTENCY_CHECK` observes
  publications and never changes them.
