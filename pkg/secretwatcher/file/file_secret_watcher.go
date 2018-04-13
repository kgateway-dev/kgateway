package file

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/util/runtime"

	"github.com/ghodss/yaml"
	"github.com/mitchellh/hashstructure"

	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/secretwatcher"
)

// FileWatcher uses .yml files in a directory
// to watch secrets
type fileWatcher struct {
	dir            string
	secretsToWatch atomic.Value // This contains []string
	secrets        chan secretwatcher.SecretMap
	errors         chan error
	lastSeen       uint64
}

func NewSecretWatcher(dir string, syncFrequency time.Duration) (*fileWatcher, error) {
	os.MkdirAll(dir, 0755)
	secrets := make(chan secretwatcher.SecretMap)
	errors := make(chan error)
	fw := &fileWatcher{
		secrets: secrets,
		errors:  errors,
		dir:     dir,
	}
	if err := WatchDir(dir, false, func(_ string) {
		fw.updateSecrets()
	}, syncFrequency); err != nil {
		return nil, fmt.Errorf("failed to start filewatcher: %v", err)
	}

	// do one on start
	go fw.updateSecrets()

	return fw, nil
}

func (fw *fileWatcher) updateSecrets() {
	secretMap, err := fw.getSecrets()
	if err != nil {
		fw.errors <- err
		return
	}
	// ignore empty configs / no secrets to watch
	if len(secretMap) == 0 {
		return
	}
	fw.secrets <- secretMap
}

// triggers an update
func (fw *fileWatcher) TrackSecrets(secretRefs []string) {
	fw.secretsToWatch.Store(secretRefs)
	fw.updateSecrets()
}

func (fw *fileWatcher) Secrets() <-chan secretwatcher.SecretMap {
	return fw.secrets
}

func (fw *fileWatcher) Error() <-chan error {
	return fw.errors
}

func (fw *fileWatcher) getSecrets() (secretwatcher.SecretMap, error) {
	desiredSecrets := make(secretwatcher.SecretMap)
	// ref should be the filename
	secretsToWatch := fw.secretsToWatch.Load()
	if secretsToWatch != nil {
		for _, ref := range secretsToWatch.([]string) {
			yml, err := ioutil.ReadFile(filepath.Join(fw.dir, ref))
			if err != nil {
				return nil, errors.Wrapf(err, "reading file: %v", filepath.Join(fw.dir, ref))
			}
			var contents map[string]string
			err = yaml.Unmarshal(yml, &contents)
			if err != nil {
				return nil, errors.Wrap(err, "unmarshalling yaml")
			}
			desiredSecrets[ref] = contents
		}
	}

	hash, err := hashstructure.Hash(desiredSecrets, nil)
	if err != nil {
		runtime.HandleError(err)
		return nil, nil
	}

	old := atomic.LoadUint64(&fw.lastSeen)
	if old == hash {
		return nil, nil
	}
	// TODO(yuval-k): do anything if we lost the race?
	atomic.CompareAndSwapUint64(&fw.lastSeen, old, hash)

	return desiredSecrets, nil
}
