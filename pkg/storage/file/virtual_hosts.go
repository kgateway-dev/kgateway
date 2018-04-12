package file

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/radovskyb/watcher"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage"
)

// TODO: evaluate efficiency of LSing a whole dir on every op
// so far this is preferable to caring what files are named
type virtualHostsClient struct {
	dir           string
	syncFrequency time.Duration
}

func (c *virtualHostsClient) Create(item *v1.VirtualHost) (*v1.VirtualHost, error) {
	// set resourceversion on clone
	virtualHostClone, ok := proto.Clone(item).(*v1.VirtualHost)
	if !ok {
		return nil, errors.New("internal error: output of proto.Clone was not expected type")
	}
	if virtualHostClone.Metadata == nil {
		virtualHostClone.Metadata = &v1.Metadata{}
	}
	virtualHostClone.Metadata.ResourceVersion = newOrIncrementResourceVer(virtualHostClone.Metadata.ResourceVersion)
	virtualHostFiles, err := c.pathsToVirtualHosts()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read virtualHost dir")
	}
	// error if exists already
	for file, existingUps := range virtualHostFiles {
		if existingUps.Name == item.Name {
			return nil, storage.NewAlreadyExistsErr(errors.Errorf("virtualHost %v already defined in %s", item.Name, file))
		}
	}
	filename := filepath.Join(c.dir, item.Name+".yml")
	err = WriteToFile(filename, virtualHostClone)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating file")
	}
	return virtualHostClone, nil
}

func (c *virtualHostsClient) Update(item *v1.VirtualHost) (*v1.VirtualHost, error) {
	if item.Metadata == nil || item.Metadata.ResourceVersion == "" {
		return nil, errors.New("resource version must be set for update operations")
	}
	virtualHostFiles, err := c.pathsToVirtualHosts()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read virtualHost dir")
	}
	// error if exists already
	for file, existingUps := range virtualHostFiles {
		if existingUps.Name != item.Name {
			continue
		}
		if existingUps.Metadata != nil && lessThan(item.Metadata.ResourceVersion, existingUps.Metadata.ResourceVersion) {
			return nil, errors.Errorf("resource version outdated for %v", item.Name)
		}
		virtualHostClone, ok := proto.Clone(item).(*v1.VirtualHost)
		if !ok {
			return nil, errors.New("internal error: output of proto.Clone was not expected type")
		}
		virtualHostClone.Metadata.ResourceVersion = newOrIncrementResourceVer(virtualHostClone.Metadata.ResourceVersion)

		err = WriteToFile(file, virtualHostClone)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating file")
		}

		return virtualHostClone, nil
	}
	return nil, errors.Errorf("virtualHost %v not found", item.Name)
}

func (c *virtualHostsClient) Delete(name string) error {
	virtualHostFiles, err := c.pathsToVirtualHosts()
	if err != nil {
		return errors.Wrap(err, "failed to read virtualHost dir")
	}
	// error if exists already
	for file, existingUps := range virtualHostFiles {
		if existingUps.Name == name {
			return os.Remove(file)
		}
	}
	return errors.Errorf("file not found for virtualHost %v", name)
}

func (c *virtualHostsClient) Get(name string) (*v1.VirtualHost, error) {
	virtualHostFiles, err := c.pathsToVirtualHosts()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read virtualHost dir")
	}
	// error if exists already
	for _, existingUps := range virtualHostFiles {
		if existingUps.Name == name {
			return existingUps, nil
		}
	}
	return nil, errors.Errorf("file not found for virtualHost %v", name)
}

func (c *virtualHostsClient) List() ([]*v1.VirtualHost, error) {
	virtualHostPaths, err := c.pathsToVirtualHosts()
	if err != nil {
		return nil, err
	}
	var virtualHosts []*v1.VirtualHost
	for _, up := range virtualHostPaths {
		virtualHosts = append(virtualHosts, up)
	}
	return virtualHosts, nil
}

func (c *virtualHostsClient) pathsToVirtualHosts() (map[string]*v1.VirtualHost, error) {
	files, err := ioutil.ReadDir(c.dir)
	if err != nil {
		return nil, errors.Wrap(err, "could not read dir")
	}
	virtualHosts := make(map[string]*v1.VirtualHost)
	for _, f := range files {
		path := filepath.Join(c.dir, f.Name())
		if !strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".yaml") {
			continue
		}
		var virtualHost v1.VirtualHost
		err := ReadFileInto(path, &virtualHost)
		if err != nil {
			return nil, errors.Wrap(err, "unable to parse .yml file as virtualHost")
		}
		virtualHosts[path] = &virtualHost
	}
	return virtualHosts, nil
}

func (u *virtualHostsClient) Watch(handlers ...storage.VirtualHostEventHandler) (*storage.Watcher, error) {
	w := watcher.New()
	w.SetMaxEvents(0)
	w.FilterOps(watcher.Create, watcher.Write, watcher.Remove)
	if err := w.AddRecursive(u.dir); err != nil {
		return nil, errors.Wrapf(err, "failed to add directory %v", u.dir)
	}

	return storage.NewWatcher(func(stop <-chan struct{}, errs chan error) {
		go func() {
			if err := w.Start(u.syncFrequency); err != nil {
				errs <- err
			}
		}()
		for {
			select {
			case event := <-w.Event:
				if err := u.onEvent(event, handlers...); err != nil {
					log.Warnf("failed to handle file event: %v", err)
				}
			case err := <-w.Error:
				errs <- err
				return
			case <-stop:
				w.Close()
				return
			}
		}
	}), nil
}

func (u *virtualHostsClient) onEvent(event watcher.Event, handlers ...storage.VirtualHostEventHandler) error {
	log.Debugf("file event: %v [%v]", event.Path, event.Op)
	current, err := u.List()
	if err != nil {
		return err
	}
	if event.IsDir() {
		return nil
	}
	switch event.Op {
	case watcher.Create:
		for _, h := range handlers {
			var created v1.VirtualHost
			err := ReadFileInto(event.Path, &created)
			if err != nil {
				return err
			}
			h.OnAdd(current, &created)
		}
	case watcher.Write:
		for _, h := range handlers {
			var updated v1.VirtualHost
			err := ReadFileInto(event.Path, &updated)
			if err != nil {
				return err
			}
			h.OnUpdate(current, &updated)
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
