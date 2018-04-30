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
	"github.com/solo-io/gloo/pkg/log"
)

type virtualMeshesClient struct {
	crds    crdclientset.Interface
	apiexts apiexts.Interface
	// write and read objects to this namespace if not specified on the GlooObjects
	namespace     string
	syncFrequency time.Duration
}

func (c *virtualMeshesClient) Create(item *v1.VirtualMesh) (*v1.VirtualMesh, error) {
	return c.createOrUpdateVirtualMeshCrd(item, crud.OperationCreate)
}

func (c *virtualMeshesClient) Update(item *v1.VirtualMesh) (*v1.VirtualMesh, error) {
	return c.createOrUpdateVirtualMeshCrd(item, crud.OperationUpdate)
}

func (c *virtualMeshesClient) Delete(name string) error {
	return c.crds.GlooV1().VirtualMeshes(c.namespace).Delete(name, nil)
}

func (c *virtualMeshesClient) Get(name string) (*v1.VirtualMesh, error) {
	crdVirtualMesh, err := c.crds.GlooV1().VirtualMeshes(c.namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed performing get api request")
	}
	var returnedVirtualMesh v1.VirtualMesh
	if err := ConfigObjectFromCrd(
		crdVirtualMesh.ObjectMeta,
		crdVirtualMesh.Spec,
		crdVirtualMesh.Status,
		&returnedVirtualMesh); err != nil {
		return nil, errors.Wrap(err, "converting returned crd to virtualMesh")
	}
	return &returnedVirtualMesh, nil
}

func (c *virtualMeshesClient) List() ([]*v1.VirtualMesh, error) {
	crdList, err := c.crds.GlooV1().VirtualMeshes(c.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed performing list api request")
	}
	var returnedVirtualMeshes []*v1.VirtualMesh
	for _, crdVirtualMesh := range crdList.Items {
		var returnedVirtualMesh v1.VirtualMesh
		if err := ConfigObjectFromCrd(
			crdVirtualMesh.ObjectMeta,
			crdVirtualMesh.Spec,
			crdVirtualMesh.Status,
			&returnedVirtualMesh); err != nil {
			return nil, errors.Wrap(err, "converting returned crd to virtualMesh")
		}
		returnedVirtualMeshes = append(returnedVirtualMeshes, &returnedVirtualMesh)
	}
	return returnedVirtualMeshes, nil
}

func (u *virtualMeshesClient) Watch(handlers ...storage.VirtualMeshEventHandler) (*storage.Watcher, error) {
	lw := cache.NewListWatchFromClient(u.crds.GlooV1().RESTClient(), crdv1.VirtualMeshCRD.Plural, u.namespace, fields.Everything())
	sw := cache.NewSharedInformer(lw, new(crdv1.VirtualMesh), u.syncFrequency)
	for _, h := range handlers {
		sw.AddEventHandler(&virtualMeshEventHandler{handler: h, store: sw.GetStore()})
	}
	return storage.NewWatcher(func(stop <-chan struct{}, _ chan error) {
		sw.Run(stop)
	}), nil
}

func (c *virtualMeshesClient) createOrUpdateVirtualMeshCrd(virtualMesh *v1.VirtualMesh, op crud.Operation) (*v1.VirtualMesh, error) {
	virtualMeshCrd, err := ConfigObjectToCrd(c.namespace, virtualMesh)
	if err != nil {
		return nil, errors.Wrap(err, "converting gloo object to crd")
	}
	virtualMeshes := c.crds.GlooV1().VirtualMeshes(virtualMeshCrd.GetNamespace())
	var returnedCrd *crdv1.VirtualMesh
	switch op {
	case crud.OperationCreate:
		returnedCrd, err = virtualMeshes.Create(virtualMeshCrd.(*crdv1.VirtualMesh))
		if err != nil {
			if kuberrs.IsAlreadyExists(err) {
				return nil, storage.NewAlreadyExistsErr(err)
			}
			return nil, errors.Wrap(err, "kubernetes create api request")
		}
	case crud.OperationUpdate:
		// need to make sure we preserve labels
		currentCrd, err := virtualMeshes.Get(virtualMeshCrd.GetName(), metav1.GetOptions{ResourceVersion: virtualMeshCrd.GetResourceVersion()})
		if err != nil {
			return nil, errors.Wrap(err, "kubernetes get api request")
		}
		// copy labels
		virtualMeshCrd.SetLabels(currentCrd.Labels)
		returnedCrd, err = virtualMeshes.Update(virtualMeshCrd.(*crdv1.VirtualMesh))
		if err != nil {
			return nil, errors.Wrap(err, "kubernetes update api request")
		}
	}
	var returnedVirtualMesh v1.VirtualMesh
	if err := ConfigObjectFromCrd(
		returnedCrd.ObjectMeta,
		returnedCrd.Spec,
		returnedCrd.Status,
		&returnedVirtualMesh); err != nil {
		return nil, errors.Wrap(err, "converting returned crd to virtualMesh")
	}
	return &returnedVirtualMesh, nil
}

// implements the kubernetes ResourceEventHandler interface
type virtualMeshEventHandler struct {
	handler storage.VirtualMeshEventHandler
	store   cache.Store
}

func (eh *virtualMeshEventHandler) getUpdatedList() []*v1.VirtualMesh {
	updatedList := eh.store.List()
	var updatedVirtualMeshList []*v1.VirtualMesh
	for _, updated := range updatedList {
		virtualMeshCrd, ok := updated.(*crdv1.VirtualMesh)
		if !ok {
			continue
		}
		var returnedVirtualMesh v1.VirtualMesh
		if err := ConfigObjectFromCrd(
			virtualMeshCrd.ObjectMeta,
			virtualMeshCrd.Spec,
			virtualMeshCrd.Status,
			&returnedVirtualMesh); err != nil {
			log.Warnf("watch event: %v", errors.Wrap(err, "converting returned crd to virtualMesh"))
		}
		updatedVirtualMeshList = append(updatedVirtualMeshList, &returnedVirtualMesh)
	}
	return updatedVirtualMeshList
}

func convertVirtualMesh(obj interface{}) (*v1.VirtualMesh, bool) {
	virtualMeshCrd, ok := obj.(*crdv1.VirtualMesh)
	if !ok {
		return nil, ok
	}
	var returnedVirtualMesh v1.VirtualMesh
	if err := ConfigObjectFromCrd(
		virtualMeshCrd.ObjectMeta,
		virtualMeshCrd.Spec,
		virtualMeshCrd.Status,
		&returnedVirtualMesh); err != nil {
		log.Warnf("watch event: %v", errors.Wrap(err, "converting returned crd to virtualMesh"))
		return nil, false
	}
	return &returnedVirtualMesh, true
}

func (eh *virtualMeshEventHandler) OnAdd(obj interface{}) {
	virtualMesh, ok := convertVirtualMesh(obj)
	if !ok {
		return
	}
	eh.handler.OnAdd(eh.getUpdatedList(), virtualMesh)
}
func (eh *virtualMeshEventHandler) OnUpdate(_, newObj interface{}) {
	newVirtualMesh, ok := convertVirtualMesh(newObj)
	if !ok {
		return
	}
	eh.handler.OnUpdate(eh.getUpdatedList(), newVirtualMesh)
}

func (eh *virtualMeshEventHandler) OnDelete(obj interface{}) {
	virtualMesh, ok := convertVirtualMesh(obj)
	if !ok {
		return
	}
	eh.handler.OnDelete(eh.getUpdatedList(), virtualMesh)
}
