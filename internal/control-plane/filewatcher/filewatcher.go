package filewatcher

import (
	"sort"
	"sync"

	"github.com/d4l3k/messagediff"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage"
	"github.com/solo-io/gloo/pkg/storage/dependencies"
)

type fileWatcher struct {
	watchers    []*storage.Watcher
	fileRefs    []string
	files       chan Files
	filestorage dependencies.FileStorage
	errs        chan error
}

func filterFiles(desiredFileRefs []string, files []*dependencies.File) Files {
	f := make(Files)
	for _, file := range files {
		for _, desired := range desiredFileRefs {
			if file.Ref == desired {
				f[file.Ref] = file
				break
			}
		}
	}
	return f
}

func NewFileWatcher(filestore dependencies.FileStorage) (Interface, error) {
	files := make(chan Files)

	fw := &fileWatcher{
		files:       files,
		filestorage: filestore,
		errs:        make(chan error),
	}

	var cachedFiles Files

	syncFiles := func(updatedList []*dependencies.File, _ *dependencies.File) {
		sort.SliceStable(updatedList, func(i, j int) bool {
			return updatedList[i].Ref < updatedList[j].Ref
		})

		filtered := filterFiles(fw.fileRefs, updatedList)

		diff, equal := messagediff.PrettyDiff(cachedFiles, filtered)
		if equal {
			return
		}
		log.GreyPrintf("change detected in files: %v", diff)

		cachedFiles = filtered
		if len(filtered) < 1 {
			return
		}
		files <- filtered
	}
	watcher, err := filestore.Watch(&dependencies.FileEventHandlerFuncs{
		AddFunc:    syncFiles,
		UpdateFunc: syncFiles,
		DeleteFunc: syncFiles,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create watcher for files")
	}

	fw.watchers = []*storage.Watcher{watcher}

	return fw, nil
}

func (w *fileWatcher) Run(stop <-chan struct{}) {
	done := &sync.WaitGroup{}
	for _, watcher := range w.watchers {
		done.Add(1)
		go func(watcher *storage.Watcher, stop <-chan struct{}, errs chan error) {
			watcher.Run(stop, errs)
			done.Done()
		}(watcher, stop, w.errs)
	}
	done.Wait()
}

func (w *fileWatcher) TrackFiles(fileRefs []string) {
	w.fileRefs = fileRefs
	list, err := w.filestorage.List()
	if err != nil {
		log.Warnf("failed to get updated file list: %v", err)
		return
	}
	files := filterFiles(fileRefs, list)
	if len(files) < 1 {
		return
	}
	w.files <- files
}

func (w *fileWatcher) Files() <-chan Files {
	return w.files
}

func (w *fileWatcher) Error() <-chan error {
	return w.errs
}
