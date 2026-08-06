package ondemand

import (
	"context"
	"fmt"
	"sync"
	"time"

	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"

	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

var logger = logging.New("krtcollections/ondemand")

// initialFetchConcurrency bounds how many full Gets run in parallel while
// populating the cache for the first time. The referenced set is normally small;
// this only matters for the pathological case of a cluster with thousands of
// referenced Secrets, where we would rather take a few extra seconds to start
// than hammer the API server.
const initialFetchConcurrency = 16

// Getter fetches the full contents of a single object.
type Getter[T controllers.ComparableObject] func(ctx context.Context, namespace, name string) (T, error)

// Cache maintains full copies of exactly those objects that some ResourceRef
// points at, backed by a cluster-wide metadata-only watch.
//
// See the package comment for the rationale.
type Cache[T controllers.ComparableObject] struct {
	name   string
	kind   string
	getter Getter[T]

	// metaClient is the cluster-wide PartialObjectMetadata watch. It answers
	// "does this object exist" and "has it changed", and backs label-selector
	// ref expansion.
	metaClient kclient.Informer[*metav1.PartialObjectMetadata]

	// out holds the full objects. Downstream KRT collections consume this and
	// cannot tell it apart from an informer-backed collection.
	out krt.StaticCollection[T]

	queue controllers.Queue

	mu sync.RWMutex
	// named holds targets of refs that name a single object.
	named sets.Set[types.NamespacedName]
	// selectors holds the currently active label-selector refs. Kept separate
	// because they force a re-expansion whenever any object's labels change,
	// which we want to skip entirely in the common case of no selector refs.
	selectors []ResourceRef
	// selected holds the current expansion of selectors. Tracked apart from
	// named so that a shrinking selector match can retract its keys without
	// disturbing keys a named ref still wants.
	selected sets.Set[types.NamespacedName]
	// fetched records the resourceVersion of each object currently published in
	// out, so a metadata event for an unchanged object costs nothing.
	fetched map[types.NamespacedName]string

	// ctx is captured in SetRefs so the queue reconciler, whose signature is
	// fixed by controllers.Queue, can cancel in-flight Gets on shutdown.
	ctx context.Context

	syncedCh   chan struct{}
	syncedOnce sync.Once
	// refsOnce guards SetRefs, which may only be called once.
	refsOnce sync.Once

	// recomputeMu serializes whole recomputations of the desired set. mu alone is
	// not enough: computing the next set reads the metadata watch outside the
	// lock, so two concurrent recomputations could each compute a set and then
	// swap in whichever finishes last, losing the other's result until the next
	// event happens to arrive.
	recomputeMu sync.Mutex
}

// Config configures a Cache.
type Config[T controllers.ComparableObject] struct {
	// Name is used for the KRT collection name and logging, e.g. "Secrets".
	Name string
	// Kind is the object kind refs must use to select this cache, e.g. "Secret".
	Kind string
	// GVR is the resource to watch.
	GVR schema.GroupVersionResource
	// Filter is applied to the metadata watch. Server-side selectors here reduce
	// what we watch at all and are strongly preferred.
	Filter kclient.Filter
	// Getter fetches the full object. It bypasses the metadata watch and reads
	// straight from the API server.
	Getter Getter[T]
}

// New builds a Cache and starts its reconciler. The returned Cache publishes an
// empty, unsynced collection until SetRefs is called and the initial set of
// referenced objects has been fetched.
func New[T controllers.ComparableObject](
	ctx context.Context,
	client kube.Client,
	krtOpts krtutil.KrtOptions,
	cfg Config[T],
) *Cache[T] {
	filter := cfg.Filter
	if filter.ObjectTransform == nil {
		filter.ObjectTransform = stripMetadata
	}

	c := &Cache[T]{
		name:       cfg.Name,
		kind:       cfg.Kind,
		getter:     cfg.Getter,
		metaClient: kclient.NewMetadata(client, cfg.GVR, filter),
		named:      sets.New[types.NamespacedName](),
		selected:   sets.New[types.NamespacedName](),
		fetched:    map[types.NamespacedName]string{},
		syncedCh:   make(chan struct{}),
	}

	// The output collection reports unsynced until the initial referenced set has
	// been fetched, so dependent collections do not observe a half-populated
	// cache and translate against missing Secrets on startup.
	c.out = krt.NewStaticCollection[T](
		channelSyncer{name: cfg.Name, ch: c.syncedCh},
		nil,
		krtOpts.ToOptions(cfg.Name)...,
	)

	c.queue = controllers.NewQueue(
		"ondemand/"+cfg.Name,
		controllers.WithReconciler(c.reconcile),
		controllers.WithMaxAttempts(maxFetchAttempts),
		controllers.WithRateLimiter(workqueue.NewTypedItemExponentialFailureRateLimiter[any](
			50*time.Millisecond, 10*time.Second)),
	)

	// Register before the initial snapshot so nothing that happens during the
	// initial fetch is lost; the queue buffers adds until Run.
	c.metaClient.AddEventHandler(controllers.FromEventHandler(c.onMetadataEvent))

	return c
}

// maxFetchAttempts bounds retries of a failing Get. A referenced object we
// cannot read is reported as absent, which surfaces to the user as a normal
// "not found" status on the referencing policy.
const maxFetchAttempts = 10

// Collection returns the collection of full objects. It only ever contains
// objects that some ResourceRef points at.
func (c *Cache[T]) Collection() krt.Collection[T] {
	return c.out
}

// HasSynced reports whether the initial referenced set has been fetched.
func (c *Cache[T]) HasSynced() bool {
	select {
	case <-c.syncedCh:
		return true
	default:
		return false
	}
}

// Exists reports whether the object exists in the cluster at all, according to
// the metadata watch. An object may exist and still be absent from Collection,
// which means no ResourceRef declared a dependency on it.
func (c *Cache[T]) Exists(namespace, name string) bool {
	return c.metaClient.Get(name, namespace) != nil
}

// SetRefs attaches the collection of references that drives what this cache
// fetches, and starts the reconciler. It may be called at most once.
//
// refs must be derived only from raw resource collections. Deriving it from a
// collection that itself reads this cache would create a dependency cycle that
// deadlocks startup: the cache would wait for refs to sync, while refs waited
// for the cache.
func (c *Cache[T]) SetRefs(ctx context.Context, refs krt.Collection[ResourceRef]) {
	started := false
	c.refsOnce.Do(func() {
		started = true
		c.ctx = ctx
		refs.RegisterBatch(func(events []krt.Event[ResourceRef]) {
			c.onRefsChanged(refs)
		}, false)
		go c.run(ctx, refs)
	})
	if !started {
		panic(fmt.Sprintf("ondemand: SetRefs called twice for %s", c.name))
	}
}

// run performs the initial population and then services incremental updates.
func (c *Cache[T]) run(ctx context.Context, refs krt.Collection[ResourceRef]) {
	if !kube.WaitForCacheSync("ondemand/"+c.name, ctx.Done(), c.metaClient.HasSynced, refs.HasSynced) {
		return
	}

	// Snapshot the referenced set and fetch it all before declaring the
	// collection synced, mirroring an informer's initial LIST.
	c.onRefsChanged(refs)
	initial := c.desiredSnapshot().UnsortedList()

	c.fetchAll(initial)

	logger.Info("on-demand cache populated",
		"kind", c.kind, "referenced", len(initial))
	c.markSynced()

	c.queue.Run(ctx.Done())
	c.metaClient.ShutdownHandlers()
}

// fetchAll populates keys with bounded parallelism.
func (c *Cache[T]) fetchAll(keys []types.NamespacedName) {
	sem := make(chan struct{}, initialFetchConcurrency)
	var wg sync.WaitGroup
	for _, key := range keys {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := c.reconcile(key); err != nil {
				// Requeue for the retry loop rather than blocking startup on a
				// transient API error.
				logger.Warn("initial fetch failed, will retry",
					"kind", c.kind, "object", key.String(), "error", err)
				c.queue.Add(key)
			}
		})
	}
	wg.Wait()
}

