package configstorage

import (
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/consul"
	"github.com/solo-io/gloo/pkg/storage/crd"
	"github.com/solo-io/gloo/pkg/storage/file"
	kubeutils "github.com/solo-io/gloo/pkg/utils/kube"
)

func Bootstrap(opts bootstrap.Options) (storage.Interface, error) {
	switch opts.ConfigStorageOptions.Type {
	case bootstrap.WatcherTypeFile:
		dir := opts.FileOptions.ConfigDir
		if dir == "" {
			return nil, errors.New("must provide directory for file config watcher")
		}
		client, err := file.NewStorage(dir, opts.ConfigStorageOptions.SyncFrequency)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start file config watcher for directory %v", dir)
		}
		return client, nil
	case bootstrap.WatcherTypeKube:
		cfg, err := kubeutils.GetConfig(opts.KubeOptions.MasterURL, opts.KubeOptions.KubeConfig)
		if err != nil {
			return nil, errors.Wrap(err, "building kube restclient")
		}
		cfgWatcher, err := crd.NewStorage(cfg, opts.KubeOptions.Namespace, opts.ConfigStorageOptions.SyncFrequency)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start kube config watcher with config %#v", opts.KubeOptions)
		}
		return cfgWatcher, nil
	case bootstrap.WatcherTypeConsul:
		cfg := opts.ConsulOptions.ToConsulConfig()
		cfgWatcher, err := consul.NewStorage(cfg, opts.ConsulOptions.RootPath, opts.ConfigStorageOptions.SyncFrequency)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to start consul config watcher with config %#v", opts.ConsulOptions)
		}
		return cfgWatcher, nil
	}
	return nil, errors.Errorf("unknown or unspecified config watcher type: %v", opts.ConfigStorageOptions.Type)
}
