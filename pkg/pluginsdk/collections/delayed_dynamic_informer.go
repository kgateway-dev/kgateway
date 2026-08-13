package collections

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	klabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

// informerGate answers whether the informer it guards should be running, from freshly read
// discovery. ok is false when discovery could not establish anything, which is not the same
// as a definite no -- and the difference matters in both directions. Starting an informer on
// a version that turns out not to be served means its initial list can only 404: it never
// syncs, and because HasSynced propagates through every collection built on it, the whole
// cache sync hangs. Treating an unreadable version as definitely absent means silently
// giving up on a kind that may well be installed. So an unresolved gate parks the informer,
// which is retried for as long as the process runs.
type informerGate func(ctx context.Context) (start bool, ok bool)

type delayedInformer[T controllers.ComparableObject] struct {
	inf *atomic.Pointer[kclient.Informer[T]]

	ctx            context.Context
	gvr            schema.GroupVersionResource
	gate           informerGate
	newInformer    func() kclient.Informer[T]
	pollingStarted atomic.Bool

	mu       sync.Mutex
	handlers []delayedHandler[T]
	indexers []delayedIndex[T]
	started  <-chan struct{}
}

type delayedHandler[T controllers.ComparableObject] struct {
	cache.ResourceEventHandler
	hasSynced delayedHandlerRegistration
}

type delayedHandlerRegistration struct {
	hasSynced *atomic.Pointer[func() bool]
}

func (r delayedHandlerRegistration) HasSynced() bool {
	if synced := r.hasSynced.Load(); synced != nil {
		return (*synced)()
	}
	return false
}

func (r delayedHandlerRegistration) HasSyncedChecker() cache.DoneChecker {
	panic("not implemented; use HasSynced")
}

type delayedIndex[T controllers.ComparableObject] struct {
	name    string
	indexer *atomic.Pointer[kclient.RawIndexer]
	extract func(o T) []string
}

func (d delayedIndex[T]) Lookup(key string) []any {
	if indexer := d.indexer.Load(); indexer != nil {
		return (*indexer).Lookup(key)
	}
	return nil
}

type (
	delayedUnstructuredInformer = delayedInformer[*unstructured.Unstructured]
	delayedUnstructuredHandler  = delayedHandler[*unstructured.Unstructured]
	delayedUnstructuredIndex    = delayedIndex[*unstructured.Unstructured]
)

// newGatedTypedInformer builds the informer only if its gate says it should be running, and
// otherwise parks it behind a placeholder that keeps polling the gate. Nothing here starts an
// informer on an unconfirmed version: see informerGate for why that is the one outcome we
// cannot recover from.
func newGatedTypedInformer[T controllers.ComparableObject](
	ctx context.Context,
	gvr schema.GroupVersionResource,
	gate informerGate,
	newInformer func() kclient.Informer[T],
) kclient.Informer[T] {
	if start, resolved := gate(ctx); resolved && start {
		return newInformer()
	}

	return &delayedInformer[T]{
		inf:         new(atomic.Pointer[kclient.Informer[T]]),
		ctx:         ctx,
		gvr:         gvr,
		gate:        gate,
		newInformer: newInformer,
	}
}

func newDelayedTypedInformer[T controllers.ComparableObject](
	ctx context.Context,
	c kube.Client,
	gvr schema.GroupVersionResource,
	newInformer func() kclient.Informer[T],
) kclient.Informer[T] {
	return newGatedTypedInformer(ctx, gvr, servedVersionGate(newRouteVersionSource(c), gvr), newInformer)
}

func newDelayedDynamicUnstructuredInformer(
	ctx context.Context,
	c kube.Client,
	gvr schema.GroupVersionResource,
	filter kclient.Filter,
) kclient.Informer[*unstructured.Unstructured] {
	return newDelayedTypedInformer(ctx, c, gvr, func() kclient.Informer[*unstructured.Unstructured] {
		return newDynamicUnstructuredInformer(c, gvr, filter)
	})
}

func newDynamicUnstructuredInformer(
	c kube.Client,
	gvr schema.GroupVersionResource,
	filter kclient.Filter,
) kclient.Informer[*unstructured.Unstructured] {
	return &typedDynamicUnstructuredInformer{
		inner: kclient.NewDynamic(c, gvr, filter),
	}
}

type typedDynamicUnstructuredInformer struct {
	inner kclient.Untyped
}

