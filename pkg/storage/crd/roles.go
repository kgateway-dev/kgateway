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

type rolesClient struct {
	crds    crdclientset.Interface
	apiexts apiexts.Interface
	// write and read objects to this namespace if not specified on the GlooObjects
	namespace     string
	syncFrequency time.Duration
}

func (c *rolesClient) Create(item *v1.Role) (*v1.Role, error) {
	return c.createOrUpdateRoleCrd(item, crud.OperationCreate)
}

func (c *rolesClient) Update(item *v1.Role) (*v1.Role, error) {
	return c.createOrUpdateRoleCrd(item, crud.OperationUpdate)
}

func (c *rolesClient) Delete(name string) error {
	return c.crds.GlooV1().Roles(c.namespace).Delete(name, nil)
}

func (c *rolesClient) Get(name string) (*v1.Role, error) {
	crdRole, err := c.crds.GlooV1().Roles(c.namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed performing get api request")
	}
	var returnedRole v1.Role
	if err := ConfigObjectFromCrd(
		crdRole.ObjectMeta,
		crdRole.Spec,
		crdRole.Status,
		&returnedRole); err != nil {
		return nil, errors.Wrap(err, "converting returned crd to role")
	}
	return &returnedRole, nil
}

func (c *rolesClient) List() ([]*v1.Role, error) {
	crdList, err := c.crds.GlooV1().Roles(c.namespace).List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed performing list api request")
	}
	var returnedRoles []*v1.Role
	for _, crdRole := range crdList.Items {
		var returnedRole v1.Role
		if err := ConfigObjectFromCrd(
			crdRole.ObjectMeta,
			crdRole.Spec,
			crdRole.Status,
			&returnedRole); err != nil {
			return nil, errors.Wrap(err, "converting returned crd to role")
		}
		returnedRoles = append(returnedRoles, &returnedRole)
	}
	return returnedRoles, nil
}

func (u *rolesClient) Watch(handlers ...storage.RoleEventHandler) (*storage.Watcher, error) {
	lw := cache.NewListWatchFromClient(u.crds.GlooV1().RESTClient(), crdv1.RoleCRD.Plural, u.namespace, fields.Everything())
	sw := cache.NewSharedInformer(lw, new(crdv1.Role), u.syncFrequency)
	for _, h := range handlers {
		sw.AddEventHandler(&roleEventHandler{handler: h, store: sw.GetStore()})
	}
	return storage.NewWatcher(func(stop <-chan struct{}, _ chan error) {
		sw.Run(stop)
	}), nil
}

func (c *rolesClient) createOrUpdateRoleCrd(role *v1.Role, op crud.Operation) (*v1.Role, error) {
	roleCrd, err := ConfigObjectToCrd(c.namespace, role)
	if err != nil {
		return nil, errors.Wrap(err, "converting gloo object to crd")
	}
	roles := c.crds.GlooV1().Roles(roleCrd.GetNamespace())
	var returnedCrd *crdv1.Role
	switch op {
	case crud.OperationCreate:
		returnedCrd, err = roles.Create(roleCrd.(*crdv1.Role))
		if err != nil {
			if kuberrs.IsAlreadyExists(err) {
				return nil, storage.NewAlreadyExistsErr(err)
			}
			return nil, errors.Wrap(err, "kubernetes create api request")
		}
	case crud.OperationUpdate:
		// need to make sure we preserve labels
		currentCrd, err := roles.Get(roleCrd.GetName(), metav1.GetOptions{ResourceVersion: roleCrd.GetResourceVersion()})
		if err != nil {
			return nil, errors.Wrap(err, "kubernetes get api request")
		}
		// copy labels
		roleCrd.SetLabels(currentCrd.Labels)
		returnedCrd, err = roles.Update(roleCrd.(*crdv1.Role))
		if err != nil {
			return nil, errors.Wrap(err, "kubernetes update api request")
		}
	}
	var returnedRole v1.Role
	if err := ConfigObjectFromCrd(
		returnedCrd.ObjectMeta,
		returnedCrd.Spec,
		returnedCrd.Status,
		&returnedRole); err != nil {
		return nil, errors.Wrap(err, "converting returned crd to role")
	}
	return &returnedRole, nil
}

// implements the kubernetes ResourceEventHandler interface
type roleEventHandler struct {
	handler storage.RoleEventHandler
	store   cache.Store
}

func (eh *roleEventHandler) getUpdatedList() []*v1.Role {
	updatedList := eh.store.List()
	var updatedRoleList []*v1.Role
	for _, updated := range updatedList {
		roleCrd, ok := updated.(*crdv1.Role)
		if !ok {
			continue
		}
		var returnedRole v1.Role
		if err := ConfigObjectFromCrd(
			roleCrd.ObjectMeta,
			roleCrd.Spec,
			roleCrd.Status,
			&returnedRole); err != nil {
			log.Warnf("watch event: %v", errors.Wrap(err, "converting returned crd to role"))
		}
		updatedRoleList = append(updatedRoleList, &returnedRole)
	}
	return updatedRoleList
}

func convertRole(obj interface{}) (*v1.Role, bool) {
	roleCrd, ok := obj.(*crdv1.Role)
	if !ok {
		return nil, ok
	}
	var returnedRole v1.Role
	if err := ConfigObjectFromCrd(
		roleCrd.ObjectMeta,
		roleCrd.Spec,
		roleCrd.Status,
		&returnedRole); err != nil {
		log.Warnf("watch event: %v", errors.Wrap(err, "converting returned crd to role"))
		return nil, false
	}
	return &returnedRole, true
}

func (eh *roleEventHandler) OnAdd(obj interface{}) {
	role, ok := convertRole(obj)
	if !ok {
		return
	}
	eh.handler.OnAdd(eh.getUpdatedList(), role)
}
func (eh *roleEventHandler) OnUpdate(_, newObj interface{}) {
	newRole, ok := convertRole(newObj)
	if !ok {
		return
	}
	eh.handler.OnUpdate(eh.getUpdatedList(), newRole)
}

func (eh *roleEventHandler) OnDelete(obj interface{}) {
	role, ok := convertRole(obj)
	if !ok {
		return
	}
	eh.handler.OnDelete(eh.getUpdatedList(), role)
}
