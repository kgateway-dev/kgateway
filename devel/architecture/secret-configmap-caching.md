# Secret and ConfigMap caching

kgateway does **not** keep every Secret and ConfigMap in memory. It watches them
cluster-wide with a metadata-only informer and fetches the full contents of only
those objects its configuration actually references.

If you are adding code that reads a Secret or ConfigMap, skip to
[Adding a new reference](#adding-a-new-reference). Everything else is background.

## Why

A typed informer stores the whole object, including `data`. kgateway needs the
contents of a handful of them — listener certificates, CA bundles, auth
credentials — but a cluster can easily hold thousands of large ConfigMaps that
kgateway never looks at. Caching all of them dominated the control plane's heap
for no benefit ([#13786](https://github.com/kgateway-dev/kgateway/issues/13786)).

A `PartialObjectMetadata` is a few hundred bytes no matter how large the object
is, so the cluster-wide view stays cheap while the expensive part shrinks to the
referenced set.

## How

Three pieces, all in `pkg/krtcollections/ondemand`:

1. **A metadata-only watch** (`kclient.NewMetadata`) over every Secret and
   ConfigMap. It answers two questions and nothing else: does this object exist,
   and has it changed. An `ObjectTransform` strips annotations and managed
   fields, because `kubectl.kubernetes.io/last-applied-configuration` holds a
   full copy of the object and would defeat the point.

2. **A reference set**, a KRT collection of `ondemand.ResourceRef` derived from
   the CRDs that name Secrets and ConfigMaps. Core Gateway API references come
   from `krtcollections.GatewayResourceRefs`; plugins contribute theirs through
   `pluginsdk.Plugin.ContributesResourceRefs`.

3. **A fetcher** that reconciles the two. For each referenced object it issues a
   direct `Get` and publishes the result into a `krt.StaticCollection` that
   downstream collections consume exactly as they would an informer-backed one.
   `SecretIndex` and `ConfigMapIndex` are unchanged; their backing collection is
   simply smaller.

The metadata watch is what makes this work without polling. `resourceVersion`
changes on every write, so a metadata event tells the fetcher precisely when a
referenced object needs re-reading — the same event stream a typed informer
would have used, minus the payload for the objects nobody asked for.

```
  metadata watch (all objects, no payload) ──┐
                                             ├──> fetcher ──> StaticCollection ──> SecretIndex
  ResourceRef collection (what we need)   ───┘                 (referenced only)
```

## Adding a new reference

Reading a Secret or ConfigMap that no `ResourceRef` points at fails exactly as
if the object did not exist. So a new read needs two changes:

1. The read itself, via `SecretIndex` / `ConfigMapIndex` as before.
2. A `ResourceRef` covering it, contributed from your plugin's
   `ContributesResourceRefs`, or added to the core derivation if it comes from a
   Gateway API field.

`TestEveryObjectReaderHasAReferenceSource` in `pkg/pluginsdk/collections` is a
tripwire for step 2: it fails when a file reads object contents without being
listed as covered. It cannot verify that your ref matches the object you read,
so still check that yourself.

### The one rule that will bite you

**Reference collections must derive only from raw resource collections, never
from a collection that resolves Secrets.**

Deriving refs from, say, a plugin's translated policy collection creates a
cycle: the Secret cache waits for the refs to sync, and the refs wait for the
policy IR, which waits for the Secret cache. Startup deadlocks.

In practice this means hanging your ref collection off the `krt.WrapClient`
collection, not the `krt.NewCollection`/`krt.NewStatusCollection` derived from
it. `GatewayResourceRefs` uses the raw `*gwv1.Gateway` collection rather than
`GatewayIndex.Gateways` for exactly this reason — the index attaches policies.

### Label selectors

A ref may select by label instead of by name (`ondemand.NewSelectorRef`), which
is how `TrafficPolicy`'s API-key `secretSelector` works. Labels are present on
`PartialObjectMetadata`, so the selector is evaluated against the metadata watch
and never needs to read a payload to decide what to fetch.

Selector membership moves in both directions, and both are handled: an object
that gains a matching label is fetched, and one that loses it is evicted. That
means any metadata event re-expands the selectors while a selector ref exists,
which is why the cache skips that work entirely when there are none. A broad
selector still pulls in every matching object cluster-wide, so it benefits less
from this design than a reference by name does.

## Consequences worth knowing

- **`NotLoadedError` vs `NotFoundError`.** A lookup that misses is reported as
  `NotFoundError` only when the metadata watch confirms the object really is
  absent. If the object exists but its contents are not loaded, the lookup
  returns `NotLoadedError` instead. Momentarily that is normal — a reference was
  just declared and the fetch has not landed. Persistently it means a ref site is
  missing, i.e. a bug in the derivation rather than in the user's config.

- **Startup ordering.** The caches report unsynced until the initial referenced
  set has been fetched, so nothing translates against a half-populated
  collection. `CommonCollections.HasSynced` covers this.

- **RBAC.** The controller needs `get` on Secrets and ConfigMaps in addition to
  `list` and `watch`.

- **The gateway controller's ConfigMap watch** (`gw_controller.go`) is a separate,
  typed informer used only to re-reconcile a Gateway when a child object drifts.
  It is restricted by label selector to objects kgateway deployed; without that
  restriction it would keep a full copy of every ConfigMap and negate the saving
  described here.

- **Tests.** Istio's fake client gives the metadata client its own empty tracker,
  and client-go's fake tracker never sets `resourceVersion`. `pkg/apiclient/fake`
  fixes both so tests exercise the real code path; see `metadata.go` there.