func (t *typedDynamicUnstructuredInformer) Get(name, namespace string) *unstructured.Unstructured {
	obj := t.inner.Get(name, namespace)
	if obj == nil {
		return nil
	}
	unstructuredObj, _ := obj.(*unstructured.Unstructured)
	return unstructuredObj
}

func (t *typedDynamicUnstructuredInformer) List(namespace string, selector klabels.Selector) []*unstructured.Unstructured {
	var out []*unstructured.Unstructured
	for _, obj := range t.inner.List(namespace, selector) {
		unstructuredObj, ok := obj.(*unstructured.Unstructured)
		if ok {
			out = append(out, unstructuredObj)
		}
	}
	return out
}

func (t *typedDynamicUnstructuredInformer) ListUnfiltered(namespace string, selector klabels.Selector) []*unstructured.Unstructured {
	var out []*unstructured.Unstructured
	for _, obj := range t.inner.ListUnfiltered(namespace, selector) {
		unstructuredObj, ok := obj.(*unstructured.Unstructured)
		if ok {
			out = append(out, unstructuredObj)
		}
	}
	return out
}

func (t *typedDynamicUnstructuredInformer) AddEventHandler(h cache.ResourceEventHandler) cache.ResourceEventHandlerRegistration {
	return t.inner.AddEventHandler(h)
}

func (t *typedDynamicUnstructuredInformer) HasSynced() bool {
	return t.inner.HasSynced()
}

func (t *typedDynamicUnstructuredInformer) HasSyncedIgnoringHandlers() bool {
	return t.inner.HasSyncedIgnoringHandlers()
}

func (t *typedDynamicUnstructuredInformer) ShutdownHandlers() {
	t.inner.ShutdownHandlers()
}

func (t *typedDynamicUnstructuredInformer) ShutdownHandler(registration cache.ResourceEventHandlerRegistration) {
	t.inner.ShutdownHandler(registration)
}

func (t *typedDynamicUnstructuredInformer) Start(stop <-chan struct{}) {
	t.inner.Start(stop)
}

func (t *typedDynamicUnstructuredInformer) Index(name string, extract func(o *unstructured.Unstructured) []string) kclient.RawIndexer {
	return t.inner.Index(name, func(o controllers.Object) []string {
		unstructuredObj, ok := o.(*unstructured.Unstructured)
		if !ok {
			return nil
		}
		return extract(unstructuredObj)
	})
}

func (d *delayedInformer[T]) Get(name, namespace string) T {
	if inf := d.inf.Load(); inf != nil {
		return (*inf).Get(name, namespace)
	}
	var empty T
	return empty
}

func (d *delayedInformer[T]) List(namespace string, selector klabels.Selector) []T {
	if inf := d.inf.Load(); inf != nil {
		return (*inf).List(namespace, selector)
	}
	return nil
}

func (d *delayedInformer[T]) ListUnfiltered(namespace string, selector klabels.Selector) []T {
	if inf := d.inf.Load(); inf != nil {
		return (*inf).ListUnfiltered(namespace, selector)
	}
	return nil
}

func (d *delayedInformer[T]) AddEventHandler(h cache.ResourceEventHandler) cache.ResourceEventHandlerRegistration {
	inf, reg := func() (*kclient.Informer[T], cache.ResourceEventHandlerRegistration) {
		d.mu.Lock()
		defer d.mu.Unlock()

		if inf := d.inf.Load(); inf != nil {
			return inf, nil
		}

		reg := delayedHandlerRegistration{hasSynced: new(atomic.Pointer[func() bool])}
		hasSynced := d.HasSynced
		reg.hasSynced.Store(&hasSynced)
		d.handlers = append(d.handlers, delayedHandler[T]{
			ResourceEventHandler: h,
			hasSynced:            reg,
		})
		return nil, reg
	}()
	if inf != nil {
		return (*inf).AddEventHandler(h)
	}
	return reg
}

// HasSynced reports true while parked. There is nothing to sync -- a parked informer holds
// no watch and its collection is empty -- and reporting false instead would hold the whole
// control plane's cache sync, which is unbounded, on one kind that may never arrive. Once an
// informer is installed its own readiness takes over, so a kind that does show up is still
// waited for properly.
func (d *delayedInformer[T]) HasSynced() bool {
	if inf := d.inf.Load(); inf != nil {
		return (*inf).HasSynced()
	}
	return true
}

func (d *delayedInformer[T]) HasSyncedIgnoringHandlers() bool {
	if inf := d.inf.Load(); inf != nil {
		return (*inf).HasSyncedIgnoringHandlers()
	}
	return true
}

