# Restricting the Secrets and ConfigMaps kgateway watches

kgateway watches every Secret and ConfigMap in the discovered namespaces (minus the Secret
types excluded by `apiclient.SecretsFieldSelector`) so that any of them can be referenced by
a Gateway, route, or policy. In clusters that hold thousands of Secrets or ConfigMaps
kgateway never references, those informer caches can dominate control plane memory even
though only a handful of objects matter.

Two opt-in settings switch each kind to a labeled-only watch:

| Setting | Env var | Helm value |
| --- | --- | --- |
| `SecretDiscoveryMode` | `KGW_SECRET_DISCOVERY_MODE` | `discovery.secrets.mode` |
| `ConfigMapDiscoveryMode` | `KGW_CONFIG_MAP_DISCOVERY_MODE` | `discovery.configMaps.mode` |

Both accept `ALL` (default, current behavior) or `LABELED`, and are validated at startup and
at chart render time.

```yaml
# values.yaml
discovery:
  secrets:
    mode: LABELED
  configMaps:
    mode: LABELED
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

Every Secret and ConfigMap that a Gateway, HTTPRoute, or kgateway policy references must
carry the label. A reference to an object that does not is indistinguishable from a
reference to an object that does not exist, and is reported the same way — for a TLS listener
certificate, for example, `ResolvedRefs: False` with reason `InvalidCertificateRef`.

## Why a fixed label rather than a configurable selector

The label is pushed to the API server as the informer's `labelSelector`, so the objects are
never sent to the control plane at all. That is the entire memory win, and it constrains the
API more than it first appears: a watch accepts exactly one selector, so a configurable list
of selectors (as in `discoveryNamespaceSelectors`) could not be honored server-side, and
evaluating the extra entries in the controller would mean caching everything first.

Given that, a single well-known label is the better API: nothing to choose, nothing that can
be misconfigured into an unsatisfiable or accidentally-empty selector, one thing to document,
and kgateway can label the objects it owns itself without deriving labels from operator
config. Adding a selector setting later would be additive if a real need for one shows up.

Contrast with `discoveryNamespaceSelectors`, which filters on read
(`kubetypes.DynamicObjectFilter`) so that it can change without restarting watches: it scopes
*what kgateway acts on*, not what it caches.

## Objects kgateway owns

Two kinds of object are written by kgateway and read back through these same watches, so
kgateway labels them itself. Both are labeled in every mode — the label is inert in `ALL`
mode, and always applying it means switching to `LABELED` needs no migration:

- **The OAuth2 HMAC Secret** (`wellknown.OAuth2HMACSecret`), created by the bootstrap
  controller and read by the OAuth2 policy. If the Secret predates the label, or someone edits
  the label away, the bootstrap controller adds it back with a merge patch rather than
  recreating the Secret, which would rotate the key. That controller's own watch is scoped by
  name, not by label, so it keeps observing the Secret in either mode and can heal it without
  a restart — which is the whole reason it reconciles on add and update, not just delete.
- **The per-proxy ConfigMap** rendered by the deployer, labeled in the envoy chart's
  `configmap.yaml`.

## One informer, not two

`gw_controller.go` opens a ConfigMap client of its own, to re-reconcile a Gateway when the
ConfigMap the deployer rendered for its proxy changes. It applies the *same* selector as the
translation collection's ConfigMap watch, which matters because `kclient` shares informers
keyed on `{type, labelSelector, fieldSelector}`: identical selectors mean the two clients
share one cache. Leaving it unfiltered in `LABELED` mode would keep a second, cluster-wide
ConfigMap cache and negate the setting; giving it a *different* selector would add a second
cache in every mode. Labeling the deployer's ConfigMap is what lets both watches use one
selector.

Note that the gateway controller also watches Deployments, Services, and ServiceAccounts
cluster-wide for the same parent-enqueue purpose. Narrowing those would save more memory and
is not done yet.

## Plugins

A plugin that opens its own Secret or ConfigMap watch bypasses these settings. Because
informers are shared per `{type, labelSelector, fieldSelector}`, an unfiltered plugin watch
does not reuse the narrowed cache — it creates a second, cluster-wide one, and the operator
sees no memory improvement at all. Plugins should read through `CommonCollections.Secrets`
and `CommonCollections.ConfigMaps`, or pass
`collections.WatchLabelSelector(commoncol.Settings.ConfigMapDiscoveryMode)` as their filter's
`LabelSelector`. `examples/plugin/main.go` shows the latter.

## Known gaps

- The settings are read once at startup. Changing one requires a controller restart, which a
  Helm upgrade does anyway.
- There is no cluster-wide way to find out which objects kgateway *would* need labeled before
  switching a live install to `LABELED`; references that lose their target surface as status
  conditions after the fact.
