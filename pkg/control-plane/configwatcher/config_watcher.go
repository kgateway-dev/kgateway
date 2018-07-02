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
	"github.com/gogo/protobuf/proto"
)

type configWatcher struct {
	watchers []*storage.Watcher
	configs  chan *v1.Config
	errs     chan error
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
	initialVirtualServices, err := storageClient.V1().VirtualServices().List()
	if err != nil {
		log.Warnf("Startup: failed to read virtual services from storage: %v", err)
		initialVirtualServices = []*v1.VirtualService{}
	}
	initialRoles, err := storageClient.V1().Roles().List()
	if err != nil {
		log.Warnf("Startup: failed to read virtual services from storage: %v", err)
		initialRoles = []*v1.Role{}
	}
	configs := make(chan *v1.Config)
	// do a first time read
	cache := &v1.Config{
		Upstreams:       initialUpstreams,
		VirtualServices: initialVirtualServices,
		Roles: initialRoles,
	}
	// throw it down the channel to get things going
	go func() {
		configs <- cache
	}()

	syncUpstreams := func(updatedList []*v1.Upstream, _ *v1.Upstream) {
		sort.SliceStable(updatedList, func(i, j int) bool {
			return updatedList[i].GetName() < updatedList[j].GetName()
		})

		diff, equal := messagediff.PrettyDiff(cache.Upstreams, updatedList)
		if equal {
			return
		}
		log.GreyPrintf("change detected in upstream: %v", diff)

		cache.Upstreams = updatedList
		configs <- proto.Clone(cache).(*v1.Config)
	}
	upstreamWatcher, err := storageClient.V1().Upstreams().Watch(&storage.UpstreamEventHandlerFuncs{
		AddFunc:    syncUpstreams,
		UpdateFunc: syncUpstreams,
		DeleteFunc: syncUpstreams,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create watcher for upstreams")
	}

	syncvServices := func(updatedList []*v1.VirtualService, _ *v1.VirtualService) {
		sort.SliceStable(updatedList, func(i, j int) bool {
			return updatedList[i].GetName() < updatedList[j].GetName()
		})

		diff, equal := messagediff.PrettyDiff(cache.VirtualServices, updatedList)
		if equal {
			return
		}
		log.GreyPrintf("change detected in virtualservices: %v", diff)

		cache.VirtualServices = updatedList
		configs <- proto.Clone(cache).(*v1.Config)
	}
	vServiceWatcher, err := storageClient.V1().VirtualServices().Watch(&storage.VirtualServiceEventHandlerFuncs{
		AddFunc:    syncvServices,
		UpdateFunc: syncvServices,
		DeleteFunc: syncvServices,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create watcher for virtualservices")
	}


	syncroles := func(updatedList []*v1.Role, _ *v1.Role) {
		sort.SliceStable(updatedList, func(i, j int) bool {
			return updatedList[i].GetName() < updatedList[j].GetName()
		})

		diff, equal := messagediff.PrettyDiff(cache.Roles, updatedList)
		if equal {
			return
		}
		log.GreyPrintf("change detected in virtualservices: %v", diff)

		cache.Roles = updatedList
		configs <- proto.Clone(cache).(*v1.Config)
	}
	roleWatcher, err := storageClient.V1().Roles().Watch(&storage.RoleEventHandlerFuncs{
		AddFunc:    syncroles,
		UpdateFunc: syncroles,
		DeleteFunc: syncroles,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create watcher for roles")
	}

	return &configWatcher{
		watchers: []*storage.Watcher{vServiceWatcher, roleWatcher, upstreamWatcher},
		configs:  configs,
		errs:     make(chan error),
	}, nil
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
