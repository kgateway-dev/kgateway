package upstreamwatcher

import (
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/storage"
)

// returns a channel of upstreams that need function discovery
// upstream selector tells us whether the upstream should be selected for discovery
func WatchUpstreams(gloo storage.Interface,
	stop <-chan struct{},
	errs chan error) (<-chan []*v1.Upstream, error) {

	upstreams := make(chan []*v1.Upstream)
	syncFunc := func(newList []*v1.Upstream, _ *v1.Upstream) {
		if len(newList) > 0 {
			upstreams <- newList
		}
	}

	w, err := gloo.V1().Upstreams().Watch(storage.UpstreamEventHandlerFuncs{
		AddFunc:    syncFunc,
		UpdateFunc: syncFunc,
		DeleteFunc: syncFunc,
	})
	if err != nil {
		return nil, err
	}
	go w.Run(stop, errs)
	return upstreams, nil
}
