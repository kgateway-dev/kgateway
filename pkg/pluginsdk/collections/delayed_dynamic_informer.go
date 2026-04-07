package collections

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	klabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

type delayedUnstructuredInformer struct {
	inf *atomic.Pointer[kclient.Informer[*unstructured.Unstructured]]

	extClient        apiextensionsclient.Interface
	gvr              schema.GroupVersionResource
	newInformer      func() kclient.Informer[*unstructured.Unstructured]
	verifiedNotReady atomic.Bool
	pollingStarted   atomic.Bool

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
	if c.Ext() == nil {
		return newDynamicUnstructuredInformer(c, gvr, filter)
	}

	served, err := crdServesVersion(c.Ext(), gvr)
	if err == nil && served {
		return newDynamicUnstructuredInformer(c, gvr, filter)
	}

	delayed := &delayedUnstructuredInformer{
		inf:       new(atomic.Pointer[kclient.Informer[*unstructured.Unstructured]]),
		extClient: c.Ext(),
		gvr:       gvr,
		newInformer: func() kclient.Informer[*unstructured.Unstructured] {
			return newDynamicUnstructuredInformer(c, gvr, filter)
		},
	}
	if err == nil {
		delayed.verifiedNotReady.Store(true)
	}

	return delayed
}

func crdServesVersion(extClient apiextensionsclient.Interface, gvr schema.GroupVersionResource) (bool, error) {
	result := getServedVersions(extClient, fmt.Sprintf("%s.%s", gvr.Resource, gvr.Group), gvr.Version)
	if !result.Authoritative {
		return false, fmt.Errorf("CRD discovery not authoritative for %s", gvr)
	}
	return result.Served[gvr.Version], nil
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
	d.mu.Lock()
	if inf := d.inf.Load(); inf != nil {
		d.mu.Unlock()
		return (*inf).AddEventHandler(h)
	}

	reg := delayedHandlerRegistration{hasSynced: new(atomic.Pointer[func() bool])}
	hasSynced := d.HasSynced
	reg.hasSynced.Store(&hasSynced)
	d.handlers = append(d.handlers, delayedUnstructuredHandler{
		ResourceEventHandler: h,
		hasSynced:            reg,
	})
	d.mu.Unlock()
	return reg
}

func (d *delayedUnstructuredInformer) HasSynced() bool {
	if inf := d.inf.Load(); inf != nil {
		return (*inf).HasSynced()
	}
	return d.verifiedNotReady.Load()
}

func (d *delayedUnstructuredInformer) HasSyncedIgnoringHandlers() bool {
	if inf := d.inf.Load(); inf != nil {
		return (*inf).HasSyncedIgnoringHandlers()
	}
	return d.verifiedNotReady.Load()
}

func (d *delayedUnstructuredInformer) ShutdownHandlers() {
	d.mu.Lock()
	if inf := d.inf.Load(); inf != nil {
		d.mu.Unlock()
		(*inf).ShutdownHandlers()
		return
	}
	d.handlers = nil
	d.mu.Unlock()
}

func (d *delayedUnstructuredInformer) ShutdownHandler(registration cache.ResourceEventHandlerRegistration) {
	d.mu.Lock()
	if inf := d.inf.Load(); inf != nil {
		d.mu.Unlock()
		(*inf).ShutdownHandler(registration)
		return
	}

	filtered := d.handlers[:0]
	for _, handler := range d.handlers {
		if handler.hasSynced != registration {
			filtered = append(filtered, handler)
		}
	}
	d.handlers = filtered
	d.mu.Unlock()
}

func (d *delayedUnstructuredInformer) Start(stop <-chan struct{}) {
	d.mu.Lock()
	d.started = stop
	inf := d.inf.Load()
	d.mu.Unlock()

	if inf != nil {
		(*inf).Start(stop)
		return
	}

	d.startPolling(stop)
}

func (d *delayedUnstructuredInformer) Index(name string, extract func(o *unstructured.Unstructured) []string) kclient.RawIndexer {
	d.mu.Lock()
	if inf := d.inf.Load(); inf != nil {
		d.mu.Unlock()
		return (*inf).Index(name, extract)
	}

	index := delayedUnstructuredIndex{
		name:    name,
		indexer: new(atomic.Pointer[kclient.RawIndexer]),
		extract: extract,
	}
	d.indexers = append(d.indexers, index)
	d.mu.Unlock()
	return index
}

func (d *delayedUnstructuredInformer) startPolling(stop <-chan struct{}) {
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

		for {
			if d.inf.Load() != nil {
				return
			}

			served, err := crdServesVersion(d.extClient, d.gvr)
			if err == nil {
				d.verifiedNotReady.Store(!served)
				if served {
					d.set(d.newInformer())
					return
				}
			}

			select {
			case <-stop:
				return
			case <-timer.C:
				interval = min(interval*2, maxInterval)
				timer.Reset(interval)
			}
		}
	}()
}

func (d *delayedUnstructuredInformer) set(inf kclient.Informer[*unstructured.Unstructured]) {
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
	_ kclient.Informer[*unstructured.Unstructured] = &delayedUnstructuredInformer{}
)