func (d *delayedInformer[T]) ShutdownHandlers() {
	inf := func() *kclient.Informer[T] {
		d.mu.Lock()
		defer d.mu.Unlock()

		if inf := d.inf.Load(); inf != nil {
			return inf
		}
		d.handlers = nil
		return nil
	}()
	if inf != nil {
		(*inf).ShutdownHandlers()
	}
}

func (d *delayedInformer[T]) ShutdownHandler(registration cache.ResourceEventHandlerRegistration) {
	inf := func() *kclient.Informer[T] {
		d.mu.Lock()
		defer d.mu.Unlock()

		if inf := d.inf.Load(); inf != nil {
			return inf
		}

		filtered := d.handlers[:0]
		for _, handler := range d.handlers {
			if handler.hasSynced != registration {
				filtered = append(filtered, handler)
			}
		}
		d.handlers = filtered
		return nil
	}()
	if inf != nil {
		(*inf).ShutdownHandler(registration)
	}
}

func (d *delayedInformer[T]) Start(stop <-chan struct{}) {
	inf := d.recordStart(stop)
	if inf != nil {
		(*inf).Start(stop)
		return
	}

	d.startPolling(stop)
}

// recordStart stores the stop channel and returns the currently published
// informer (if any) while holding d.mu. Kept in its own function so defer
// guarantees the lock is released even if a future change introduces a panic
// between acquisition and release.
func (d *delayedInformer[T]) recordStart(stop <-chan struct{}) *kclient.Informer[T] {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.started = stop
	return d.inf.Load()
}

func (d *delayedInformer[T]) Index(name string, extract func(o T) []string) kclient.RawIndexer {
	inf, index := func() (*kclient.Informer[T], kclient.RawIndexer) {
		d.mu.Lock()
		defer d.mu.Unlock()

		if inf := d.inf.Load(); inf != nil {
			return inf, nil
		}

		index := delayedIndex[T]{
			name:    name,
			indexer: new(atomic.Pointer[kclient.RawIndexer]),
			extract: extract,
		}
		d.indexers = append(d.indexers, index)
		return nil, index
	}()
	if inf != nil {
		return (*inf).Index(name, extract)
	}
	return index
}

func (d *delayedInformer[T]) startPolling(stop <-chan struct{}) {
	if !d.pollingStarted.CompareAndSwap(false, true) {
		return
	}

	go func() {
		const (
			initialInterval = time.Second
			maxInterval     = 30 * time.Second
		)
		interval := initialInterval
		timer := time.NewTimer(interval)
		defer timer.Stop()

		// Reported once per unresolved spell rather than every round: at the steady-state
		// interval this would otherwise be a line every 30s, forever, per parked informer.
		reportedUnresolved := false

		for {
			if d.inf.Load() != nil {
				return
			}

			start, resolved := d.gate(d.ctx)
			switch {
			case !resolved:
				if !reportedUnresolved {
					logger.Warn("cannot determine whether this API version is served; its resources are not being reconciled, and this will keep being retried",
						"gvr", d.gvr.String(),
					)
					reportedUnresolved = true
				}
			case start:
				logger.Info("API version is now served; starting its informer", "gvr", d.gvr.String())
				d.set(d.newInformer())
				return
			default:
				reportedUnresolved = false
			}

			select {
			case <-stop:
				return
			case <-d.ctx.Done():
				return
			case <-timer.C:
				interval = min(interval*2, maxInterval)
				timer.Reset(interval)
			}
		}
	}()
}

func (d *delayedInformer[T]) set(inf kclient.Informer[T]) {
	if inf == nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	for _, handler := range d.handlers {
		reg := inf.AddEventHandler(handler)
		hasSynced := reg.HasSynced
		handler.hasSynced.hasSynced.Store(&hasSynced)
	}
	d.handlers = nil

	for _, indexer := range d.indexers {
		idx := inf.Index(indexer.name, indexer.extract)
		indexer.indexer.Store(&idx)
	}
	d.indexers = nil

	if d.started != nil {
		inf.Start(d.started)
	}

	// Publish the informer only after replaying delayed state so callers never
	// observe a partially initialized informer transition.
	d.inf.Store(&inf)
}

var (
	_ kclient.Informer[*unstructured.Unstructured] = &typedDynamicUnstructuredInformer{}
	_ kclient.Informer[*unstructured.Unstructured] = &delayedInformer[*unstructured.Unstructured]{}
)
