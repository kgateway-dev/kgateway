package vault

import (
	"time"

	"github.com/d4l3k/messagediff"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	"reflect"
	"sort"
	"strings"
)

type secretStorage struct {
	vault         *vaultapi.Client
	rootPath      string
	syncFrequency time.Duration
}

const secretPrefix = "/secret/"

func NewSecretStorage(vault *vaultapi.Client, rootPath string, syncFrequency time.Duration) dependencies.SecretStorage {
	if !strings.HasPrefix(rootPath, secretPrefix) {
		rootPath = secretPrefix + rootPath
	}
	return &secretStorage{
		vault:         vault,
		rootPath:      rootPath,
		syncFrequency: syncFrequency,
	}
}

func (s *secretStorage) fullPath(ref string) string {
	return s.rootPath + "/" + ref
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
	var lastSeen []*dependencies.Secret
	sync := func() error {
		list, err := s.List()
		if err != nil {
			return errors.Wrap(err, "listing secrets")
		}
		sort.SliceStable(list, func(i, j int) bool {
			return list[i].Ref < list[j].Ref
		})

		// no change since last poll
		if _, equal := messagediff.PrettyDiff(lastSeen, list); equal {
			return nil
		}

		// update index
		lastSeen = list
		for _, h := range handlers {
			h.OnUpdate(list, nil)
		}
		return nil
	}
	return storage.NewWatcher(func(stop <-chan struct{}, errs chan error) {
		for {
			select {
			default:
				if err := sync(); err != nil {
					log.Warnf("error syncing with vault: %v", err)
				}
			case err := <-errs:
				log.Warnf("failed to start watcher to: %v", err)
				return
			case <-stop:
				return
			}
		}
	}), nil
}
