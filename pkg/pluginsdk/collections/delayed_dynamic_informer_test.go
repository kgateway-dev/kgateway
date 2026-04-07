package collections

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	"sigs.k8s.io/gateway-api/pkg/consts"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
)

type fakeDelayedHandlerRegistration struct {
	synced bool
}

func (f fakeDelayedHandlerRegistration) HasSynced() bool {
	return f.synced
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

func TestDelayedDynamicUnstructuredInformerReportsSyncedWithoutTLSRouteCRD_Issue13661(t *testing.T) {
	stop := test.NewStop(t)
	_ = apiextensionsv1.AddToScheme(kube.FakeIstioScheme)
	client := kube.NewFakeClient()

	inf := newDelayedDynamicUnstructuredInformer(client, wellknown.LegacyTLSRouteGVR, kclient.Filter{})
	inf.Start(stop)

	require.True(t, inf.HasSynced(), "missing legacy TLSRoute CRDs should not block startup")
	require.Empty(t, inf.List(metav1.NamespaceAll, labels.Everything()))
}

func TestDelayedDynamicUnstructuredInformerBypassesCrdWatcherFilterForLegacyTLSRoute_Issue13735(t *testing.T) {
	stop := test.NewStop(t)
	_ = apiextensionsv1.AddToScheme(kube.FakeIstioScheme)
	client := kube.NewFakeClient()
	makeServedCRD(t, client, wellknown.LegacyTLSRouteGVR, "v1.4.1")

	_, err := client.Dynamic().Resource(wellknown.LegacyTLSRouteGVR).Namespace("default").Create(
		context.Background(),
		&unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": wellknown.LegacyTLSRouteGVK.GroupVersion().String(),
				"kind":       wellknown.TLSRouteKind,
				"metadata": map[string]any{
					"name":      "legacy-route",
					"namespace": "default",
				},
				"spec": map[string]any{
					"parentRefs": []any{
						map[string]any{
							"name": "gateway",
						},
					},
					"hostnames": []any{"example.com"},
				},
			},
		},
		metav1.CreateOptions{},
	)
	require.NoError(t, err)

	client.RunAndWait(stop)

	require.False(t, client.CrdWatcher().KnownOrCallback(wellknown.LegacyTLSRouteGVR, func(<-chan struct{}) {}),
		"Gateway API v1.4.x legacy TLSRoute should be filtered from CrdWatcher known state")

	inf := newDelayedDynamicUnstructuredInformer(client, wellknown.LegacyTLSRouteGVR, kclient.Filter{})
	inf.Start(stop)

	require.Eventually(t, inf.HasSynced, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		return len(inf.List("default", labels.Everything())) == 1
	}, time.Second, 10*time.Millisecond, "legacy TLSRoute should still be discoverable through the dynamic informer path")
}

func TestCrdServesVersionWithNilClientIsNonAuthoritative(t *testing.T) {
	served, err := crdServesVersion(nil, wellknown.LegacyTLSRouteGVR)
	require.Error(t, err)
	require.False(t, served)
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
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name:    resource.Version,
				Served:  true,
				Storage: true,
			}},
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
				Scope: apiextensionsv1.NamespaceScoped,
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
					Name:    resource.Version,
					Served:  true,
					Storage: true,
				}},
			},
		}, "")
	}
	require.NoError(t, err)
}
