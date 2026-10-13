# Restricting the objects kgateway watches

kgateway watches every Secret, ConfigMap, and Service in the discovered namespaces (minus the
Secret types excluded by `apiclient.SecretsFieldSelector`) so that any of them can be
referenced by a Gateway, route, or policy. In clusters that hold thousands of objects
kgateway never references, those informer caches can dominate control plane memory even
though only a handful of objects matter. For Services there is a second cost: every watched
Service becomes a backend, so the cache size also sets how many clusters kgateway sends to
each Envoy.

Three opt-in settings narrow these watches, one per kind:

| Setting | Env var | Helm value |
| --- | --- | --- |
| `SecretLabelSelector` | `KGW_SECRET_LABEL_SELECTOR` | `secretLabelSelector` |
| `ConfigMapLabelSelector` | `KGW_CONFIG_MAP_LABEL_SELECTOR` | `configMapLabelSelector` |
| `ServiceLabelSelector` | `KGW_SERVICE_LABEL_SELECTOR` | `serviceLabelSelector` |

```yaml
# values.yaml
secretLabelSelector: "kgateway.dev/watch=true"
configMapLabelSelector: "kgateway.dev/watch=true"
serviceLabelSelector: "team in (a,b)"
```

Each takes any selector `labels.Parse` accepts, including set-based, inequality, existence,
and non-existence requirements. Empty, the default, watches everything. `kgateway.dev/watch=true`
is the conventional choice when there is no existing label to select on; with it, setting
`kgateway.dev/watch: "false"` on an object drops it from the watch without removing the label.
The settings are separate because the kinds are labeled by different tools (application
charts, cert-manager or external-secrets, CA bundle tooling), and a selector lets each
install keep the labels its objects already carry, or several kgateway installs each watch a
disjoint subset.

A malformed selector fails at startup, both when the environment is decoded and again in
`NewCommonCollections` before any informer exists; otherwise the API server would reject
every List and the informer would never sync. Helm also rejects a non-string value at render
time.

In every case the selector is pushed to the API server as the informer's `labelSelector`, so
non-matching objects are never sent to the control plane at all. That is the entire memory
win. A watch accepts exactly one selector, so a list of selectors (as in
`discoveryNamespaceSelectors`) could not be honored server-side; evaluating extra entries in
the controller would mean caching everything first.

Contrast with `discoveryNamespaceSelectors`, which filters on read
(`kubetypes.DynamicObjectFilter`) so that it can change without restarting watches: it scopes
*what kgateway acts on*, not what it caches.

## Unmatched references

Every object that a Gateway, HTTPRoute, or kgateway policy references must match its kind's
selector. A reference to an object that does not is indistinguishable from a reference to an
object that does not exist, and is reported the same way — for a TLS listener certificate,
`ResolvedRefs: False` with reason `InvalidCertificateRef`; for a route `backendRef`,
`ResolvedRefs: False` with reason `BackendNotFound`. A Service outside the selector is also not
matched as a waypoint.

While a selector is set, a lookup miss for that kind names the selector and where it is
configured (`krtcollections.SelectorNotFoundError`, which unwraps to `NotFoundError`), so the
cause shows up in status messages. This covers Service backends resolved through the
`BackendIndex`, Secrets resolved through the `SecretIndex`, and ConfigMaps resolved through
the `ConfigMapIndex`.

## Objects kgateway owns

kgateway cannot make the objects it creates match an arbitrary operator selector, so none of
them is required to. Each is read back through a separate, narrow watch instead:

- **The per-proxy Service and ConfigMap** rendered by the deployer. Only the gateway
  controller reads them: it enqueues the owning Gateway when they change, and derives
  `Gateway.status.addresses` from the Service. When a selector is set for the kind,
  `gw_controller.go` selects on `app.kubernetes.io/managed-by=kgateway` instead (see
  `ownedObjectLabelSelector`), which the envoy chart stamps on both and which
  `service.extraLabels` and `gatewayLabels` cannot override. That is a second informer, sized
  by the number of Gateways. With no selector set, both watches are unfiltered and share one
  informer, as before.
- **The OAuth2 HMAC Secret** (`wellknown.OAuth2HMACSecret`), created by the bootstrap
  controller and read by the OAuth2 policy through `CommonCollections.Secrets`. When
  `SecretLabelSelector` is set, `NewCommonCollections` joins a watch scoped to that Secret by
  name and namespace into the Secrets collection. Its filter matches the bootstrap
  controller's, so the two share an informer holding one object.

Neither watch is cluster-wide, which is what matters: leaving one unfiltered would keep the
cluster-wide cache the setting exists to remove. Keeping kgateway's own objects off the
operator's selector also means proxy Services are not pulled into the translation collection
as backends just so that status can be computed, and an operator cannot break
`status.addresses` or OAuth2 by excluding them.

## Plugins

A plugin that opens its own Secret, ConfigMap, or Service watch bypasses these settings.
Because informers are shared per `{type, labelSelector, fieldSelector}`, an unfiltered plugin
watch does not reuse the narrowed cache — it creates a second, cluster-wide one, and the
operator sees no memory improvement at all. Plugins should read through
`CommonCollections.Secrets`, `CommonCollections.ConfigMaps`, and `CommonCollections.Services`,
or pass the matching setting as their filter's `LabelSelector`, for example
`string(commoncol.Settings.ConfigMapLabelSelector)`. `examples/plugin/main.go` shows the
latter.

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
  same parent-enqueue purpose. They could be narrowed with the same `managed-by` selector.
- Lookups that bypass the indexes, such as the JWT provider's local JWKS ConfigMap lookup in
  `trafficpolicy/jwt.go`, report the plain not-found error without naming the selector.
