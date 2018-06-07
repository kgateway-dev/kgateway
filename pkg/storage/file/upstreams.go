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
type upstreamsClient struct {
	dir           string
	syncFrequency time.Duration
}

func (c *upstreamsClient) Create(item *v1.Upstream) (*v1.Upstream, error) {
	if item.Name == "" {
		return nil, errors.Errorf("name required")
	}
	// set resourceversion on clone
	upstreamClone, ok := proto.Clone(item).(*v1.Upstream)
	if !ok {
		return nil, errors.New("internal error: output of proto.Clone was not expected type")
	}
	if upstreamClone.Metadata == nil {
		upstreamClone.Metadata = &v1.Metadata{}
	}
	upstreamClone.Metadata.ResourceVersion = newOrIncrementResourceVer(upstreamClone.Metadata.ResourceVersion)
	upstreamFiles, err := c.pathsToUpstreams()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read upstream dir")
	}
	// error if exists already
	for file, existingUps := range upstreamFiles {
		if existingUps.Name == item.Name {
			return nil, storage.NewAlreadyExistsErr(errors.Errorf("upstream %v already defined in %s", item.Name, file))
		}
	}
	filename := filepath.Join(c.dir, item.Name+".yml")
	err = WriteToFile(filename, upstreamClone)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating file")
	}
	return upstreamClone, nil
}

func (c *upstreamsClient) Update(item *v1.Upstream) (*v1.Upstream, error) {
	if item.Name == "" {
		return nil, errors.Errorf("name required")
	}
	if item.Metadata == nil || item.Metadata.ResourceVersion == "" {
		return nil, errors.New("resource version must be set for update operations")
	}
	upstreamFiles, err := c.pathsToUpstreams()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read upstream dir")
	}
	// error if exists already
	for file, existingUps := range upstreamFiles {
		if existingUps.Name != item.Name {
			continue
		}
		if existingUps.Metadata != nil && lessThan(item.Metadata.ResourceVersion, existingUps.Metadata.ResourceVersion) {
			return nil, errors.Errorf("resource version outdated for %v", item.Name)
		}
		upstreamClone, ok := proto.Clone(item).(*v1.Upstream)
		if !ok {
			return nil, errors.New("internal error: output of proto.Clone was not expected type")
		}
		upstreamClone.Metadata.ResourceVersion = newOrIncrementResourceVer(upstreamClone.Metadata.ResourceVersion)

		err = WriteToFile(file, upstreamClone)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating file")
		}

		return upstreamClone, nil
	}
	return nil, errors.Errorf("upstream %v not found", item.Name)
}

func (c *upstreamsClient) Delete(name string) error {
	upstreamFiles, err := c.pathsToUpstreams()
	if err != nil {
		return errors.Wrap(err, "failed to read upstream dir")
	}
	// error if exists already
	for file, existingUps := range upstreamFiles {
		if existingUps.Name == name {
			return os.Remove(file)
		}
	}
	return errors.Errorf("file not found for upstream %v", name)
}

func (c *upstreamsClient) Get(name string) (*v1.Upstream, error) {
	upstreamFiles, err := c.pathsToUpstreams()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read upstream dir")
	}
	// error if exists already
	for _, existingUps := range upstreamFiles {
		if existingUps.Name == name {
			return existingUps, nil
		}
	}
	return nil, errors.Errorf("file not found for upstream %v", name)
}

func (c *upstreamsClient) List() ([]*v1.Upstream, error) {
	upstreamPaths, err := c.pathsToUpstreams()
	if err != nil {
		return nil, err
	}
	var upstreams []*v1.Upstream
	for _, up := range upstreamPaths {
		upstreams = append(upstreams, up)
	}
	return upstreams, nil
}

func (c *upstreamsClient) pathsToUpstreams() (map[string]*v1.Upstream, error) {
	files, err := ioutil.ReadDir(c.dir)
	if err != nil {
		return nil, errors.Wrap(err, "could not read dir")
	}
	upstreams := make(map[string]*v1.Upstream)
	for _, f := range files {
		path := filepath.Join(c.dir, f.Name())
		if !strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".yaml") {
			continue
		}

		upstream, err := pathToUpstream(path)
        if err != nil {
            return nil, errors.Wrap(err, "unable to parse .yml file as upstream")
        }

        upstreams[path] = upstream
	}
	return upstreams, nil
}

func pathToUpstream(path string) (*v1.Upstream, error) {
	var upstream v1.Upstream
	err := ReadFileInto(path, &upstream)
	if err != nil {
		return nil, err
	}
	if upstream.Metadata == nil {
		upstream.Metadata = &v1.Metadata{}
	}
	if upstream.Metadata.ResourceVersion == "" {
		upstream.Metadata.ResourceVersion = "1"
	}
	return &upstream, nil
}

func (u *upstreamsClient) Watch(handlers ...storage.UpstreamEventHandler) (*storage.Watcher, error) {
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

func (u *upstreamsClient) onEvent(event watcher.Event, handlers ...storage.UpstreamEventHandler) error {
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
			created, err := pathToUpstream(event.Path)
			if err != nil {
				return err
			}
			h.OnAdd(current, created)
		}
	case watcher.Write:
		for _, h := range handlers {
			updated, err := pathToUpstream(event.Path)
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
