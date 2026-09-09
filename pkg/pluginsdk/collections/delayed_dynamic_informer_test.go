package collections

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	istiogvr "istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/kclient/clienttest"
	"istio.io/istio/pkg/test"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	extfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	"sigs.k8s.io/gateway-api/pkg/consts"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

type fakeDelayedHandlerRegistration struct {
	synced bool
}

func (f fakeDelayedHandlerRegistration) HasSynced() bool {
	return f.synced
}

func (f fakeDelayedHandlerRegistration) HasSyncedChecker() cache.DoneChecker {
	panic("not implemented; use HasSynced")
}

type fakeDelayedRawIndexer struct{}

func (f *fakeDelayedRawIndexer) Lookup(string) []any {
	return nil
}

type fakeDelayedUnstructuredInformer struct {
	mu sync.Mutex

	handlers     []cache.ResourceEventHandler
	indexNames   []string
	shutdownRegs []cache.ResourceEventHandlerRegistration
	shutdownAll  int
	starts       int

	addEventHandlerEntered chan struct{}
	addEventHandlerRelease chan struct{}
	startEntered           chan struct{}
	startRelease           chan struct{}
}

func (f *fakeDelayedUnstructuredInformer) Get(string, string) *unstructured.Unstructured {
	return nil
}

func (f *fakeDelayedUnstructuredInformer) List(string, labels.Selector) []*unstructured.Unstructured {
	return nil
}

func (f *fakeDelayedUnstructuredInformer) ListUnfiltered(string, labels.Selector) []*unstructured.Unstructured {
	return nil
}

func (f *fakeDelayedUnstructuredInformer) AddEventHandler(h cache.ResourceEventHandler) cache.ResourceEventHandlerRegistration {
	if f.addEventHandlerEntered != nil {
		close(f.addEventHandlerEntered)
	}
	if f.addEventHandlerRelease != nil {
		<-f.addEventHandlerRelease
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers = append(f.handlers, h)
	return fakeDelayedHandlerRegistration{synced: true}
}

func (f *fakeDelayedUnstructuredInformer) HasSynced() bool {
	return true
}

func (f *fakeDelayedUnstructuredInformer) HasSyncedIgnoringHandlers() bool {
	return true
}

func (f *fakeDelayedUnstructuredInformer) ShutdownHandlers() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shutdownAll++
}

func (f *fakeDelayedUnstructuredInformer) ShutdownHandler(registration cache.ResourceEventHandlerRegistration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shutdownRegs = append(f.shutdownRegs, registration)
}

