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

func (v *virtualServicesClient) Create(item *v1.VirtualService) (*v1.VirtualService, error) {
	return v.createOrUpdateVirtualServiceCrd(item, crud.OperationCreate)
}

func (v *virtualServicesClient) Update(item *v1.VirtualService) (*v1.VirtualService, error) {
	return v.createOrUpdateVirtualServiceCrd(item, crud.OperationUpdate)
}

func (v *virtualServicesClient) Delete(name string) error {
	return v.crds.GlooV1().VirtualServices(v.namespace).Delete(name, nil)
}

func (v *virtualServicesClient) Get(name string) (*v1.VirtualService, error) {
	crdVh, err := v.crds.GlooV1().VirtualServices(v.namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed performing get api request")
	}
	returnedVirtualService, err := VirtualServiceFromCrd(crdVh)
	if err != nil {
		return nil, errors.Wrap(err, "converting returned crd to virtualService")
	}
	return returnedVirtualService, nil
}

func (v *virtualServicesClient) List() ([]*v1.VirtualService, error) {
	crdList, err := v.crds.GlooV1().VirtualServices(v.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed performing list api request")
	}
	var returnedVirtualServices []*v1.VirtualService
	for _, crdVh := range crdList.Items {
		virtualService, err := VirtualServiceFromCrd(&crdVh)
		if err != nil {
			return nil, errors.Wrap(err, "converting returned crd to virtualService")
		}
		returnedVirtualServices = append(returnedVirtualServices, virtualService)
	}
	return returnedVirtualServices, nil
}

func (v *virtualServicesClient) Watch(handlers ...storage.VirtualServiceEventHandler) (*storage.Watcher, error) {
	lw := cache.NewListWatchFromClient(v.crds.GlooV1().RESTClient(), crdv1.VirtualServiceCRD.Plural, v.namespace, fields.Everything())
	sw := cache.NewSharedInformer(lw, new(crdv1.VirtualService), v.syncFrequency)
	for _, h := range handlers {
		sw.AddEventHandler(&virtualServiceEventHandler{handler: h, store: sw.GetStore()})
	}
	return storage.NewWatcher(func(stop <-chan struct{}, _ chan error) {
		sw.Run(stop)
	}), nil
}

func (v *virtualServicesClient) createOrUpdateVirtualServiceCrd(virtualService *v1.VirtualService, op crud.Operation) (*v1.VirtualService, error) {
	vServiceCrd, err := VirtualServiceToCrd(v.namespace, virtualService)
	if err != nil {
		return nil, errors.Wrap(err, "converting gloo object to crd")
	}
	vServices := v.crds.GlooV1().VirtualServices(vServiceCrd.Namespace)
	var returnedCrd *crdv1.VirtualService
	switch op {
	case crud.OperationCreate:
		returnedCrd, err = vServices.Create(vServiceCrd)
		if err != nil {
			if kuberrs.IsAlreadyExists(err) {
				return nil, storage.NewAlreadyExistsErr(err)
			}
			err = errors.Wrap(err, "kubernetes create api request")
			return nil, err
		}
	case crud.OperationUpdate:
		// need to make sure we preserve labels and annotations
		currentCrd, err := vServices.Get(vServiceCrd.Name, metav1.GetOptions{ResourceVersion: vServiceCrd.ResourceVersion})
		if err != nil {
			return nil, errors.Wrap(err, "kubernetes get api request")
		}
		// copy labels
		vServiceCrd.Labels = currentCrd.Labels
		returnedCrd, err = vServices.Update(vServiceCrd)
		if err != nil {
			return nil, errors.Wrap(err, "kubernetes update api request")
		}
	}
	returnedvService, err := VirtualServiceFromCrd(returnedCrd)
	if err != nil {
		return nil, errors.Wrap(err, "converting returned crd to virtualService")
	}
	return returnedvService, nil
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
		vsCrd, ok := updated.(*crdv1.VirtualService)
		if !ok {
			continue
		}
		updatedVirtualService, err := VirtualServiceFromCrd(vsCrd)
		if err != nil {
			continue
		}
		updatedVirtualServiceList = append(updatedVirtualServiceList, updatedVirtualService)
	}
	return updatedVirtualServiceList
}

func convertVh(obj interface{}) (*v1.VirtualService, bool) {
	vsCrd, ok := obj.(*crdv1.VirtualService)
	if !ok {
		return nil, false
	}
	vs, err := VirtualServiceFromCrd(vsCrd)
	if err != nil {
		return nil, false
	}
	return vs, ok
}

func (eh *virtualServiceEventHandler) OnAdd(obj interface{}) {
	vs, ok := convertVh(obj)
	if !ok {
		return
	}
	eh.handler.OnAdd(eh.getUpdatedList(), vs)
}
func (eh *virtualServiceEventHandler) OnUpdate(_, newObj interface{}) {
	newVh, ok := convertVh(newObj)
	if !ok {
		return
	}
	eh.handler.OnUpdate(eh.getUpdatedList(), newVh)
}

func (eh *virtualServiceEventHandler) OnDelete(obj interface{}) {
	vs, ok := convertVh(obj)
	if !ok {
		return
	}
	eh.handler.OnDelete(eh.getUpdatedList(), vs)
}
