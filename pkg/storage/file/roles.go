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
type rolesClient struct {
	dir           string
	syncFrequency time.Duration
}

func (c *rolesClient) Create(item *v1.Role) (*v1.Role, error) {
	// set resourceversion on clone
	roleClone, ok := proto.Clone(item).(*v1.Role)
	if !ok {
		return nil, errors.New("internal error: output of proto.Clone was not expected type")
	}
	if roleClone.Metadata == nil {
		roleClone.Metadata = &v1.Metadata{}
	}
	roleClone.Metadata.ResourceVersion = newOrIncrementResourceVer(roleClone.Metadata.ResourceVersion)
	roleFiles, err := c.pathsToRoles()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read role dir")
	}
	// error if exists already
	for file, existingUps := range roleFiles {
		if existingUps.Name == item.Name {
			return nil, storage.NewAlreadyExistsErr(errors.Errorf("role %v already defined in %s", item.Name, file))
		}
	}
	filename := filepath.Join(c.dir, item.Name+".yml")
	err = WriteToFile(filename, roleClone)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating file")
	}
	return roleClone, nil
}

func (c *rolesClient) Update(item *v1.Role) (*v1.Role, error) {
	if item.Metadata == nil || item.Metadata.ResourceVersion == "" {
		return nil, errors.New("resource version must be set for update operations")
	}
	roleFiles, err := c.pathsToRoles()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read role dir")
	}
	// error if exists already
	for file, existingUps := range roleFiles {
		if existingUps.Name != item.Name {
			continue
		}
		if existingUps.Metadata != nil && lessThan(item.Metadata.ResourceVersion, existingUps.Metadata.ResourceVersion) {
			return nil, errors.Errorf("resource version outdated for %v", item.Name)
		}
		roleClone, ok := proto.Clone(item).(*v1.Role)
		if !ok {
			return nil, errors.New("internal error: output of proto.Clone was not expected type")
		}
		roleClone.Metadata.ResourceVersion = newOrIncrementResourceVer(roleClone.Metadata.ResourceVersion)

		err = WriteToFile(file, roleClone)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating file")
		}

		return roleClone, nil
	}
	return nil, errors.Errorf("role %v not found", item.Name)
}

func (c *rolesClient) Delete(name string) error {
	roleFiles, err := c.pathsToRoles()
	if err != nil {
		return errors.Wrap(err, "failed to read role dir")
	}
	// error if exists already
	for file, existingUps := range roleFiles {
		if existingUps.Name == name {
			return os.Remove(file)
		}
	}
	return errors.Errorf("file not found for role %v", name)
}

func (c *rolesClient) Get(name string) (*v1.Role, error) {
	roleFiles, err := c.pathsToRoles()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read role dir")
	}
	// error if exists already
	for _, existingUps := range roleFiles {
		if existingUps.Name == name {
			return existingUps, nil
		}
	}
	return nil, errors.Errorf("file not found for role %v", name)
}

func (c *rolesClient) List() ([]*v1.Role, error) {
	rolePaths, err := c.pathsToRoles()
	if err != nil {
		return nil, err
	}
	var roles []*v1.Role
	for _, up := range rolePaths {
		roles = append(roles, up)
	}
	return roles, nil
}

func (c *rolesClient) pathsToRoles() (map[string]*v1.Role, error) {
	files, err := ioutil.ReadDir(c.dir)
	if err != nil {
		return nil, errors.Wrap(err, "could not read dir")
	}
	roles := make(map[string]*v1.Role)
	for _, f := range files {
		path := filepath.Join(c.dir, f.Name())
		if !strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".yaml") {
			continue
		}
		var role v1.Role
		err := ReadFileInto(path, &role)
		if err != nil {
			return nil, errors.Wrap(err, "unable to parse .yml file as role")
		}
		roles[path] = &role
	}
	return roles, nil
}

func (u *rolesClient) Watch(handlers ...storage.RoleEventHandler) (*storage.Watcher, error) {
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

func (u *rolesClient) onEvent(event watcher.Event, handlers ...storage.RoleEventHandler) error {
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
			var created v1.Role
			err := ReadFileInto(event.Path, &created)
			if err != nil {
				return err
			}
			h.OnAdd(current, &created)
		}
	case watcher.Write:
		for _, h := range handlers {
			var updated v1.Role
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
