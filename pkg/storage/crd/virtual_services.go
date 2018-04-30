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

type virtualServicesClient struct {
	crds    crdclientset.Interface
	apiexts apiexts.Interface
	// write and read objects to this namespace if not specified on the GlooObjects
	namespace     string
	syncFrequency time.Duration
}

func (c *virtualServicesClient) Create(item *v1.VirtualService) (*v1.VirtualService, error) {
	return c.createOrUpdateVirtualServiceCrd(item, crud.OperationCreate)
}

func (c *virtualServicesClient) Update(item *v1.VirtualService) (*v1.VirtualService, error) {
	return c.createOrUpdateVirtualServiceCrd(item, crud.OperationUpdate)
}

func (c *virtualServicesClient) Delete(name string) error {
	return c.crds.GlooV1().VirtualServices(c.namespace).Delete(name, nil)
}

func (c *virtualServicesClient) Get(name string) (*v1.VirtualService, error) {
	crdVirtualService, err := c.crds.GlooV1().VirtualServices(c.namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed performing get api request")
	}
	returnedVirtualService, err := VirtualServiceFromCrd(crdVirtualService)
	if err != nil {
		return nil, errors.Wrap(err, "converting returned crd to virtualService")
	}
	return returnedVirtualService, nil
}

func (c *virtualServicesClient) List() ([]*v1.VirtualService, error) {
	crdList, err := c.crds.GlooV1().VirtualServices(c.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed performing list api request")
	}
	var returnedVirtualServices []*v1.VirtualService
	for _, crdVirtualService := range crdList.Items {
		virtualService, err := VirtualServiceFromCrd(&crdVirtualService)
		if err != nil {
			return nil, errors.Wrap(err, "converting returned crd to virtualService")
		}
		returnedVirtualServices = append(returnedVirtualServices, virtualService)
	}
	return returnedVirtualServices, nil
}

func (u *virtualServicesClient) Watch(handlers ...storage.VirtualServiceEventHandler) (*storage.Watcher, error) {
	lw := cache.NewListWatchFromClient(u.crds.GlooV1().RESTClient(), crdv1.VirtualServiceCRD.Plural, u.namespace, fields.Everything())
	sw := cache.NewSharedInformer(lw, new(crdv1.VirtualService), u.syncFrequency)
	for _, h := range handlers {
		sw.AddEventHandler(&virtualServiceEventHandler{handler: h, store: sw.GetStore()})
	}
	return storage.NewWatcher(func(stop <-chan struct{}, _ chan error) {
		sw.Run(stop)
	}), nil
}

func (c *virtualServicesClient) createOrUpdateVirtualServiceCrd(virtualService *v1.VirtualService, op crud.Operation) (*v1.VirtualService, error) {
	virtualServiceCrd, err := VirtualServiceToCrd(c.namespace, virtualService)
	if err != nil {
		return nil, errors.Wrap(err, "converting gloo object to crd")
	}
	virtualServices := c.crds.GlooV1().VirtualServices(virtualServiceCrd.Namespace)
	var returnedCrd *crdv1.VirtualService
	switch op {
	case crud.OperationCreate:
		returnedCrd, err = virtualServices.Create(virtualServiceCrd)
		if err != nil {
			if kuberrs.IsAlreadyExists(err) {
				return nil, storage.NewAlreadyExistsErr(err)
			}
			return nil, errors.Wrap(err, "kubernetes create api request")
		}
	case crud.OperationUpdate:
		// need to make sure we preserve labels
		currentCrd, err := virtualServices.Get(virtualServiceCrd.Name, metav1.GetOptions{ResourceVersion: virtualServiceCrd.ResourceVersion})
		if err != nil {
			return nil, errors.Wrap(err, "kubernetes get api request")
		}
		// copy labels
		virtualServiceCrd.Labels = currentCrd.Labels
		returnedCrd, err = virtualServices.Update(virtualServiceCrd)
		if err != nil {
			return nil, errors.Wrap(err, "kubernetes update api request")
		}
	}
	returnedVirtualService, err := VirtualServiceFromCrd(returnedCrd)
	if err != nil {
		return nil, errors.Wrap(err, "converting returned crd to virtualService")
	}
	return returnedVirtualService, nil
}

// implements the kubernetes ResourceEventHandler interface
type virtualServiceEventHandler struct {
	handler storage.VirtualServiceEventHandler
	store   cache.Store
}

func (eh *virtualServiceEventHandler) getUpdatedList() []*v1.VirtualService {
	updatedList := eh.store.List()
	var updatedVirtualServiceList []*v1.VirtualService
	for _, updated := range updatedList {
		virtualServiceCrd, ok := updated.(*crdv1.VirtualService)
		if !ok {
			continue
		}
		updatedVirtualService, err := VirtualServiceFromCrd(virtualServiceCrd)
		if err != nil {
			continue
		}
		updatedVirtualServiceList = append(updatedVirtualServiceList, updatedVirtualService)
	}
	return updatedVirtualServiceList
}

func convertVirtualService(obj interface{}) (*v1.VirtualService, bool) {
	virtualServiceCrd, ok := obj.(*crdv1.VirtualService)
	if !ok {
		return nil, ok
	}
	virtualService, err := VirtualServiceFromCrd(virtualServiceCrd)
	if err != nil {
		return nil, false
	}
	return virtualService, ok
}

func (eh *virtualServiceEventHandler) OnAdd(obj interface{}) {
	virtualService, ok := convertVirtualService(obj)
	if !ok {
		return
	}
	eh.handler.OnAdd(eh.getUpdatedList(), virtualService)
}
func (eh *virtualServiceEventHandler) OnUpdate(_, newObj interface{}) {
	newVirtualService, ok := convertVirtualService(newObj)
	if !ok {
		return
	}
	eh.handler.OnUpdate(eh.getUpdatedList(), newVirtualService)
}

func (eh *virtualServiceEventHandler) OnDelete(obj interface{}) {
	virtualService, ok := convertVirtualService(obj)
	if !ok {
		return
	}
	eh.handler.OnDelete(eh.getUpdatedList(), virtualService)
}
