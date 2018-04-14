package file

import (
	"time"

	"io/ioutil"

	"path/filepath"

	"github.com/pkg/errors"
	"github.com/radovskyb/watcher"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
)

type fileStorage struct {
	dir           string
	syncFrequency time.Duration
}

func NewFileStorage(dir string, syncFrequency time.Duration) (dependencies.FileStorage, error) {
	return &fileStorage{
		dir:           dir,
		syncFrequency: syncFrequency,
	}, nil
}

func copyFile(file *dependencies.File) *dependencies.File {
	contents := make([]byte, len(file.Contents))
	copy(contents, file.Contents)
	return &dependencies.File{
		Ref:             file.Ref,
		Contents:        contents,
		ResourceVersion: file.ResourceVersion,
	}
}

func (s *fileStorage) Create(file *dependencies.File) (*dependencies.File, error) {
	if _, err := s.Get(file.Ref); err == nil {
		return nil, errors.Errorf("file %v already exists", file.Ref)
	}
	if err := writeFile(s.dir, file); err != nil {
		return nil, errors.Wrap(err, "writing file")
	}
	return copyFile(file), nil
}

func (s *fileStorage) Update(file *dependencies.File) (*dependencies.File, error) {
	if err := writeFile(s.dir, file); err != nil {
		return nil, errors.Wrap(err, "writing file")
	}
	return copyFile(file), nil
}

func (s *fileStorage) Delete(name string) error {
	err := deleteFile(s.dir, name)
	if err != nil {
		return errors.Wrap(err, "deleting file")
	}
	return nil
}

func (s *fileStorage) Get(name string) (*dependencies.File, error) {
	file, err := readFile(s.dir, name)
	if err != nil {
		return nil, errors.Wrap(err, "reading file")
	}
	return file, nil
}

func (s *fileStorage) List() ([]*dependencies.File, error) {
	osFiles, err := ioutil.ReadDir(s.dir)
	if err != nil {
		return nil, errors.Wrap(err, "could not read dir")
	}
	var files []*dependencies.File
	for _, f := range osFiles {
		file, err := s.Get(f.Name())
		if err != nil {
			return nil, errors.Wrapf(err, "getting file %v", f.Name())
		}
		files = append(files, file)
	}
	return files, nil
}

func (s *fileStorage) Watch(handlers ...dependencies.FileEventHandler) (*storage.Watcher, error) {
	w := watcher.New()
	w.SetMaxEvents(0)
	w.FilterOps(watcher.Create, watcher.Write, watcher.Remove)
	if err := w.AddRecursive(s.dir); err != nil {
		return nil, errors.Wrapf(err, "failed to add directory %v", s.dir)
	}

	return storage.NewWatcher(func(stop <-chan struct{}, errs chan error) {
		go func() {
			if err := w.Start(s.syncFrequency); err != nil {
				errs <- err
			}
		}()
		// start the watch with an "initial read" event
		current, err := s.List()
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
				if err := s.onEvent(event, handlers...); err != nil {
					log.Warnf("watcher encountered error: %v", err)
				}
			case err := <-w.Error:
				log.Warnf("watcher encountered error: %v", err)
				return
			case err := <-errs:
				log.Warnf("failed to start watcher to: %v", err)
				return
			case <-stop:
				w.Close()
				return
			}
		}
	}), nil
}

func (s *fileStorage) onEvent(event watcher.Event, handlers ...dependencies.FileEventHandler) error {
	log.Debugf("file event: %v [%v]", event.Path, event.Op)
	current, err := s.List()
	if err != nil {
		return err
	}
	if event.IsDir() {
		return nil
	}
	switch event.Op {
	case watcher.Create:
		for _, h := range handlers {
			created, err := readFile(s.dir, filepath.Base(event.Path))
			if err != nil {
				return err
			}
			h.OnAdd(current, created)
		}
	case watcher.Write:
		for _, h := range handlers {
			updated, err := readFile(s.dir, filepath.Base(event.Path))
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
