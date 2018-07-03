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
type attributesClient struct {
	dir           string
	syncFrequency time.Duration
}

func (c *attributesClient) Create(item *v1.Attribute) (*v1.Attribute, error) {
	if item.Name == "" {
		return nil, errors.Errorf("name required")
	}
	// set resourceversion on clone
	attributeClone, ok := proto.Clone(item).(*v1.Attribute)
	if !ok {
		return nil, errors.New("internal error: output of proto.Clone was not expected type")
	}
	if attributeClone.Metadata == nil {
		attributeClone.Metadata = &v1.Metadata{}
	}
	attributeClone.Metadata.ResourceVersion = newOrIncrementResourceVer(attributeClone.Metadata.ResourceVersion)
	attributeFiles, err := c.pathsToAttributes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read attribute dir")
	}
	// error if exists already
	for file, existingUps := range attributeFiles {
		if existingUps.Name == item.Name {
			return nil, storage.NewAlreadyExistsErr(errors.Errorf("attribute %v already defined in %s", item.Name, file))
		}
	}
	filename := filepath.Join(c.dir, item.Name+".yml")
	err = WriteToFile(filename, attributeClone)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating file")
	}
	return attributeClone, nil
}

func (c *attributesClient) Update(item *v1.Attribute) (*v1.Attribute, error) {
	if item.Name == "" {
		return nil, errors.Errorf("name required")
	}
	if item.Metadata == nil || item.Metadata.ResourceVersion == "" {
		return nil, errors.New("resource version must be set for update operations")
	}
	attributeFiles, err := c.pathsToAttributes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read attribute dir")
	}
	// error if exists already
	for file, existingUps := range attributeFiles {
		if existingUps.Name != item.Name {
			continue
		}
		if existingUps.Metadata != nil && lessThan(item.Metadata.ResourceVersion, existingUps.Metadata.ResourceVersion) {
			return nil, errors.Errorf("resource version outdated for %v", item.Name)
		}
		attributeClone, ok := proto.Clone(item).(*v1.Attribute)
		if !ok {
			return nil, errors.New("internal error: output of proto.Clone was not expected type")
		}
		attributeClone.Metadata.ResourceVersion = newOrIncrementResourceVer(attributeClone.Metadata.ResourceVersion)

		err = WriteToFile(file, attributeClone)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating file")
		}

		return attributeClone, nil
	}
	return nil, errors.Errorf("attribute %v not found", item.Name)
}

func (c *attributesClient) Delete(name string) error {
	attributeFiles, err := c.pathsToAttributes()
	if err != nil {
		return errors.Wrap(err, "failed to read attribute dir")
	}
	// error if exists already
	for file, existingUps := range attributeFiles {
		if existingUps.Name == name {
			return os.Remove(file)
		}
	}
	return errors.Errorf("file not found for attribute %v", name)
}

func (c *attributesClient) Get(name string) (*v1.Attribute, error) {
	attributeFiles, err := c.pathsToAttributes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read attribute dir")
	}
	// error if exists already
	for _, existingUps := range attributeFiles {
		if existingUps.Name == name {
			return existingUps, nil
		}
	}
	return nil, errors.Errorf("file not found for attribute %v", name)
}

func (c *attributesClient) List() ([]*v1.Attribute, error) {
	attributePaths, err := c.pathsToAttributes()
	if err != nil {
		return nil, err
	}
	var attributes []*v1.Attribute
	for _, up := range attributePaths {
		attributes = append(attributes, up)
	}
	return attributes, nil
}

func (c *attributesClient) pathsToAttributes() (map[string]*v1.Attribute, error) {
	files, err := ioutil.ReadDir(c.dir)
	if err != nil {
		return nil, errors.Wrap(err, "could not read dir")
	}
	attributes := make(map[string]*v1.Attribute)
	for _, f := range files {
		path := filepath.Join(c.dir, f.Name())
		if !strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".yaml") {
			continue
		}

		attribute, err := pathToAttribute(path)
        if err != nil {
            return nil, errors.Wrap(err, "unable to parse .yml file as attribute")
        }

        attributes[path] = attribute
	}
	return attributes, nil
}

func pathToAttribute(path string) (*v1.Attribute, error) {
	var attribute v1.Attribute
	err := ReadFileInto(path, &attribute)
	if err != nil {
		return nil, err
	}
	if attribute.Metadata == nil {
		attribute.Metadata = &v1.Metadata{}
	}
	if attribute.Metadata.ResourceVersion == "" {
		attribute.Metadata.ResourceVersion = "1"
	}
	return &attribute, nil
}

func (u *attributesClient) Watch(handlers ...storage.AttributeEventHandler) (*storage.Watcher, error) {
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

func (u *attributesClient) onEvent(event watcher.Event, handlers ...storage.AttributeEventHandler) error {
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
			created, err := pathToAttribute(event.Path)
			if err != nil {
				return err
			}
			h.OnAdd(current, created)
		}
	case watcher.Write:
		for _, h := range handlers {
			updated, err := pathToAttribute(event.Path)
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
