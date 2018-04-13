package configwatcher

import (
	"fmt"
	"sort"
	"sync"

	"github.com/d4l3k/messagediff"
	"github.com/pkg/errors"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/gloo/pkg/storage"
)

type configWatcher struct {
	watchers []*storage.Watcher
	configs  chan *v1.Config
	errs     chan error

	cache     *v1.Config
	cacheLock sync.Mutex
}

func NewConfigWatcher(storageClient storage.Interface) (*configWatcher, error) {
	if err := storageClient.V1().Register(); err != nil && !storage.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to register to storage backend: %v", err)
	}

	initialUpstreams, err := storageClient.V1().Upstreams().List()
	if err != nil {
		log.Warnf("Startup: failed to read upstreams from storage: %v", err)
		initialUpstreams = []*v1.Upstream{}
	}
	initialVirtualHosts, err := storageClient.V1().VirtualHosts().List()
	if err != nil {
		log.Warnf("Startup: failed to read virtual hosts from storage: %v", err)
		initialVirtualHosts = []*v1.VirtualHost{}
	}
	configs := make(chan *v1.Config)
	// do a first time read
	cache := &v1.Config{
		Upstreams:    initialUpstreams,
		VirtualHosts: initialVirtualHosts,
	}
	// throw it down the channel to get things going
	go func(cache v1.Config) {
		configs <- &cache
	}(*cache)

	cw := &configWatcher{
		configs: configs,
		errs:    make(chan error),
		cache:   cache,
	}

	upstreamWatcher, err := storageClient.V1().Upstreams().Watch(&storage.UpstreamEventHandlerFuncs{
		AddFunc:    cw.syncUpstreams,
		UpdateFunc: cw.syncUpstreams,
		DeleteFunc: cw.syncUpstreams,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create watcher for upstreams")
	}
	vhostWatcher, err := storageClient.V1().VirtualHosts().Watch(&storage.VirtualHostEventHandlerFuncs{
		AddFunc:    cw.syncVhosts,
		UpdateFunc: cw.syncVhosts,
		DeleteFunc: cw.syncVhosts,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create watcher for virtualhosts")
	}

	cw.watchers = []*storage.Watcher{vhostWatcher, upstreamWatcher}
	return cw, nil
}

func (w *configWatcher) syncVhosts(updatedList []*v1.VirtualHost, _ *v1.VirtualHost) {
	sort.SliceStable(updatedList, func(i, j int) bool {
		return updatedList[i].GetName() < updatedList[j].GetName()
	})

	w.cacheLock.Lock()
	vhosts := w.cache.VirtualHosts
	w.cacheLock.Unlock()

	diff, equal := messagediff.PrettyDiff(vhosts, updatedList)
	if equal {
		return
	}
	log.GreyPrintf("change detected in virtualhosts: %v", diff)

	w.cacheLock.Lock()
	w.cache.VirtualHosts = updatedList
	copyCache := *w.cache
	w.cacheLock.Unlock()

	w.configs <- &copyCache
}

func (w *configWatcher) syncUpstreams(updatedList []*v1.Upstream, _ *v1.Upstream) {
	sort.SliceStable(updatedList, func(i, j int) bool {
		return updatedList[i].GetName() < updatedList[j].GetName()
	})

	w.cacheLock.Lock()
	upstreams := w.cache.Upstreams
	w.cacheLock.Unlock()

	diff, equal := messagediff.PrettyDiff(upstreams, updatedList)
	if equal {
		return
	}
	log.GreyPrintf("change detected in upstream: %v", diff)

	w.cacheLock.Lock()
	w.cache.Upstreams = updatedList
	copyCache := *w.cache
	w.cacheLock.Unlock()

	w.configs <- &copyCache
}

func (w *configWatcher) Run(stop <-chan struct{}) {
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

func (w *configWatcher) Config() <-chan *v1.Config {
	return w.configs
}

func (w *configWatcher) Error() <-chan error {
	return w.errs
}