func (f *fakeDelayedUnstructuredInformer) Start(<-chan struct{}) {
	if f.startEntered != nil {
		close(f.startEntered)
	}
	if f.startRelease != nil {
		<-f.startRelease
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts++
}

func (f *fakeDelayedUnstructuredInformer) Index(name string, _ func(o *unstructured.Unstructured) []string) kclient.RawIndexer {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.indexNames = append(f.indexNames, name)
	return &fakeDelayedRawIndexer{}
}

func TestDelayedDynamicUnstructuredInformerReportsSyncedWithoutCRD(t *testing.T) {
	stop := test.NewStop(t)
	_ = apiextensionsv1.AddToScheme(kube.FakeIstioScheme)
	client := kube.NewFakeClient()

	inf := newDelayedDynamicUnstructuredInformer(context.Background(), client, wellknown.XListenerSetGVR, kclient.Filter{})
	inf.Start(stop)

	require.True(t, inf.HasSynced(), "missing CRDs should not block startup")
	require.Empty(t, inf.List(metav1.NamespaceAll, labels.Everything()))
}

func TestServedVersionGateTracksAbsenceAuthoritatively(t *testing.T) {
	_ = apiextensionsv1.AddToScheme(kube.FakeIstioScheme)
	client := kube.NewFakeClient()

	start, resolved := servedVersionGate(newRouteVersionSource(client), wellknown.TLSRouteV1Alpha3GVR)(context.Background())
	require.True(t, resolved)
	require.False(t, start)
}

func TestServedVersionGateStartsWhenVersionIsServed(t *testing.T) {
	_ = apiextensionsv1.AddToScheme(kube.FakeIstioScheme)
	client := kube.NewFakeClient()
	makeServedCRD(t, client, wellknown.TLSRouteV1Alpha3GVR, "v1.4.1")

	start, resolved := servedVersionGate(newRouteVersionSource(client), wellknown.TLSRouteV1Alpha3GVR)(context.Background())
	require.True(t, resolved)
	require.True(t, start)
}

// An unresolved gate must park, never start. A real informer for a version that turns out not
// to be served can only 404 its initial list, so it never syncs -- and HasSynced propagates
// from there through every collection built on it into the control plane's unbounded
// WaitForCacheSync. Parking trades that hang for one kind going unreconciled until the poll
// loop resolves it.
func TestGatedInformerParksWhenTheGateCannotResolve(t *testing.T) {
	stop := test.NewStop(t)
	built := false

	inf := newGatedTypedInformer(
		context.Background(),
		wellknown.TLSRouteV1GVR,
		func(context.Context) (bool, bool) { return false, false },
		func() kclient.Informer[*unstructured.Unstructured] {
			built = true
			return &fakeDelayedUnstructuredInformer{}
		},
	)
	inf.Start(stop)

	require.False(t, built, "an unresolved version must not get a live informer")
	require.True(t, inf.HasSynced(), "a parked informer must not hold the control plane's cache sync")
	require.Empty(t, inf.List(metav1.NamespaceAll, labels.Everything()))
}

// The flip side: once the gate resolves to yes, the parked placeholder must hand over to a
// real informer, so a kind installed after startup is still picked up.
func TestGatedInformerStartsOnceTheGateResolvesToServed(t *testing.T) {
	stop := test.NewStop(t)
	var served atomic.Bool
	fake := &fakeDelayedUnstructuredInformer{}

	inf := newGatedTypedInformer(
		context.Background(),
		wellknown.TLSRouteV1GVR,
		func(context.Context) (bool, bool) { return served.Load(), true },
		func() kclient.Informer[*unstructured.Unstructured] { return fake },
	)
	inf.Start(stop)
	served.Store(true)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		assert.Equal(c, 1, fake.starts)
	}, 10*time.Second, 10*time.Millisecond, "the parked informer should hand over once its version is served")
}

func TestDelayedDynamicUnstructuredInformerSetPublishesInformerAfterReplay(t *testing.T) {
	handlerSynced := func() bool { return false }
	delayedReg := delayedHandlerRegistration{hasSynced: new(atomic.Pointer[func() bool])}
	delayedReg.hasSynced.Store(&handlerSynced)
	delayedIndex := delayedUnstructuredIndex{
		name:    "by-name",
		indexer: new(atomic.Pointer[kclient.RawIndexer]),
		extract: func(*unstructured.Unstructured) []string { return nil },
	}
	stop := make(chan struct{})
	fake := &fakeDelayedUnstructuredInformer{
		addEventHandlerEntered: make(chan struct{}),
		addEventHandlerRelease: make(chan struct{}),
		startEntered:           make(chan struct{}),
		startRelease:           make(chan struct{}),
	}
	delayed := &delayedUnstructuredInformer{
		inf: new(atomic.Pointer[kclient.Informer[*unstructured.Unstructured]]),
		handlers: []delayedUnstructuredHandler{{
			ResourceEventHandler: cache.ResourceEventHandlerFuncs{},
			hasSynced:            delayedReg,
		}},
		indexers: []delayedUnstructuredIndex{delayedIndex},
		started:  stop,
	}

	done := make(chan struct{})
	go func() {
		delayed.set(fake)
		close(done)
	}()

	<-fake.addEventHandlerEntered
	require.Nil(t, delayed.inf.Load(), "informer should not be published before delayed handlers replay")

	close(fake.addEventHandlerRelease)

	<-fake.startEntered
	require.Nil(t, delayed.inf.Load(), "informer should not be published before delayed start completes")

	close(fake.startRelease)
	<-done

	require.NotNil(t, delayed.inf.Load())
	require.True(t, delayedReg.HasSynced(), "delayed handler registration should switch to the real registration")
	require.NotNil(t, delayedIndex.indexer.Load(), "delayed index should switch to the real indexer")

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Len(t, fake.handlers, 1, "delayed handlers should replay onto the real informer")
	require.Equal(t, []string{"by-name"}, fake.indexNames)
	require.Equal(t, 1, fake.starts, "set should start the real informer when Start already ran")
}

