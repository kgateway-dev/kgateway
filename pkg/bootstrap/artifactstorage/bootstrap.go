package artifactstorage

import (
	"os"

	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/storage/crd"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
	consulfiles "github.com/solo-io/gloo/pkg/storage/dependencies/consul"
	filestorage "github.com/solo-io/gloo/pkg/storage/dependencies/file"
	"github.com/solo-io/gloo/pkg/storage/dependencies/kube"
)

func Bootstrap(opts bootstrap.Options) (dependencies.FileStorage, error) {
	switch opts.FileStorageOptions.Type {
	case bootstrap.WatcherTypeFile:
		dir := opts.FileOptions.FilesDir
		if dir == "" {
			return nil, errors.New("must provide directory for file file storage client")
		}
		err := os.MkdirAll(dir, 0755)
		if err != nil && err != os.ErrExist {
			return nil, errors.Wrap(err, "creating files dir")
		}
		store, err := filestorage.NewFileStorage(dir, opts.FileStorageOptions.SyncFrequency)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start file based file storage client for directory %v", dir)
		}
		return store, nil
	case bootstrap.WatcherTypeKube:
		cfg, err := crd.GetConfig(opts.KubeOptions.MasterURL, opts.KubeOptions.KubeConfig)
		if err != nil {
			return nil, errors.Wrap(err, "building kube restclient")
		}
		store, err := kube.NewFileStorage(cfg, opts.KubeOptions.Namespace, opts.FileStorageOptions.SyncFrequency)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start kube file storage client with config %#v", opts.KubeOptions)
		}
		return store, nil
	case bootstrap.WatcherTypeConsul:
		cfg := opts.ConsulOptions.ToConsulConfig()
		store, err := consulfiles.NewFileStorage(cfg, opts.ConsulOptions.RootPath, opts.ConfigStorageOptions.SyncFrequency)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start consul KV-based file storage client with config %#v", opts.ConsulOptions)
		}
		return store, nil
	}
	return nil, errors.Errorf("unknown or unspecified file storage client type: %v", opts.FileStorageOptions.Type)
}
