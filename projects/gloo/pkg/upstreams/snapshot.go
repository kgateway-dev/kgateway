package upstreams

import (
	"sync"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/hashutils"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
)

// An upstream collection that provides utility functions to add upstreams and converted services
type HybridUpstreamSnapshot interface {
	SetUpstreams(upstreams v1.UpstreamList)
	SetServices(services skkube.ServiceList)
	ToList() v1.UpstreamList
	Clone() HybridUpstreamSnapshot
	Hash() uint64
}

type upstreamSnapshot struct {
	sync.RWMutex
	realUpstreams, serviceUpstreams v1.UpstreamList
}

func NewHybridUpstreamSnapshot() HybridUpstreamSnapshot {
	return &upstreamSnapshot{}
}

// Merges the given upstreams into the underlying upstream collection
func (u *upstreamSnapshot) SetUpstreams(upstreams v1.UpstreamList) {
	u.Lock()
	defer u.Unlock()
	u.realUpstreams = upstreams
}

// Converts the given kubernetes services to upstreams and merges them into the underlying upstream collection
func (u *upstreamSnapshot) SetServices(services skkube.ServiceList) {
	u.Lock()
	defer u.Unlock()
	u.serviceUpstreams = servicesToUpstreams(services)
}

// List the content of the underlying upstream collection
func (u *upstreamSnapshot) ToList() v1.UpstreamList {
	u.RLock()
	defer u.RUnlock()
	return append(u.realUpstreams, u.serviceUpstreams...)
}

func (u *upstreamSnapshot) Clone() HybridUpstreamSnapshot {
	u.RLock()
	defer u.RUnlock()

	return &upstreamSnapshot{
		realUpstreams:    u.realUpstreams.Clone(),
		serviceUpstreams: u.serviceUpstreams.Clone()}
}

func (u *upstreamSnapshot) Hash() uint64 {
	u.RLock()
	defer u.RUnlock()

	// Sort merged slice for consistent hashing
	usList := append(u.realUpstreams, u.serviceUpstreams...).Sort()

	return hashutils.HashAll(usList.AsInterfaces()...)
}
