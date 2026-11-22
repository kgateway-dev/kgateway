package apiclient

import (
	"cmp"
	"maps"
	"slices"
	"sync"

	"istio.io/istio/pkg/kube/informerfactory"
	"istio.io/istio/pkg/kube/kubetypes"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
)

// TrackInformerResources wraps client and returns a snapshot function for the
// resources requested through its informer factory, including delayed informers
// once their feature and API gates allow them to be constructed.
func TrackInformerResources(client Client) (Client, func() []schema.GroupVersionResource) {
	factory := &trackingInformerFactory{
		InformerFactory: client.Informers(),
		resources:       sets.New[schema.GroupVersionResource](),
	}
	return &informerTrackingClient{Client: client, factory: factory}, factory.snapshot
}

type informerTrackingClient struct {
	Client
	factory *trackingInformerFactory
}

func (c *informerTrackingClient) Informers() informerfactory.InformerFactory {
	return c.factory
}

type trackingInformerFactory struct {
	informerfactory.InformerFactory
	mu        sync.RWMutex
	resources sets.Set[schema.GroupVersionResource]
}

func (f *trackingInformerFactory) InformerFor(
	resource schema.GroupVersionResource,
	opts kubetypes.InformerOptions,
	newFunc informerfactory.NewInformerFunc,
) informerfactory.StartableInformer {
	informer := f.InformerFactory.InformerFor(resource, opts, newFunc)
	f.mu.Lock()
	f.resources.Insert(resource)
	f.mu.Unlock()
	return informer
}

func (f *trackingInformerFactory) snapshot() []schema.GroupVersionResource {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return slices.SortedFunc(maps.Keys(f.resources), func(a, b schema.GroupVersionResource) int {
		return cmp.Compare(a.String(), b.String())
	})
}
