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
type virtualServicesClient struct {
	dir           string
	syncFrequency time.Duration
}

func (c *virtualServicesClient) Create(item *v1.VirtualService) (*v1.VirtualService, error) {
	// set resourceversion on clone
	virtualServiceClone, ok := proto.Clone(item).(*v1.VirtualService)
	if !ok {
		return nil, errors.New("internal error: output of proto.Clone was not expected type")
	}
	if virtualServiceClone.Metadata == nil {
		virtualServiceClone.Metadata = &v1.Metadata{}
	}
	virtualServiceClone.Metadata.ResourceVersion = newOrIncrementResourceVer(virtualServiceClone.Metadata.ResourceVersion)
	virtualServiceFiles, err := c.pathsToVirtualServices()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read virtualService dir")
	}
	// error if exists already
	for file, existingUps := range virtualServiceFiles {
		if existingUps.Name == item.Name {
			return nil, storage.NewAlreadyExistsErr(errors.Errorf("virtualService %v already defined in %s", item.Name, file))
		}
	}
	filename := filepath.Join(c.dir, item.Name+".yml")
	err = WriteToFile(filename, virtualServiceClone)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating file")
	}
	return virtualServiceClone, nil
}

func (c *virtualServicesClient) Update(item *v1.VirtualService) (*v1.VirtualService, error) {
	if item.Metadata == nil || item.Metadata.ResourceVersion == "" {
		return nil, errors.New("resource version must be set for update operations")
	}
	virtualServiceFiles, err := c.pathsToVirtualServices()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read virtualService dir")
	}
	// error if exists already
	for file, existingUps := range virtualServiceFiles {
		if existingUps.Name != item.Name {
			continue
		}
		if existingUps.Metadata != nil && lessThan(item.Metadata.ResourceVersion, existingUps.Metadata.ResourceVersion) {
			return nil, errors.Errorf("resource version outdated for %v", item.Name)
		}
		virtualServiceClone, ok := proto.Clone(item).(*v1.VirtualService)
		if !ok {
			return nil, errors.New("internal error: output of proto.Clone was not expected type")
		}
		virtualServiceClone.Metadata.ResourceVersion = newOrIncrementResourceVer(virtualServiceClone.Metadata.ResourceVersion)

		err = WriteToFile(file, virtualServiceClone)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating file")
		}

		return virtualServiceClone, nil
	}
	return nil, errors.Errorf("virtualService %v not found", item.Name)
}

func (c *virtualServicesClient) Delete(name string) error {
	virtualServiceFiles, err := c.pathsToVirtualServices()
	if err != nil {
		return errors.Wrap(err, "failed to read virtualService dir")
	}
	// error if exists already
	for file, existingUps := range virtualServiceFiles {
		if existingUps.Name == name {
			return os.Remove(file)
		}
	}
	return errors.Errorf("file not found for virtualService %v", name)
}

func (c *virtualServicesClient) Get(name string) (*v1.VirtualService, error) {
	virtualServiceFiles, err := c.pathsToVirtualServices()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read virtualService dir")
	}
	// error if exists already
	for _, existingUps := range virtualServiceFiles {
		if existingUps.Name == name {
			return existingUps, nil
		}
	}
	return nil, errors.Errorf("file not found for virtualService %v", name)
}

func (c *virtualServicesClient) List() ([]*v1.VirtualService, error) {
	virtualServicePaths, err := c.pathsToVirtualServices()
	if err != nil {
		return nil, err
	}
	var virtualServices []*v1.VirtualService
	for _, up := range virtualServicePaths {
		virtualServices = append(virtualServices, up)
	}
	return virtualServices, nil
}

func (c *virtualServicesClient) pathsToVirtualServices() (map[string]*v1.VirtualService, error) {
	files, err := ioutil.ReadDir(c.dir)
	if err != nil {
		return nil, errors.Wrap(err, "could not read dir")
	}
	virtualServices := make(map[string]*v1.VirtualService)
	for _, f := range files {
		path := filepath.Join(c.dir, f.Name())
		if !strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".yaml") {
			continue
		}
		var virtualService v1.VirtualService
		err := ReadFileInto(path, &virtualService)
		if err != nil {
			return nil, errors.Wrap(err, "unable to parse .yml file as virtualService")
		}
		virtualServices[path] = &virtualService
	}
	return virtualServices, nil
}

func (u *virtualServicesClient) Watch(handlers ...storage.VirtualServiceEventHandler) (*storage.Watcher, error) {
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
		// start the watch with an "initial read" event
		current, err := u.List()
		if err != nil {
			errs <- err
			return
		}
		for _, h := range handlers {
			h.OnAdd(current, nil)
		}
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

func (u *virtualServicesClient) onEvent(event watcher.Event, handlers ...storage.VirtualServiceEventHandler) error {
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
			var created v1.VirtualService
			err := ReadFileInto(event.Path, &created)
			if err != nil {
				return err
			}
			h.OnAdd(current, &created)
		}
	case watcher.Write:
		for _, h := range handlers {
			var updated v1.VirtualService
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
