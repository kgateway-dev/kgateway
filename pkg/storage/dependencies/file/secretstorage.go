package file

import (
	"time"

	"io/ioutil"

	"path/filepath"

	"github.com/pkg/errors"
	"github.com/radovskyb/watcher"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
)

type secretStorage struct {
	dir           string
	syncFrequency time.Duration
}

func NewSecretStorage(dir string, syncFrequency time.Duration) (dependencies.SecretStorage, error) {
	return &secretStorage{
		dir:           dir,
		syncFrequency: syncFrequency,
	}, nil
}

func (s *secretStorage) Create(secret *dependencies.Secret) (*dependencies.Secret, error) {
	if _, err := s.Get(secret.Ref); err == nil {
		return nil, errors.Errorf("secret %v already exists", secret.Ref)
	}
	if err := writeSecret(s.dir, secret); err != nil {
		return nil, errors.Wrap(err, "writing secret")
	}
	return s.Get(secret.Ref)
}

func (s *secretStorage) Update(secret *dependencies.Secret) (*dependencies.Secret, error) {
	if err := writeSecret(s.dir, secret); err != nil {
		return nil, errors.Wrap(err, "writing secret")
	}
	return s.Get(secret.Ref)
}

func (s *secretStorage) Delete(ref string) error {
	err := deleteFile(s.dir, ref)
	if err != nil {
		return errors.Wrap(err, "deleting secret")
	}
	return nil
}

func (s *secretStorage) Get(ref string) (*dependencies.Secret, error) {
	secret, err := readSecret(s.dir, ref)
	if err != nil {
		return nil, errors.Wrap(err, "reading secret")
	}
	return secret, nil
}

func (s *secretStorage) List() ([]*dependencies.Secret, error) {
	osSecrets, err := ioutil.ReadDir(s.dir)
	if err != nil {
		return nil, errors.Wrap(err, "could not read dir")
	}
	var secrets []*dependencies.Secret
	for _, f := range osSecrets {
		secret, err := s.Get(f.Name())
		if err != nil {
			return nil, errors.Wrapf(err, "getting secret %v", f.Name())
		}
		secrets = append(secrets, secret)
	}
	return secrets, nil
}

func (s *secretStorage) Watch(handlers ...dependencies.SecretEventHandler) (*storage.Watcher, error) {
	w := watcher.New()
	w.SetMaxEvents(0)
	w.FilterOps(watcher.Create, watcher.Write, watcher.Remove)
	if err := w.AddRecursive(s.dir); err != nil {
		return nil, errors.Wrapf(err, "failed to add directory %v", s.dir)
	}

	return storage.NewWatcher(func(stop <-chan struct{}, errs chan error) {
		go func() {
			if err := w.Start(s.syncFrequency); err != nil {
				errs <- err
			}
		}()
		for {
			select {
			case event := <-w.Event:
				if err := s.onEvent(event, handlers...); err != nil {
					log.Warnf("watcher encoutnered error: %v", err)
				}
			case err := <-w.Error:
				log.Warnf("watcher encoutnered error: %v", err)
				return
			case err := <-errs:
				log.Warnf("failed to start watcher to: %v", err)
				return
			case <-stop:
				w.Close()
				return
			}
		}
	}), nil
}

func (s *secretStorage) onEvent(event watcher.Event, handlers ...dependencies.SecretEventHandler) error {
	log.Debugf("secret event: %v [%v]", event.Path, event.Op)
	current, err := s.List()
	if err != nil {
		return err
	}
	if event.IsDir() {
		return nil
	}
	switch event.Op {
	case watcher.Create:
		for _, h := range handlers {
			created, err := readSecret(s.dir, filepath.Base(event.Path))
			if err != nil {
				return err
			}
			h.OnAdd(current, created)
		}
	case watcher.Write:
		for _, h := range handlers {
			updated, err := readSecret(s.dir, filepath.Base(event.Path))
			if err != nil {
				return err
			}
			h.OnUpdate(current, updated)
		}
	case watcher.Remove:
		for _, h := range handlers {
			// can't read the deleted object
			// callers beware
			h.OnDelete(current, nil)
		}
	}
	return nil
}
