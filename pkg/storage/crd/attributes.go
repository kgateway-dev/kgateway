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

type attributesClient struct {
	crds    crdclientset.Interface
	apiexts apiexts.Interface
	// write and read objects to this namespace if not specified on the GlooObjects
	namespace     string
	syncFrequency time.Duration
}

func (c *attributesClient) Create(item *v1.Attribute) (*v1.Attribute, error) {
	return c.createOrUpdateAttributeCrd(item, crud.OperationCreate)
}

func (c *attributesClient) Update(item *v1.Attribute) (*v1.Attribute, error) {
	return c.createOrUpdateAttributeCrd(item, crud.OperationUpdate)
}

func (c *attributesClient) Delete(name string) error {
	return c.crds.GlooV1().Attributes(c.namespace).Delete(name, nil)
}

func (c *attributesClient) Get(name string) (*v1.Attribute, error) {
	crdAttribute, err := c.crds.GlooV1().Attributes(c.namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed performing get api request")
	}
	var returnedAttribute v1.Attribute
	if err := ConfigObjectFromCrd(
		crdAttribute.ObjectMeta,
		crdAttribute.Spec,
		crdAttribute.Status,
		&returnedAttribute); err != nil {
		return nil, errors.Wrap(err, "converting returned crd to attribute")
	}
	return &returnedAttribute, nil
}

func (c *attributesClient) List() ([]*v1.Attribute, error) {
	crdList, err := c.crds.GlooV1().Attributes(c.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed performing list api request")
	}
	var returnedAttributes []*v1.Attribute
	for _, crdAttribute := range crdList.Items {
		var returnedAttribute v1.Attribute
		if err := ConfigObjectFromCrd(
			crdAttribute.ObjectMeta,
			crdAttribute.Spec,
			crdAttribute.Status,
			&returnedAttribute); err != nil {
			return nil, errors.Wrap(err, "converting returned crd to attribute")
		}
		returnedAttributes = append(returnedAttributes, &returnedAttribute)
	}
	return returnedAttributes, nil
}

func (u *attributesClient) Watch(handlers ...storage.AttributeEventHandler) (*storage.Watcher, error) {
	lw := cache.NewListWatchFromClient(u.crds.GlooV1().RESTClient(), crdv1.AttributeCRD.Plural, u.namespace, fields.Everything())
	sw := cache.NewSharedInformer(lw, new(crdv1.Attribute), u.syncFrequency)
	for _, h := range handlers {
		sw.AddEventHandler(&attributeEventHandler{handler: h, store: sw.GetStore()})
	}
	return storage.NewWatcher(func(stop <-chan struct{}, _ chan error) {
		sw.Run(stop)
	}), nil
}

func (c *attributesClient) createOrUpdateAttributeCrd(attribute *v1.Attribute, op crud.Operation) (*v1.Attribute, error) {
	attributeCrd, err := ConfigObjectToCrd(c.namespace, attribute)
	if err != nil {
		return nil, errors.Wrap(err, "converting gloo object to crd")
	}
	attributes := c.crds.GlooV1().Attributes(attributeCrd.GetNamespace())
	var returnedCrd *crdv1.Attribute
	switch op {
	case crud.OperationCreate:
		returnedCrd, err = attributes.Create(attributeCrd.(*crdv1.Attribute))
		if err != nil {
			if kuberrs.IsAlreadyExists(err) {
				return nil, storage.NewAlreadyExistsErr(err)
			}
			return nil, errors.Wrap(err, "kubernetes create api request")
		}
	case crud.OperationUpdate:
		// need to make sure we preserve labels
		currentCrd, err := attributes.Get(attributeCrd.GetName(), metav1.GetOptions{ResourceVersion: attributeCrd.GetResourceVersion()})
		if err != nil {
			return nil, errors.Wrap(err, "kubernetes get api request")
		}
		// copy labels
		attributeCrd.SetLabels(currentCrd.Labels)
		returnedCrd, err = attributes.Update(attributeCrd.(*crdv1.Attribute))
		if err != nil {
			return nil, errors.Wrap(err, "kubernetes update api request")
		}
	}
	var returnedAttribute v1.Attribute
	if err := ConfigObjectFromCrd(
		returnedCrd.ObjectMeta,
		returnedCrd.Spec,
		returnedCrd.Status,
		&returnedAttribute); err != nil {
		return nil, errors.Wrap(err, "converting returned crd to attribute")
	}
	return &returnedAttribute, nil
}

// implements the kubernetes ResourceEventHandler interface
type attributeEventHandler struct {
	handler storage.AttributeEventHandler
	store   cache.Store
}

func (eh *attributeEventHandler) getUpdatedList() []*v1.Attribute {
	updatedList := eh.store.List()
	var updatedAttributeList []*v1.Attribute
	for _, updated := range updatedList {
		attributeCrd, ok := updated.(*crdv1.Attribute)
		if !ok {
			continue
		}
		var returnedAttribute v1.Attribute
		if err := ConfigObjectFromCrd(
			attributeCrd.ObjectMeta,
			attributeCrd.Spec,
			attributeCrd.Status,
			&returnedAttribute); err != nil {
			log.Warnf("watch event: %v", errors.Wrap(err, "converting returned crd to attribute"))
		}
		updatedAttributeList = append(updatedAttributeList, &returnedAttribute)
	}
	return updatedAttributeList
}

func convertAttribute(obj interface{}) (*v1.Attribute, bool) {
	attributeCrd, ok := obj.(*crdv1.Attribute)
	if !ok {
		return nil, ok
	}
	var returnedAttribute v1.Attribute
	if err := ConfigObjectFromCrd(
		attributeCrd.ObjectMeta,
		attributeCrd.Spec,
		attributeCrd.Status,
		&returnedAttribute); err != nil {
		log.Warnf("watch event: %v", errors.Wrap(err, "converting returned crd to attribute"))
		return nil, false
	}
	return &returnedAttribute, true
}

func (eh *attributeEventHandler) OnAdd(obj interface{}) {
	attribute, ok := convertAttribute(obj)
	if !ok {
		return
	}
	eh.handler.OnAdd(eh.getUpdatedList(), attribute)
}
func (eh *attributeEventHandler) OnUpdate(_, newObj interface{}) {
	newAttribute, ok := convertAttribute(newObj)
	if !ok {
		return
	}
	eh.handler.OnUpdate(eh.getUpdatedList(), newAttribute)
}

func (eh *attributeEventHandler) OnDelete(obj interface{}) {
	attribute, ok := convertAttribute(obj)
	if !ok {
		return
	}
	eh.handler.OnDelete(eh.getUpdatedList(), attribute)
}
