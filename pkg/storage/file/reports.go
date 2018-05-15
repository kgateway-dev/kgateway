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
type reportsClient struct {
	dir           string
	syncFrequency time.Duration
}

func (c *reportsClient) Create(item *v1.Report) (*v1.Report, error) {
	// set resourceversion on clone
	reportClone, ok := proto.Clone(item).(*v1.Report)
	if !ok {
		return nil, errors.New("internal error: output of proto.Clone was not expected type")
	}
	if reportClone.Metadata == nil {
		reportClone.Metadata = &v1.Metadata{}
	}
	reportClone.Metadata.ResourceVersion = newOrIncrementResourceVer(reportClone.Metadata.ResourceVersion)
	reportFiles, err := c.pathsToReports()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read report dir")
	}
	// error if exists already
	for file, existingUps := range reportFiles {
		if existingUps.Name == item.Name {
			return nil, storage.NewAlreadyExistsErr(errors.Errorf("report %v already defined in %s", item.Name, file))
		}
	}
	filename := filepath.Join(c.dir, item.Name+".yml")
	err = WriteToFile(filename, reportClone)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating file")
	}
	return reportClone, nil
}

func (c *reportsClient) Update(item *v1.Report) (*v1.Report, error) {
	if item.Metadata == nil || item.Metadata.ResourceVersion == "" {
		return nil, errors.New("resource version must be set for update operations")
	}
	reportFiles, err := c.pathsToReports()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read report dir")
	}
	// error if exists already
	for file, existingUps := range reportFiles {
		if existingUps.Name != item.Name {
			continue
		}
		if existingUps.Metadata != nil && lessThan(item.Metadata.ResourceVersion, existingUps.Metadata.ResourceVersion) {
			return nil, errors.Errorf("resource version outdated for %v", item.Name)
		}
		reportClone, ok := proto.Clone(item).(*v1.Report)
		if !ok {
			return nil, errors.New("internal error: output of proto.Clone was not expected type")
		}
		reportClone.Metadata.ResourceVersion = newOrIncrementResourceVer(reportClone.Metadata.ResourceVersion)

		err = WriteToFile(file, reportClone)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating file")
		}

		return reportClone, nil
	}
	return nil, errors.Errorf("report %v not found", item.Name)
}

func (c *reportsClient) Delete(name string) error {
	reportFiles, err := c.pathsToReports()
	if err != nil {
		return errors.Wrap(err, "failed to read report dir")
	}
	// error if exists already
	for file, existingUps := range reportFiles {
		if existingUps.Name == name {
			return os.Remove(file)
		}
	}
	return errors.Errorf("file not found for report %v", name)
}

func (c *reportsClient) Get(name string) (*v1.Report, error) {
	reportFiles, err := c.pathsToReports()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read report dir")
	}
	// error if exists already
	for _, existingUps := range reportFiles {
		if existingUps.Name == name {
			return existingUps, nil
		}
	}
	return nil, errors.Errorf("file not found for report %v", name)
}

func (c *reportsClient) List() ([]*v1.Report, error) {
	reportPaths, err := c.pathsToReports()
	if err != nil {
		return nil, err
	}
	var reports []*v1.Report
	for _, up := range reportPaths {
		reports = append(reports, up)
	}
	return reports, nil
}

func (c *reportsClient) pathsToReports() (map[string]*v1.Report, error) {
	files, err := ioutil.ReadDir(c.dir)
	if err != nil {
		return nil, errors.Wrap(err, "could not read dir")
	}
	reports := make(map[string]*v1.Report)
	for _, f := range files {
		path := filepath.Join(c.dir, f.Name())
		if !strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".yaml") {
			continue
		}
		var report v1.Report
		err := ReadFileInto(path, &report)
		if err != nil {
			return nil, errors.Wrap(err, "unable to parse .yml file as report")
		}
		reports[path] = &report
	}
	return reports, nil
}

func (u *reportsClient) Watch(handlers ...storage.ReportEventHandler) (*storage.Watcher, error) {
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

func (u *reportsClient) onEvent(event watcher.Event, handlers ...storage.ReportEventHandler) error {
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
			var created v1.Report
			err := ReadFileInto(event.Path, &created)
			if err != nil {
				return err
			}
			h.OnAdd(current, &created)
		}
	case watcher.Write:
		for _, h := range handlers {
			var updated v1.Report
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
