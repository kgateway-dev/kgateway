package crd

import (
	"time"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/storage"
	crdclientset "github.com/solo-io/gloo/pkg/storage/crd/client/clientset/versioned"
	crdv1 "github.com/solo-io/gloo/pkg/storage/crd/solo.io/v1"
	apiexts "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/solo-io/gloo/pkg/storage/crud"
	kuberrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
)

type upstreamsClient struct {
	crds    crdclientset.Interface
	apiexts apiexts.Interface
	// write and read objects to this namespace if not specified on the GlooObjects
	namespace     string
	syncFrequency time.Duration
}

func (c *upstreamsClient) Create(item *v1.Upstream) (*v1.Upstream, error) {
	return c.createOrUpdateUpstreamCrd(item, crud.OperationCreate)
}

func (c *upstreamsClient) Update(item *v1.Upstream) (*v1.Upstream, error) {
	return c.createOrUpdateUpstreamCrd(item, crud.OperationUpdate)
}

func (c *upstreamsClient) Delete(name string) error {
	return c.crds.GlooV1().Upstreams(c.namespace).Delete(name, nil)
}

func (c *upstreamsClient) Get(name string) (*v1.Upstream, error) {
	crdUpstream, err := c.crds.GlooV1().Upstreams(c.namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed performing get api request")
	}
	returnedUpstream, err := UpstreamFromCrd(crdUpstream)
	if err != nil {
		return nil, errors.Wrap(err, "converting returned crd to upstream")
	}
	return returnedUpstream, nil
}

func (c *upstreamsClient) List() ([]*v1.Upstream, error) {
	crdList, err := c.crds.GlooV1().Upstreams(c.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed performing list api request")
	}
	var returnedUpstreams []*v1.Upstream
	for _, crdUpstream := range crdList.Items {
		upstream, err := UpstreamFromCrd(&crdUpstream)
		if err != nil {
			return nil, errors.Wrap(err, "converting returned crd to upstream")
		}
		returnedUpstreams = append(returnedUpstreams, upstream)
	}
	return returnedUpstreams, nil
}

func (u *upstreamsClient) Watch(handlers ...storage.UpstreamEventHandler) (*storage.Watcher, error) {
	lw := cache.NewListWatchFromClient(u.crds.GlooV1().RESTClient(), crdv1.UpstreamCRD.Plural, u.namespace, fields.Everything())
	sw := cache.NewSharedInformer(lw, new(crdv1.Upstream), u.syncFrequency)
	for _, h := range handlers {
		sw.AddEventHandler(&upstreamEventHandler{handler: h, store: sw.GetStore()})
	}
	return storage.NewWatcher(func(stop <-chan struct{}, _ chan error) {
		sw.Run(stop)
	}), nil
}

func (c *upstreamsClient) createOrUpdateUpstreamCrd(upstream *v1.Upstream, op crud.Operation) (*v1.Upstream, error) {
	upstreamCrd, err := UpstreamToCrd(c.namespace, upstream)
	if err != nil {
		return nil, errors.Wrap(err, "converting gloo object to crd")
	}
	upstreams := c.crds.GlooV1().Upstreams(upstreamCrd.Namespace)
	var returnedCrd *crdv1.Upstream
	switch op {
	case crud.OperationCreate:
		returnedCrd, err = upstreams.Create(upstreamCrd)
		if err != nil {
			if kuberrs.IsAlreadyExists(err) {
				return nil, storage.NewAlreadyExistsErr(err)
			}
			return nil, errors.Wrap(err, "kubernetes create api request")
		}
	case crud.OperationUpdate:
		// need to make sure we preserve labels
		currentCrd, err := upstreams.Get(upstreamCrd.Name, metav1.GetOptions{ResourceVersion: upstreamCrd.ResourceVersion})
		if err != nil {
			return nil, errors.Wrap(err, "kubernetes get api request")
		}
		// copy labels
		upstreamCrd.Labels = currentCrd.Labels
		returnedCrd, err = upstreams.Update(upstreamCrd)
		if err != nil {
			return nil, errors.Wrap(err, "kubernetes update api request")
		}
	}
	returnedUpstream, err := UpstreamFromCrd(returnedCrd)
	if err != nil {
		return nil, errors.Wrap(err, "converting returned crd to upstream")
	}
	return returnedUpstream, nil
}

// implements the kubernetes ResourceEventHandler interface
type upstreamEventHandler struct {
	handler storage.UpstreamEventHandler
	store   cache.Store
}

func (eh *upstreamEventHandler) getUpdatedList() []*v1.Upstream {
	updatedList := eh.store.List()
	var updatedUpstreamList []*v1.Upstream
	for _, updated := range updatedList {
		upstreamCrd, ok := updated.(*crdv1.Upstream)
		if !ok {
			continue
		}
		updatedUpstream, err := UpstreamFromCrd(upstreamCrd)
		if err != nil {
			continue
		}
		updatedUpstreamList = append(updatedUpstreamList, updatedUpstream)
	}
	return updatedUpstreamList
}

func convertUpstream(obj interface{}) (*v1.Upstream, bool) {
	upstreamCrd, ok := obj.(*crdv1.Upstream)
	if !ok {
		return nil, ok
	}
	upstream, err := UpstreamFromCrd(upstreamCrd)
	if err != nil {
		return nil, false
	}
	return upstream, ok
}

func (eh *upstreamEventHandler) OnAdd(obj interface{}) {
	upstream, ok := convertUpstream(obj)
	if !ok {
		return
	}
	eh.handler.OnAdd(eh.getUpdatedList(), upstream)
}
func (eh *upstreamEventHandler) OnUpdate(_, newObj interface{}) {
	newUpstream, ok := convertUpstream(newObj)
	if !ok {
		return
	}
	eh.handler.OnUpdate(eh.getUpdatedList(), newUpstream)
}

func (eh *upstreamEventHandler) OnDelete(obj interface{}) {
	upstream, ok := convertUpstream(obj)
	if !ok {
		return
	}
	eh.handler.OnDelete(eh.getUpdatedList(), upstream)
}
