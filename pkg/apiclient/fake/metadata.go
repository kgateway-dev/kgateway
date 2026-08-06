package fake

import (
	"fmt"
	"strconv"
	"sync/atomic"

	"istio.io/istio/pkg/config/schema/gvr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/metadata"
	metadatafake "k8s.io/client-go/metadata/fake"
	clienttesting "k8s.io/client-go/testing"
)

// wireMetadataReadThrough makes the fake metadata client serve reads from the
// typed fake client's object tracker.
//
// Istio's fake kube.Client gives the metadata client its own, empty
// ObjectTracker, unrelated to the typed one and never seeded with the test's
// objects. Any code that watches a resource with kclient.NewMetadata -- as the
// on-demand Secret and ConfigMap caches do -- would therefore see an empty
// cluster in tests while the typed clients see the real fixtures.
//
// Rather than mirroring writes (which would drift on patches and race on
// timing), delegate reads: get, list and watch on the metadata client are
// answered from the typed tracker and converted to PartialObjectMetadata. That
// is exactly what a real API server does, so tests exercise the production code
// path with no special-casing.
func wireMetadataReadThrough(metadataClient metadata.Interface, typed *kubefake.Clientset) {
	tracker := typed.Tracker()
	fake, ok := metadataClient.(*metadatafake.FakeMetadataClient)
	if !ok {
		panic(fmt.Sprintf("unexpected fake metadata client type %T", metadataClient))
	}

	// Only intercept resources the typed clientset actually holds. Istio runs its
	// own metadata watch over CustomResourceDefinitions, which live in the
	// apiextensions tracker; answering that from the kube tracker would report an
	// empty cluster and stall every CRD-gated informer.
	handled := func(action clienttesting.Action) bool {
		switch action.GetResource() {
		case gvr.Secret, gvr.ConfigMap:
			return true
		default:
			return false
		}
	}

	fake.PrependReactor("get", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if !handled(action) {
			return false, nil, nil
		}
		get := action.(clienttesting.GetAction)
		obj, err := tracker.Get(action.GetResource(), action.GetNamespace(), get.GetName())
		if err != nil {
			return true, nil, err
		}
		md, err := toPartialObjectMetadata(obj)
		if err != nil {
			return true, nil, err
		}
		return true, md, nil
	})

	fake.PrependReactor("list", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if !handled(action) {
			return false, nil, nil
		}
		gvr := action.GetResource()
		objs, err := tracker.List(gvr, listKindFor(gvr), action.GetNamespace())
		if err != nil {
			return true, nil, err
		}
		items, err := meta.ExtractList(objs)
		if err != nil {
			return true, nil, err
		}

		// metadataResourceClient.List expects a *metav1.List of
		// *metav1.PartialObjectMetadata and applies the label selector itself.
		out := &metav1.List{}
		for _, item := range items {
			md, err := toPartialObjectMetadata(item)
			if err != nil {
				return true, nil, err
			}
			out.Items = append(out.Items, runtime.RawExtension{Object: md})
		}
		if lm, err := meta.ListAccessor(objs); err == nil {
			out.ResourceVersion = lm.GetResourceVersion()
		}
		return true, out, nil
	})

	fake.PrependWatchReactor("*", func(action clienttesting.Action) (bool, watch.Interface, error) {
		if !handled(action) {
			return false, nil, nil
		}
		w, err := tracker.Watch(action.GetResource(), action.GetNamespace())
		if err != nil {
			return true, nil, err
		}
		return true, newMetadataWatch(w), nil
	})
}

// listKindFor returns the item kind the tracker expects for a resource. The
// tracker appends "List" itself when building the returned list object, so this
// must be the singular kind.
func listKindFor(gvr schema.GroupVersionResource) schema.GroupVersionKind {
	return gvr.GroupVersion().WithKind(kindForResource(gvr.Resource))
}

func kindForResource(resource string) string {
	switch resource {
	case "secrets":
		return "Secret"
	case "configmaps":
		return "ConfigMap"
	default:
		// Fall back to a naive singularisation; only used if a new resource gains
		// a metadata watch without updating this switch.
		if len(resource) > 1 && resource[len(resource)-1] == 's' {
			r := []rune(resource)
			r[0] = []rune(string(r[0]))[0] - 32
			return string(r[:len(r)-1])
		}
		return resource
	}
}

func toPartialObjectMetadata(obj runtime.Object) (*metav1.PartialObjectMetadata, error) {
	acc, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	return &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{Kind: "PartialObjectMetadata", APIVersion: "meta.k8s.io/v1"},
		ObjectMeta: *(&metav1.ObjectMeta{
			Name:            acc.GetName(),
			Namespace:       acc.GetNamespace(),
			UID:             acc.GetUID(),
			ResourceVersion: acc.GetResourceVersion(),
			Labels:          acc.GetLabels(),
			Annotations:     acc.GetAnnotations(),
			Generation:      acc.GetGeneration(),
		}).DeepCopy(),
	}, nil
}

// metadataWatch converts a typed watch stream into a PartialObjectMetadata one.
type metadataWatch struct {
	inner watch.Interface
	out   chan watch.Event
	stop  chan struct{}
}

func newMetadataWatch(inner watch.Interface) watch.Interface {
	w := &metadataWatch{
		inner: inner,
		out:   make(chan watch.Event),
		stop:  make(chan struct{}),
	}
	go w.run()
	return w
}

func (w *metadataWatch) run() {
	defer close(w.out)
	for {
		select {
		case <-w.stop:
			return
		case ev, ok := <-w.inner.ResultChan():
			if !ok {
				return
			}
			if ev.Object != nil {
				md, err := toPartialObjectMetadata(ev.Object)
				if err == nil {
					ev.Object = md
				}
			}
			select {
			case w.out <- ev:
			case <-w.stop:
				return
			}
		}
	}
}

func (w *metadataWatch) Stop() {
	select {
	case <-w.stop:
	default:
		close(w.stop)
	}
	w.inner.Stop()
}

func (w *metadataWatch) ResultChan() <-chan watch.Event { return w.out }

// wireResourceVersions makes the typed fake client assign a fresh
// resourceVersion on every write to a Secret or ConfigMap.
//
// client-go's ObjectTracker leaves resourceVersion empty and never bumps it. A
// real API server changes it on every write, and the on-demand caches rely on
// exactly that: a metadata event whose resourceVersion is unchanged means the
// object's contents are unchanged and no re-fetch is needed. Without this, an
// update to a Secret in a test would be invisible to anything reading through
// the cache, which is a silent wrong answer rather than a visible failure.
//
// Scoped to the two resources the caches watch, to avoid perturbing tests that
// make assertions about other objects.
func wireResourceVersions(typed *kubefake.Clientset) {
	var counter atomic.Int64

	bump := func(action clienttesting.Action) (bool, runtime.Object, error) {
		switch action.GetResource() {
		case gvr.Secret, gvr.ConfigMap:
		default:
			return false, nil, nil
		}
		obj, ok := action.(interface{ GetObject() runtime.Object })
		if !ok {
			return false, nil, nil
		}
		acc, err := meta.Accessor(obj.GetObject())
		if err != nil {
			return false, nil, nil
		}
		acc.SetResourceVersion(strconv.FormatInt(counter.Add(1), 10))
		// Fall through so the tracker stores the object we just stamped.
		return false, nil, nil
	}

	typed.PrependReactor("create", "*", bump)
	typed.PrependReactor("update", "*", bump)
}