func (c *Cache[T]) markSynced() {
	c.syncedOnce.Do(func() { close(c.syncedCh) })
}

// onMetadataEvent handles a change to any object in the cluster. Objects nobody
// references are dropped here, which is what keeps the cluster-wide watch cheap
// in CPU as well as memory.
func (c *Cache[T]) onMetadataEvent(o controllers.Event) {
	obj := o.Latest()
	key := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}

	c.mu.RLock()
	referenced := c.named.Has(key) || c.selected.Has(key)
	hasSelectors := len(c.selectors) > 0
	c.mu.RUnlock()

	if referenced {
		c.queue.Add(key)
	}
	// Selector membership moves in both directions: an object we do not reference
	// can gain a matching label, and one we do reference can lose it. Re-expand on
	// either, or a Secret that stopped matching would stay cached forever. Only
	// pay for this when a selector ref actually exists, which is uncommon.
	if hasSelectors {
		c.queue.Add(recomputeKey)
	}
}

// recomputeKey is a sentinel queue item that triggers re-expansion of
// label-selector refs. Object names cannot collide with it because it has an
// empty name.
var recomputeKey = types.NamespacedName{Namespace: "", Name: ""}

// desiredSnapshot returns the union of named and selector-derived targets.
func (c *Cache[T]) desiredSnapshot() sets.Set[types.NamespacedName] {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.named.Union(c.selected)
}

// onRefsChanged recomputes the desired set from the current refs and enqueues
// everything that entered or left it.
func (c *Cache[T]) onRefsChanged(refs krt.Collection[ResourceRef]) {
	c.recomputeMu.Lock()
	defer c.recomputeMu.Unlock()

	named := sets.New[types.NamespacedName]()
	selected := sets.New[types.NamespacedName]()
	var selectors []ResourceRef

	for _, ref := range refs.List() {
		if ref.Kind != c.kind {
			continue
		}
		if !ref.isSelector() {
			named.Insert(ref.target())
			continue
		}
		selectors = append(selectors, ref)
		selected.Insert(c.expandSelector(ref)...)
	}

	c.mu.Lock()
	prev := c.named.Union(c.selected)
	c.named = named
	c.selected = selected
	c.selectors = selectors
	next := named.Union(selected)
	c.mu.Unlock()

	c.enqueueDiff(prev, next)
}

