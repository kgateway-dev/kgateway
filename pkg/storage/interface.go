package storage

import "github.com/solo-io/gloo/pkg/api/types/v1"

// Interface is interface to the storage backend
type Interface interface {
	V1() V1
}

type V1 interface {
	Register() error
	Upstreams() Upstreams
	VirtualServices() VirtualServices
}

type Upstreams interface {
	Create(*v1.Upstream) (*v1.Upstream, error)
	Update(*v1.Upstream) (*v1.Upstream, error)
	Delete(name string) error
	Get(name string) (*v1.Upstream, error)
	List() ([]*v1.Upstream, error)
	Watch(handlers ...UpstreamEventHandler) (*Watcher, error)
}

type VirtualServices interface {
	Create(*v1.VirtualService) (*v1.VirtualService, error)
	Update(*v1.VirtualService) (*v1.VirtualService, error)
	Delete(name string) error
	Get(name string) (*v1.VirtualService, error)
	List() ([]*v1.VirtualService, error)
	Watch(...VirtualServiceEventHandler) (*Watcher, error)
}
