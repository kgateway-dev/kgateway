package file

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/radovskyb/watcher"

	"time"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage"
)

// TODO: evaluate efficiency of LSing a whole dir on every op
// so far this is preferable to caring what files are named
type virtualMeshesClient struct {
	dir           string
	syncFrequency time.Duration
}

func (c *virtualMeshesClient) Create(item *v1.VirtualMesh) (*v1.VirtualMesh, error) {
	// set resourceversion on clone
	virtualMeshClone, ok := proto.Clone(item).(*v1.VirtualMesh)
	if !ok {
		return nil, errors.New("internal error: output of proto.Clone was not expected type")
	}
	if virtualMeshClone.Metadata == nil {
		virtualMeshClone.Metadata = &v1.Metadata{}
	}
	virtualMeshClone.Metadata.ResourceVersion = newOrIncrementResourceVer(virtualMeshClone.Metadata.ResourceVersion)
	virtualMeshFiles, err := c.pathsToVirtualMeshes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read virtualMesh dir")
	}
	// error if exists already
	for file, existingUps := range virtualMeshFiles {
		if existingUps.Name == item.Name {
			return nil, storage.NewAlreadyExistsErr(errors.Errorf("virtualMesh %v already defined in %s", item.Name, file))
		}
	}
	filename := filepath.Join(c.dir, item.Name+".yml")
	err = WriteToFile(filename, virtualMeshClone)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating file")
	}
	return virtualMeshClone, nil
}

func (c *virtualMeshesClient) Update(item *v1.VirtualMesh) (*v1.VirtualMesh, error) {
	if item.Metadata == nil || item.Metadata.ResourceVersion == "" {
		return nil, errors.New("resource version must be set for update operations")
	}
	virtualMeshFiles, err := c.pathsToVirtualMeshes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read virtualMesh dir")
	}
	// error if exists already
	for file, existingUps := range virtualMeshFiles {
		if existingUps.Name != item.Name {
			continue
		}
		if existingUps.Metadata != nil && lessThan(item.Metadata.ResourceVersion, existingUps.Metadata.ResourceVersion) {
			return nil, errors.Errorf("resource version outdated for %v", item.Name)
		}
		virtualMeshClone, ok := proto.Clone(item).(*v1.VirtualMesh)
		if !ok {
			return nil, errors.New("internal error: output of proto.Clone was not expected type")
		}
		virtualMeshClone.Metadata.ResourceVersion = newOrIncrementResourceVer(virtualMeshClone.Metadata.ResourceVersion)

		err = WriteToFile(file, virtualMeshClone)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating file")
		}

		return virtualMeshClone, nil
	}
	return nil, errors.Errorf("virtualMesh %v not found", item.Name)
}

func (c *virtualMeshesClient) Delete(name string) error {
	virtualMeshFiles, err := c.pathsToVirtualMeshes()
	if err != nil {
		return errors.Wrap(err, "failed to read virtualMesh dir")
	}
	// error if exists already
	for file, existingUps := range virtualMeshFiles {
		if existingUps.Name == name {
			return os.Remove(file)
		}
	}
	return errors.Errorf("file not found for virtualMesh %v", name)
}

func (c *virtualMeshesClient) Get(name string) (*v1.VirtualMesh, error) {
	virtualMeshFiles, err := c.pathsToVirtualMeshes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read virtualMesh dir")
	}
	// error if exists already
	for _, existingUps := range virtualMeshFiles {
		if existingUps.Name == name {
			return existingUps, nil
		}
	}
	return nil, errors.Errorf("file not found for virtualMesh %v", name)
}

func (c *virtualMeshesClient) List() ([]*v1.VirtualMesh, error) {
	virtualMeshPaths, err := c.pathsToVirtualMeshes()
	if err != nil {
		return nil, err
	}
	var virtualMeshes []*v1.VirtualMesh
	for _, up := range virtualMeshPaths {
		virtualMeshes = append(virtualMeshes, up)
	}
	return virtualMeshes, nil
}

func (c *virtualMeshesClient) pathsToVirtualMeshes() (map[string]*v1.VirtualMesh, error) {
	files, err := ioutil.ReadDir(c.dir)
	if err != nil {
		return nil, errors.Wrap(err, "could not read dir")
	}
	virtualMeshes := make(map[string]*v1.VirtualMesh)
	for _, f := range files {
		path := filepath.Join(c.dir, f.Name())
		if !strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".yaml") {
			continue
		}
		var virtualMesh v1.VirtualMesh
		err := ReadFileInto(path, &virtualMesh)
		if err != nil {
			return nil, errors.Wrap(err, "unable to parse .yml file as virtualMesh")
		}
		virtualMeshes[path] = &virtualMesh
	}
	return virtualMeshes, nil
}

func (u *virtualMeshesClient) Watch(handlers ...storage.VirtualMeshEventHandler) (*storage.Watcher, error) {
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
					log.Warnf("event handle error in file-based config storage client: %v", err)
				}
			case err := <-w.Error:
				log.Warnf("watcher error in file-based config storage client: %v", err)
				return
			case err := <-errs:
				log.Warnf("failed to start file watcher: %v", err)
				return
			case <-stop:
				w.Close()
				return
			}
		}
	}), nil
}

func (u *virtualMeshesClient) onEvent(event watcher.Event, handlers ...storage.VirtualMeshEventHandler) error {
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
			var created v1.VirtualMesh
			err := ReadFileInto(event.Path, &created)
			if err != nil {
				return err
			}
			h.OnAdd(current, &created)
		}
	case watcher.Write:
		for _, h := range handlers {
			var updated v1.VirtualMesh
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
