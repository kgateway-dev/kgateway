package collections

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	klabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

const crdLookupTimeout = 5 * time.Second

type delayedInformer[T controllers.ComparableObject] struct {
	inf *atomic.Pointer[kclient.Informer[T]]

	extClient        apiextensionsclient.Interface
	gvr              schema.GroupVersionResource
	newInformer      func() kclient.Informer[T]
	verifiedNotReady atomic.Bool
	pollingStarted   atomic.Bool

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

func newDelayedTypedInformer[T controllers.ComparableObject](
	c kube.Client,
	gvr schema.GroupVersionResource,
	newInformer func() kclient.Informer[T],
) kclient.Informer[T] {
	if c.Ext() == nil {
		return newInformer()
	}

	served, err := crdServesVersion(c.Ext(), gvr)
	if err != nil {
		// Discovery failed but the route API itself may still be readable. Do not
		// suppress route watching solely because CRD discovery is unavailable or
		// non-authoritative (for example due to RBAC).
		return newInformer()
	}
	if served {
		return newInformer()
	}

	delayed := &delayedInformer[T]{
		inf:       new(atomic.Pointer[kclient.Informer[T]]),
		extClient: c.Ext(),
		gvr:       gvr,
		newInformer: func() kclient.Informer[T] {
			return newInformer()
		},
	}
	// The delayed path is only used for authoritative "not served yet" results.
	// Keep HasSynced unblocked so startup does not wait on an optional CRD that
	// is absent and may be installed later.
	delayed.verifiedNotReady.Store(true)

	return delayed
}

func newDelayedDynamicUnstructuredInformer(
	c kube.Client,
	gvr schema.GroupVersionResource,
	filter kclient.Filter,
) kclient.Informer[*unstructured.Unstructured] {
	return newDelayedTypedInformer(c, gvr, func() kclient.Informer[*unstructured.Unstructured] {
		return newDynamicUnstructuredInformer(c, gvr, filter)
	})
}

func crdServesVersion(extClient apiextensionsclient.Interface, gvr schema.GroupVersionResource) (bool, error) {
	if extClient == nil {
		return false, fmt.Errorf("CRD discovery not authoritative for %s", gvr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), crdLookupTimeout)
	defer cancel()

	crd, err := extClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, fmt.Sprintf("%s.%s", gvr.Resource, gvr.Group), metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("CRD discovery not authoritative for %s: %w", gvr, err)
	}

	for _, version := range crd.Spec.Versions {
		if version.Name == gvr.Version {
			return version.Served, nil
		}
	}

	return false, nil
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

func (d *delayedInformer[T]) HasSynced() bool {
	if inf := d.inf.Load(); inf != nil {
		return (*inf).HasSynced()
	}
	return d.verifiedNotReady.Load()
}

func (d *delayedInformer[T]) HasSyncedIgnoringHandlers() bool {
	if inf := d.inf.Load(); inf != nil {
		return (*inf).HasSyncedIgnoringHandlers()
	}
	return d.verifiedNotReady.Load()
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

		for {
			if d.inf.Load() != nil {
				return
			}

			served, err := crdServesVersion(d.extClient, d.gvr)
			if err != nil {
				// Discovery is non-authoritative; unblock HasSynced so
				// startup is not held indefinitely by a flaky API call.
				d.verifiedNotReady.Store(true)
			} else {
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
