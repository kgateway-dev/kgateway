# Restricting the objects kgateway watches

kgateway watches every Secret, ConfigMap, and Service in the discovered namespaces (minus the
Secret types excluded by `apiclient.SecretsFieldSelector`) so that any of them can be
referenced by a Gateway, route, or policy. In clusters that hold thousands of objects
kgateway never references, those informer caches can dominate control plane memory even
though only a handful of objects matter. For Services there is a second cost: every watched
Service becomes a backend, so the cache size also sets how many clusters kgateway sends to
each Envoy.

Three opt-in settings narrow these watches:

| Setting | Env var | Helm value | Values |
| --- | --- | --- | --- |
| `SecretDiscoveryMode` | `KGW_SECRET_DISCOVERY_MODE` | `secretDiscoveryMode` | `ALL` (default) or `LABELED` |
| `ConfigMapDiscoveryMode` | `KGW_CONFIG_MAP_DISCOVERY_MODE` | `configMapDiscoveryMode` | `ALL` (default) or `LABELED` |
| `ServiceLabelSelector` | `KGW_SERVICE_LABEL_SELECTOR` | `serviceLabelSelector` | a label selector; empty (default) watches all |

All three are validated at startup; the two modes are also validated at chart render time.

In every case the filter is pushed to the API server as the informer's `labelSelector`, so
non-matching objects are never sent to the control plane at all. That is the entire memory
win. A watch accepts exactly one selector, so a list of selectors (as in
`discoveryNamespaceSelectors`) could not be honored server-side; evaluating extra entries in
the controller would mean caching everything first.

Contrast with `discoveryNamespaceSelectors`, which filters on read
(`kubetypes.DynamicObjectFilter`) so that it can change without restarting watches: it scopes
*what kgateway acts on*, not what it caches.

Every object that a Gateway, HTTPRoute, or kgateway policy references must be selected. A
reference to an object that is not is indistinguishable from a reference to an object that
does not exist, and is reported the same way — for a TLS listener certificate,
`ResolvedRefs: False` with reason `InvalidCertificateRef`; for a route `backendRef`,
`ResolvedRefs: False` with reason `BackendNotFound`. A Service outside the selector is also
not matched as a waypoint. With `ServiceLabelSelector` set, the `BackendNotFound` message for
a missing Service also names the active selector and how it was configured
(`krtcollections.ServiceBackendNotFoundError`), since kgateway cannot tell a nonexistent
Service from an excluded one.

## Secrets and ConfigMaps: the watch label

```yaml
# values.yaml
secretDiscoveryMode: LABELED
configMapDiscoveryMode: LABELED
```

In `LABELED` mode kgateway watches only objects carrying:

```yaml
metadata:
  labels:
    kgateway.dev/watch: "true"
```

The key and value are `wellknown.WatchLabel` / `wellknown.WatchLabelValue`. Matching is on
the exact value, so setting the label to anything else (`"false"`) drops an object from the
watch without having to remove the label.

### Objects kgateway owns

Two kinds of object are written by kgateway and read back through these watches, so kgateway
labels them itself. Both are labeled in every mode — the label is inert in `ALL` mode, and
always applying it means switching to `LABELED` needs no migration:

- **The OAuth2 HMAC Secret** (`wellknown.OAuth2HMACSecret`), created by the bootstrap
  controller and read by the OAuth2 policy. If the Secret predates the label, or someone edits
  the label away, the bootstrap controller adds it back with a merge patch rather than
  recreating the Secret, which would rotate the key. That controller's own watch is scoped by
  name, not by label, so it keeps observing the Secret in either mode and can heal it without
  a restart — which is the whole reason it reconciles on add and update, not just delete.
- **The per-proxy ConfigMap** rendered by the deployer, labeled in the envoy chart's
  `configmap.yaml`. `gw_controller.go` opens a ConfigMap client of its own, to re-reconcile a
  Gateway when that ConfigMap changes, and gives it the same selector as the translation
  collection's watch. `kclient` shares informers keyed on `{type, labelSelector,
  fieldSelector}`, so identical selectors mean one cache.

## Services: a label selector

```yaml
# values.yaml
serviceLabelSelector: "team in (a,b)"   # or "kgateway.dev/watch=true", "tier!=batch", ...
```

Any selector `labels.Parse` accepts works, including set-based and existence requirements.
A malformed selector fails at startup, both when the environment is decoded and again in
`NewCommonCollections` before any informer exists; otherwise the API server would reject
every List and the Service informer would never sync.
This lets operators select on labels their Services already carry, and lets several kgateway
installs each watch a disjoint subset of Services, neither of which a single fixed label can
express. To get the same behavior as the Secret and ConfigMap modes, use
`kgateway.dev/watch=true`.

### The proxy Services kgateway renders

The gateway controller reads the Service the deployer renders per proxy to derive
`Gateway.status.addresses`, and enqueues the owning Gateway when it changes. kgateway cannot
make its own Services match an arbitrary operator selector, so that watch does not use it.
Instead, when `ServiceLabelSelector` is set, `gw_controller.go` selects on
`app.kubernetes.io/managed-by=kgateway`, which the envoy chart stamps on every proxy Service
and which `service.extraLabels` and `gatewayLabels` cannot override (see
`proxyServiceLabelSelector`).

That makes two Service informers rather than one: the translation collection's, sized by the
operator's selector, and the gateway controller's, sized by the number of Gateways. Neither
is cluster-wide, which is what matters — leaving the gateway controller's watch unfiltered
would keep the cluster-wide cache the setting exists to remove. With no selector set, both
watches are unfiltered and share one informer, as before.

Keeping the two concerns on separate labels also means proxy Services are not pulled into the
translation collection as backends just so that status can be computed, and an operator
cannot break `status.addresses` by excluding a proxy Service from their selector.

## Plugins

A plugin that opens its own Secret, ConfigMap, or Service watch bypasses these settings.
Because informers are shared per `{type, labelSelector, fieldSelector}`, an unfiltered plugin
watch does not reuse the narrowed cache — it creates a second, cluster-wide one, and the
operator sees no memory improvement at all. Plugins should read through
`CommonCollections.Secrets`, `CommonCollections.ConfigMaps`, and `CommonCollections.Services`,
or pass the same selector as their filter's `LabelSelector`:
`collections.WatchLabelSelector(commoncol.Settings.ConfigMapDiscoveryMode)` for ConfigMaps and
Secrets, `string(commoncol.Settings.ServiceLabelSelector)` for Services.
`examples/plugin/main.go` shows the former.

## Known gaps

- The settings are read once at startup. Changing one requires a controller restart, which a
  Helm upgrade does anyway.
- There is no cluster-wide way to find out which objects kgateway *would* need selected before
  narrowing a live install; references that lose their target surface as status conditions
  after the fact.
- `ServiceLabelSelector` does not narrow the EndpointSlice watch (see
  `extensions2/plugins/kubernetes/k8s.go`), which stays cluster-wide. Endpoints are joined to
  Service backends, so this is correct — it just leaves the larger share of the memory on the
  table. Narrowing it would need the Service's labels mirrored onto EndpointSlices, which
  kgateway does not own.
- The gateway controller also watches Deployments and ServiceAccounts cluster-wide for the
  same parent-enqueue purpose. They could be narrowed with the same `managed-by` selector the
  proxy Service watch uses.