func TestDelayedDynamicUnstructuredInformerMutationsUseInstalledInformer(t *testing.T) {
	fake := &fakeDelayedUnstructuredInformer{}
	delayed := &delayedUnstructuredInformer{
		inf: new(atomic.Pointer[kclient.Informer[*unstructured.Unstructured]]),
	}
	var installed kclient.Informer[*unstructured.Unstructured] = fake
	delayed.inf.Store(&installed)

	reg := delayed.AddEventHandler(cache.ResourceEventHandlerFuncs{})
	idx := delayed.Index("by-name", func(*unstructured.Unstructured) []string { return nil })
	delayed.ShutdownHandler(reg)
	delayed.ShutdownHandlers()

	stop := make(chan struct{})
	delayed.Start(stop)

	require.NotNil(t, idx)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Len(t, fake.handlers, 1)
	require.Equal(t, []string{"by-name"}, fake.indexNames)
	require.Len(t, fake.shutdownRegs, 1)
	require.Equal(t, reg, fake.shutdownRegs[0])
	require.Equal(t, 1, fake.shutdownAll)
	require.Equal(t, 1, fake.starts)
}

func makeServedCRD(t *testing.T, client kube.Client, resource schema.GroupVersionResource, bundleVersion string) {
	makeCRDWithVersions(t, client, resource, bundleVersion, []apiextensionsv1.CustomResourceDefinitionVersion{{
		Name:    resource.Version,
		Served:  true,
		Storage: true,
	}})
}

func makeGatewayAPIV141TLSRouteCRD(t *testing.T, client kube.Client) {
	makeCRDWithVersions(t, client, wellknown.TLSRouteV1Alpha3GVR, "v1.4.1", []apiextensionsv1.CustomResourceDefinitionVersion{
		{
			Name:    gwv1a2.GroupVersion.Version,
			Served:  true,
			Storage: false,
		},
		{
			Name:    wellknown.TLSRouteV1Alpha3Version,
			Served:  true,
			Storage: true,
		},
	})
}

func makeCRDWithVersions(
	t *testing.T,
	client kube.Client,
	resource schema.GroupVersionResource,
	bundleVersion string,
	versions []apiextensionsv1.CustomResourceDefinitionVersion,
) {
	t.Helper()

	clienttest.MakeCRDWithAnnotations(t, client, resource, map[string]string{
		consts.BundleVersionAnnotation: bundleVersion,
	})

	extClient, ok := client.Ext().(*extfake.Clientset)
	require.True(t, ok)

	err := extClient.Tracker().Add(&apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: resource.Resource + "." + resource.Group,
			Annotations: map[string]string{
				consts.BundleVersionAnnotation: bundleVersion,
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: resource.Group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural: resource.Resource,
				Kind:   wellknown.TLSRouteKind,
			},
			Scope:    apiextensionsv1.NamespaceScoped,
			Versions: versions,
		},
	})
	if apierrors.IsAlreadyExists(err) {
		err = extClient.Tracker().Update(istiogvr.CustomResourceDefinition, &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: resource.Resource + "." + resource.Group,
				Annotations: map[string]string{
					consts.BundleVersionAnnotation: bundleVersion,
				},
			},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Group: resource.Group,
				Names: apiextensionsv1.CustomResourceDefinitionNames{
					Plural: resource.Resource,
					Kind:   wellknown.TLSRouteKind,
				},
				Scope:    apiextensionsv1.NamespaceScoped,
				Versions: versions,
			},
		}, "")
	}
	require.NoError(t, err)
}
