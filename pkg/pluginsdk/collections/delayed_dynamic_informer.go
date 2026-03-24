package collections

import (
	"sync"
	"sync/atomic"

	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/kubetypes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	klabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

type delayedUnstructuredInformer struct {
	inf *atomic.Pointer[kclient.Informer[*unstructured.Unstructured]]

	watcher kubetypes.CrdWatcher

	mu       sync.Mutex
	handlers []delayedUnstructuredHandler
	indexers []delayedUnstructuredIndex
	started  <-chan struct{}
}

type delayedUnstructuredHandler struct {
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

type delayedUnstructuredIndex struct {
	name    string
	indexer *atomic.Pointer[kclient.RawIndexer]
	extract func(o *unstructured.Unstructured) []string
}

func (d delayedUnstructuredIndex) Lookup(key string) []any {
	if indexer := d.indexer.Load(); indexer != nil {
		return (*indexer).Lookup(key)
	}
	return nil
}

func newDelayedDynamicUnstructuredInformer(
	c kube.Client,
	gvr schema.GroupVersionResource,
	filter kclient.Filter,
) kclient.Informer[*unstructured.Unstructured] {
	watcher := c.CrdWatcher()
	if watcher == nil {
		panic("newDelayedDynamicUnstructuredInformer called without a CRD watcher enabled")
	}

	delayed := &delayedUnstructuredInformer{
		inf:     new(atomic.Pointer[kclient.Informer[*unstructured.Unstructured]]),
		watcher: watcher,
	}

	readyNow := watcher.KnownOrCallback(gvr, func(stop <-chan struct{}) {
		inf := newDynamicUnstructuredInformer(c, gvr, filter)
		inf.Start(stop)
		delayed.set(inf)
	})
	if readyNow {
		return newDynamicUnstructuredInformer(c, gvr, filter)
	}

	return delayed
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

func (d *delayedUnstructuredInformer) Get(name, namespace string) *unstructured.Unstructured {
	if inf := d.inf.Load(); inf != nil {
		return (*inf).Get(name, namespace)
	}
	return nil
}

func (d *delayedUnstructuredInformer) List(namespace string, selector klabels.Selector) []*unstructured.Unstructured {
	if inf := d.inf.Load(); inf != nil {
		return (*inf).List(namespace, selector)
	}
	return nil
}

func (d *delayedUnstructuredInformer) ListUnfiltered(namespace string, selector klabels.Selector) []*unstructured.Unstructured {
	if inf := d.inf.Load(); inf != nil {
		return (*inf).ListUnfiltered(namespace, selector)
	}
	return nil
}

func (d *delayedUnstructuredInformer) AddEventHandler(h cache.ResourceEventHandler) cache.ResourceEventHandlerRegistration {
	if inf := d.inf.Load(); inf != nil {
		return (*inf).AddEventHandler(h)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	reg := delayedHandlerRegistration{hasSynced: new(atomic.Pointer[func() bool])}
	reg.hasSynced.Store(new(d.watcher.HasSynced))
	d.handlers = append(d.handlers, delayedUnstructuredHandler{
		ResourceEventHandler: h,
		hasSynced:            reg,
	})
	return reg
}

func (d *delayedUnstructuredInformer) HasSynced() bool {
	if inf := d.inf.Load(); inf != nil {
		return (*inf).HasSynced()
	}
	return d.watcher.HasSynced()
}

func (d *delayedUnstructuredInformer) HasSyncedIgnoringHandlers() bool {
	if inf := d.inf.Load(); inf != nil {
		return (*inf).HasSyncedIgnoringHandlers()
	}
	return d.watcher.HasSynced()
}

func (d *delayedUnstructuredInformer) ShutdownHandlers() {
	if inf := d.inf.Load(); inf != nil {
		(*inf).ShutdownHandlers()
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers = nil
}

func (d *delayedUnstructuredInformer) ShutdownHandler(registration cache.ResourceEventHandlerRegistration) {
	if inf := d.inf.Load(); inf != nil {
		(*inf).ShutdownHandler(registration)
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	filtered := d.handlers[:0]
	for _, handler := range d.handlers {
		if handler.hasSynced != registration {
			filtered = append(filtered, handler)
		}
	}
	d.handlers = filtered
}

func (d *delayedUnstructuredInformer) Start(stop <-chan struct{}) {
	if inf := d.inf.Load(); inf != nil {
		(*inf).Start(stop)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.started = stop
}

func (d *delayedUnstructuredInformer) Index(name string, extract func(o *unstructured.Unstructured) []string) kclient.RawIndexer {
	if inf := d.inf.Load(); inf != nil {
		return (*inf).Index(name, extract)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	index := delayedUnstructuredIndex{
		name:    name,
		indexer: new(atomic.Pointer[kclient.RawIndexer]),
		extract: extract,
	}
	d.indexers = append(d.indexers, index)
	return index
}

func (d *delayedUnstructuredInformer) set(inf kclient.Informer[*unstructured.Unstructured]) {
	if inf == nil {
		return
	}

	d.inf.Swap(&inf)

	d.mu.Lock()
	defer d.mu.Unlock()

	for _, handler := range d.handlers {
		reg := inf.AddEventHandler(handler)
		handler.hasSynced.hasSynced.Store(new(reg.HasSynced))
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
}

var (
	_ kclient.Informer[*unstructured.Unstructured] = &typedDynamicUnstructuredInformer{}
	_ kclient.Informer[*unstructured.Unstructured] = &delayedUnstructuredInformer{}
)
