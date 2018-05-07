package storage

import "github.com/solo-io/gloo/pkg/api/types/v1"

type Watcher struct {
	runFunc func(stop <-chan struct{}, errs chan error)
}

func NewWatcher(runFunc func(stop <-chan struct{}, errs chan error)) *Watcher {
	return &Watcher{runFunc: runFunc}
}

func (w *Watcher) Run(stop <-chan struct{}, errs chan error) {
	w.runFunc(stop, errs)
}

type UpstreamEventHandler interface {
	OnAdd(updatedList []*v1.Upstream, obj *v1.Upstream)
	OnUpdate(updatedList []*v1.Upstream, newObj *v1.Upstream)
	OnDelete(updatedList []*v1.Upstream, obj *v1.Upstream)
}

type VirtualServiceEventHandler interface {
	OnAdd(updatedList []*v1.VirtualService, obj *v1.VirtualService)
	OnUpdate(updatedList []*v1.VirtualService, newObj *v1.VirtualService)
	OnDelete(updatedList []*v1.VirtualService, obj *v1.VirtualService)
}

// UpstreamEventHandlerFuncs is an adaptor to let you easily specify as many or
// as few of the notification functions as you want while still implementing
// UpstreamEventHandler.
type UpstreamEventHandlerFuncs struct {
	AddFunc    func(updatedList []*v1.Upstream, obj *v1.Upstream)
	UpdateFunc func(updatedList []*v1.Upstream, newObj *v1.Upstream)
	DeleteFunc func(updatedList []*v1.Upstream, obj *v1.Upstream)
}

// OnAdd calls AddFunc if it's not nil.
func (r UpstreamEventHandlerFuncs) OnAdd(updatedList []*v1.Upstream, obj *v1.Upstream) {
	if r.AddFunc != nil {
		r.AddFunc(updatedList, obj)
	}
}

// OnUpdate calls UpdateFunc if it's not nil.
func (r UpstreamEventHandlerFuncs) OnUpdate(updatedList []*v1.Upstream, newObj *v1.Upstream) {
	if r.UpdateFunc != nil {
		r.UpdateFunc(updatedList, newObj)
	}
}

// OnDelete calls DeleteFunc if it's not nil.
func (r UpstreamEventHandlerFuncs) OnDelete(updatedList []*v1.Upstream, obj *v1.Upstream) {
	if r.DeleteFunc != nil {
		r.DeleteFunc(updatedList, obj)
	}
}

// VirtualServiceEventHandlerFuncs is an adaptor to let you easily specify as many or
// as few of the notification functions as you want while still implementing
// VirtualServiceEventHandler.
type VirtualServiceEventHandlerFuncs struct {
	AddFunc    func(updatedList []*v1.VirtualService, obj *v1.VirtualService)
	UpdateFunc func(updatedList []*v1.VirtualService, newObj *v1.VirtualService)
	DeleteFunc func(updatedList []*v1.VirtualService, obj *v1.VirtualService)
}

// OnAdd calls AddFunc if it's not nil.
func (r VirtualServiceEventHandlerFuncs) OnAdd(updatedList []*v1.VirtualService, obj *v1.VirtualService) {
	if r.AddFunc != nil {
		r.AddFunc(updatedList, obj)
	}
}

// OnUpdate calls UpdateFunc if it's not nil.
func (r VirtualServiceEventHandlerFuncs) OnUpdate(updatedList []*v1.VirtualService, newObj *v1.VirtualService) {
	if r.UpdateFunc != nil {
		r.UpdateFunc(updatedList, newObj)
	}
}

// OnDelete calls DeleteFunc if it's not nil.
func (r VirtualServiceEventHandlerFuncs) OnDelete(updatedList []*v1.VirtualService, obj *v1.VirtualService) {
	if r.DeleteFunc != nil {
		r.DeleteFunc(updatedList, obj)
	}
}