// enqueueDiff queues every key whose membership in the desired set changed.
func (c *Cache[T]) enqueueDiff(prev, next sets.Set[types.NamespacedName]) {
	added := next.Difference(prev)
	removed := prev.Difference(next)
	if added.Len() == 0 && removed.Len() == 0 {
		return
	}
	logger.Debug("referenced set changed",
		"kind", c.kind, "added", added.Len(), "removed", removed.Len(), "total", next.Len())
	for key := range added {
		c.queue.Add(key)
	}
	for key := range removed {
		c.queue.Add(key)
	}
}

// expandSelector resolves a label-selector ref against the metadata watch.
// Labels are present on PartialObjectMetadata, so this needs no payload.
func (c *Cache[T]) expandSelector(ref ResourceRef) []types.NamespacedName {
	sel := klabels.SelectorFromSet(ref.MatchLabels)
	matched := c.metaClient.List(ref.Namespace, sel)
	out := make([]types.NamespacedName, 0, len(matched))
	for _, m := range matched {
		out = append(out, types.NamespacedName{Namespace: m.GetNamespace(), Name: m.GetName()})
	}
	return out
}

// reconcile brings a single object's cached contents in line with the desired
// set and the metadata watch.
func (c *Cache[T]) reconcile(key types.NamespacedName) error {
	if key == recomputeKey {
		c.recomputeSelectors()
		return nil
	}

	c.mu.RLock()
	wanted := c.named.Has(key) || c.selected.Has(key)
	have, isCached := c.fetched[key]
	c.mu.RUnlock()

	if !wanted {
		if isCached {
			c.drop(key)
		}
		return nil
	}

	md := c.metaClient.Get(key.Name, key.Namespace)
	if md == nil {
		// The object does not exist (or is excluded by the watch filter). Absence
		// from the collection is how downstream reports "not found".
		if isCached {
			c.drop(key)
		}
		return nil
	}
	if isCached && have == md.GetResourceVersion() {
		return nil
	}

	obj, err := c.getter(c.ctx, key.Namespace, key.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Raced with a delete; the metadata watch will catch up.
			if isCached {
				c.drop(key)
			}
			return nil
		}
		return fmt.Errorf("fetching %s %s: %w", c.kind, key, err)
	}

	c.mu.Lock()
	// Re-check under the lock: the ref may have been withdrawn while the Get was
	// in flight, in which case publishing would leak the object into the cache.
	if !c.named.Has(key) && !c.selected.Has(key) {
		c.mu.Unlock()
		return nil
	}
	c.fetched[key] = obj.GetResourceVersion()
	c.mu.Unlock()

	c.out.UpdateObject(obj)
	return nil
}

func (c *Cache[T]) drop(key types.NamespacedName) {
	c.mu.Lock()
	delete(c.fetched, key)
	c.mu.Unlock()
	c.out.DeleteObject(key.String())
}

// recomputeSelectors re-expands label-selector refs after a label change
// somewhere in the cluster. Named refs are untouched: only the selector-derived
// half of the desired set can move as a result of a label change.
func (c *Cache[T]) recomputeSelectors() {
	c.recomputeMu.Lock()
	defer c.recomputeMu.Unlock()

	c.mu.RLock()
	selectors := c.selectors
	c.mu.RUnlock()
	if len(selectors) == 0 {
		return
	}

	selected := sets.New[types.NamespacedName]()
	for _, ref := range selectors {
		selected.Insert(c.expandSelector(ref)...)
	}

	c.mu.Lock()
	prev := c.named.Union(c.selected)
	c.selected = selected
	next := c.named.Union(selected)
	c.mu.Unlock()

	c.enqueueDiff(prev, next)
}

// stripMetadata is the informer transform for the metadata watch. It discards
// everything we never read, which matters because annotations can carry a full
// copy of the object (kubectl's last-applied-configuration) and would defeat the
// point of watching metadata only. Labels are kept: label-selector refs are
// resolved against this cache.
func stripMetadata(obj any) (any, error) {
	md, ok := obj.(*metav1.PartialObjectMetadata)
	if !ok {
		// Tombstones and unexpected types pass through untouched.
		return obj, nil
	}
	md.ManagedFields = nil
	md.Annotations = nil
	md.Finalizers = nil
	md.OwnerReferences = nil
	return md, nil
}

// channelSyncer adapts a close-once channel to krt.Syncer.
type channelSyncer struct {
	name string
	ch   <-chan struct{}
}

func (c channelSyncer) WaitUntilSynced(stop <-chan struct{}) bool {
	select {
	case <-c.ch:
		return true
	case <-stop:
		return false
	}
}

func (c channelSyncer) HasSynced() bool {
	select {
	case <-c.ch:
		return true
	default:
		return false
	}
}
