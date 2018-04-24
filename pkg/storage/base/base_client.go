package base

import (
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/storage/dependencies"

	"github.com/hashicorp/consul/api"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage"
)

// TODO: evaluate efficiency of LSing a whole dir on every op
// so far this is preferable to caring what files are named
type ConsulStorageClient struct {
	rootPath string
	consul   *api.Client
}

func NewConsulStorageClient(rootPath string, consul *api.Client) *ConsulStorageClient {
	return &ConsulStorageClient{
		rootPath: rootPath,
		consul:   consul,
	}
}

func (c *ConsulStorageClient) Create(item *StorableItem) (*StorableItem, error) {
	p, err := toKVPair(c.rootPath, item)
	if err != nil {
		return nil, errors.Wrapf(err, "converting %s to kv pair", item.GetName())
	}

	// error if the key already exists
	existingP, _, err := c.consul.KV().Get(p.Key, &api.QueryOptions{RequireConsistent: true})
	if err != nil {
		return nil, errors.Wrap(err, "failed to query consul")
	}
	if existingP != nil {
		return nil, storage.NewAlreadyExistsErr(
			errors.Errorf("key found for storageItem %s: %s", item.GetName(), p.Key))
	}

	// create the item
	if _, err := c.consul.KV().Put(p, nil); err != nil {
		return nil, errors.Wrapf(err, "writing kv pair %s", p.Key)
	}
	cfgObject, err := c.Get(item.GetName())
	if err != nil {
		return nil, errors.Wrapf(err, "getting newly created cfg object %s", p.Key)
	}
	return cfgObject, nil
}

func (c *ConsulStorageClient) Update(item *StorableItem) (*StorableItem, error) {
	updatedP, err := toKVPair(c.rootPath, item)
	if err != nil {
		return nil, errors.Wrapf(err, "converting %s to kv pair", item.GetName())
	}

	// error if the key doesn't already exist
	existingP, _, err := c.consul.KV().Get(updatedP.Key, &api.QueryOptions{RequireConsistent: true})
	if err != nil {
		return nil, errors.Wrap(err, "failed to query consul")
	}
	if existingP == nil {
		return nil, errors.Errorf("key not found for storageItem %s: %s", item.GetName(), updatedP.Key)
	}

	// update the item
	if success, _, err := c.consul.KV().CAS(updatedP, nil); err != nil {
		return nil, errors.Wrapf(err, "writing kv pair %s", updatedP.Key)
	} else if !success {
		return nil, errors.Errorf("resource version was invalid for storageItem: %s", item.GetName())
	}

	cfgObject, err := c.Get(item.GetName())
	if err != nil {
		return nil, errors.Wrapf(err, "getting updated created cfg object %s", existingP.Key)
	}
	return cfgObject, nil
}

func (c *ConsulStorageClient) Delete(name string) error {
	key := key(c.rootPath, name)

	_, err := c.consul.KV().Delete(key, nil)
	if err != nil {
		return errors.Wrapf(err, "deleting %s", name)
	}
	return nil
}

func (c *ConsulStorageClient) Get(name string) (*StorableItem, error) {
	key := key(c.rootPath, name)
	p, _, err := c.consul.KV().Get(key, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "getting pair for for key %v", key)
	}
	if p == nil {
		return nil, errors.Errorf("keypair %s not found for storageItem %s", key, name)
	}
	obj, err := itemFromKVPair(c.rootPath, p)
	if err != nil {
		return nil, errors.Wrap(err, "converting consul kv-pair to storageItem")
	}
	return obj, nil
}

func (c *ConsulStorageClient) List() ([]*StorableItem, error) {
	pairs, _, err := c.consul.KV().List(c.rootPath, &api.QueryOptions{RequireConsistent: true})
	if err != nil {
		return nil, errors.Wrapf(err, "listing key-value pairs for root %s", c.rootPath)
	}
	var storageItems []*StorableItem
	for _, p := range pairs {
		obj, err := itemFromKVPair(c.rootPath, p)
		if err != nil {
			return nil, errors.Wrapf(err, "converting %s to storageItem", p.Key)
		}
		storageItems = append(storageItems, obj)
	}
	return storageItems, nil
}

// TODO: be clear that watch for consul only calls update
func (c *ConsulStorageClient) Watch(handlers ...StorableItemEventHandler) (*storage.Watcher, error) {
	var lastIndex uint64
	sync := func() error {
		pairs, meta, err := c.consul.KV().List(c.rootPath, &api.QueryOptions{RequireConsistent: true, WaitIndex: lastIndex})
		if err != nil {
			return errors.Wrap(err, "getting kv-pairs list")
		}
		// no change since last poll
		if lastIndex == meta.LastIndex {
			return nil
		}
		var (
			virtualServices []*v1.VirtualService
			upstreams       []*v1.Upstream
			files           []*dependencies.File
		)
		for _, p := range pairs {
			item, err := itemFromKVPair(c.rootPath, p)
			if err != nil {
				return errors.Wrapf(err, "converting %s to storageItem", p.Key)
			}

			switch {
			case item.Upstream != nil:
				upstreams = append(upstreams, item.Upstream)
			case item.VirtualService != nil:
				virtualServices = append(virtualServices, item.VirtualService)
			case item.File != nil:
				files = append(files, item.File)
			default:
				panic("virtual service, file or upstream must be set")

			}
		}
		// update index
		lastIndex = meta.LastIndex
		switch {
		case len(upstreams) > 0:
			for _, h := range handlers {
				h.UpstreamEventHandler.OnUpdate(upstreams, nil)
			}
		case len(virtualServices) > 0:
			for _, h := range handlers {
				h.VirtualServiceEventHandler.OnUpdate(virtualServices, nil)
			}
		case len(files) > 0:
			for _, h := range handlers {
				h.FileEventHandler.OnUpdate(files, nil)
			}
		}
		return nil
	}
	return storage.NewWatcher(func(stop <-chan struct{}, errs chan error) {
		for {
			select {
			default:
				if err := sync(); err != nil {
					log.Warnf("error syncing with consul kv-pairs: %v", err)
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
