package vault

import (
	"time"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	"reflect"
	"strings"
)

type secretStorage struct {
	vault         *vaultapi.Client
	rootPath      string
	syncFrequency time.Duration
}

const secretPrefix = "/secret/"

func NewSecretStorage(vault *vaultapi.Client, rootPath string, syncFrequency time.Duration) dependencies.SecretStorage {
	return &secretStorage{
		vault:         vault,
		rootPath:      rootPath,
		syncFrequency: syncFrequency,
	}
}

func (s *secretStorage) fullPath(ref string) string {
	if strings.HasPrefix(s.rootPath, secretPrefix) {
		return s.rootPath + "/" + ref
	}
	return secretPrefix + s.rootPath + "/" + ref
}

func (s *secretStorage) Create(secret *dependencies.Secret) (*dependencies.Secret, error) {
	if _, err := s.Get(secret.Ref); err == nil {
		return nil, storage.NewAlreadyExistsErr(errors.Errorf("secret %v already exists", secret.Ref))
	}
	return s.Update(secret)
}

func (s *secretStorage) Update(secret *dependencies.Secret) (*dependencies.Secret, error) {
	_, err := s.vault.Logical().Write(s.fullPath(secret.Ref), toInterfaceMap(secret.Data))
	if err != nil {
		return nil, errors.Wrap(err, "vault api call")
	}
	return s.Get(secret.Ref)
}

func (s *secretStorage) Delete(ref string) error {
	if _, err := s.vault.Logical().Delete(s.fullPath(ref)); err != nil {
		return errors.Wrap(err, "vault api call")
	}
	return nil
}

func (s *secretStorage) Get(ref string) (*dependencies.Secret, error) {
	vaultSecret, err := s.vault.Logical().Read(s.fullPath(ref))
	if err != nil {
		return nil, errors.Wrap(err, "vault api call")
	}
	if vaultSecret == nil {
		return nil, errors.Errorf("secret with path %s not found", s.fullPath(ref))
	}
	return vaultSecretToSecret(ref, vaultSecret)
}

func (s *secretStorage) List() ([]*dependencies.Secret, error) {
	var secrets []*dependencies.Secret
	vaultSecretList, err := s.vault.Logical().List(s.rootPath)
	if err != nil {
		return nil, errors.Wrap(err, "vault api call")
	}
	// empty list
	if vaultSecretList == nil {
		return nil, nil
	}
	val, ok := vaultSecretList.Data["keys"]
	if !ok {
		return nil, errors.Errorf("vault secret list at root %s did not contain key \"keys\"", s.rootPath)
	}
	keys, ok := val.([]interface{})
	if !ok {
		return nil, errors.Errorf("expected secret list of type []interface{} but got %v", reflect.TypeOf(val))
	}
	for _, keyAsInterface := range keys {
		key, ok := keyAsInterface.(string)
		if !ok {
			return nil, errors.Errorf("expected key of type string but got %v", reflect.TypeOf(keyAsInterface))
		}
		secret, err := s.Get(key)
		if err != nil {
			return nil, errors.Wrapf(err, "getting secret %s", key)
		}
		secrets = append(secrets, secret)
	}
	return secrets, nil
}

func (s *secretStorage) Watch(handlers ...dependencies.SecretEventHandler) (*storage.Watcher, error) {

	return storage.NewWatcher(func(stop <-chan struct{}, _ chan error) {
		//sw.Run(stop)
	}), nil
}

//// implements the vaultrnetes ResourceEventHandler interface
//type secretEventHandler struct {
//	handler dependencies.SecretEventHandler
//	store   cache.Store
//}
//
//func secretSecret(obj interface{}) (*dependencies.Secret, bool) {
//	vaultSecret, ok := obj.(*v1.Secret)
//	if !ok {
//		return nil, ok
//	}
//	return vaultSecretToSecret(vaultSecret), ok
//}
//
//func (eh *secretEventHandler) getUpdatedList() []*dependencies.Secret {
//	updatedList := eh.store.List()
//	var updatedSecretList []*dependencies.Secret
//	for _, updated := range updatedList {
//		vaultSecret, ok := updated.(*v1.Secret)
//		if !ok {
//			continue
//		}
//		updatedSecretList = append(updatedSecretList, vaultSecretToSecret(vaultSecret))
//	}
//	return updatedSecretList
//}
//
//func (eh *secretEventHandler) OnAdd(obj interface{}) {
//	secret, ok := secretSecret(obj)
//	if !ok {
//		return
//	}
//	eh.handler.OnAdd(eh.getUpdatedList(), secret)
//}
//func (eh *secretEventHandler) OnUpdate(_, newObj interface{}) {
//	secret, ok := secretSecret(newObj)
//	if !ok {
//		return
//	}
//	eh.handler.OnUpdate(eh.getUpdatedList(), secret)
//}
//
//func (eh *secretEventHandler) OnDelete(obj interface{}) {
//	secret, ok := secretSecret(obj)
//	if !ok {
//		return
//	}
//	eh.handler.OnDelete(eh.getUpdatedList(), secret)
//}
