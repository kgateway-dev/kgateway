package secretwatcher

import (
	"sync"

	"github.com/d4l3k/messagediff"
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
)

type secretWatcher struct {
	watchers      []*storage.Watcher
	secretRefs    []string
	secrets       chan SecretMap
	secretStorage dependencies.SecretStorage
	lastSeen      SecretMap
	errs          chan error
}

func toMap(list []*dependencies.Secret) SecretMap {
	secrets := make(SecretMap)
	for _, sec := range list {
		secrets[sec.Ref] = sec
	}
	return secrets
}

func filterSecrets(secrets SecretMap, secretRefs []string) SecretMap {
	filtered := make(SecretMap)
	for k, v := range secrets {
		for _, ref := range secretRefs {
			if ref == k {
				filtered[k] = v
			}
		}
	}
	return filtered
}

func NewSecretWatcher(secretClient dependencies.SecretStorage) (*secretWatcher, error) {
	sw := &secretWatcher{
		secrets:       make(chan SecretMap),
		errs:          make(chan error),
		secretStorage: secretClient,
	}

	watcher, err := secretClient.Watch(&dependencies.SecretEventHandlerFuncs{
		AddFunc:    sw.syncSecrets,
		UpdateFunc: sw.syncSecrets,
		DeleteFunc: sw.syncSecrets,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create watcher for secrets")
	}
	sw.watchers = []*storage.Watcher{watcher}

	return sw, nil
}

func (w *secretWatcher) syncSecrets(updatedList []*dependencies.Secret, _ *dependencies.Secret) {
	updatedMap := filterSecrets(toMap(updatedList), w.secretRefs)
	if len(updatedMap) == 0 {
		return
	}
	if _, equal := messagediff.PrettyDiff(w.lastSeen, updatedMap); equal {
		return
	}

	w.lastSeen = updatedMap
	w.secrets <- updatedMap
}

func (w *secretWatcher) Run(stop <-chan struct{}) {
	done := &sync.WaitGroup{}
	for _, watcher := range w.watchers {
		done.Add(1)
		go func(watcher *storage.Watcher, stop <-chan struct{}, errs chan error) {
			watcher.Run(stop, errs)
			done.Done()
		}(watcher, stop, w.errs)
	}
	done.Wait()
}

func (w *secretWatcher) TrackSecrets(secretRefs []string) {
	w.secretRefs = secretRefs
	list, err := w.secretStorage.List()
	if err != nil {
		log.Warnf("failed to get updated secret list: %v", err)
		return
	}
	w.syncSecrets(list, nil)
}

func (w *secretWatcher) Secrets() <-chan SecretMap {
	return w.secrets
}

func (w *secretWatcher) Error() <-chan error {
	return w.errs
}
