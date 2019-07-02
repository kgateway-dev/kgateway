package upstreams

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/hashutils"
)

// Groups real and service-derived upstreams
type hybridUpstreamSnapshot struct {
	realUpstreams, serviceUpstreams v1.UpstreamList
}

func (s *hybridUpstreamSnapshot) SetRealUpstreams(upstreams v1.UpstreamList) {
	s.realUpstreams = upstreams
}

func (s *hybridUpstreamSnapshot) SetServiceUpstreams(upstreams v1.UpstreamList) {
	s.serviceUpstreams = upstreams
}

func (s *hybridUpstreamSnapshot) ToList() v1.UpstreamList {
	return append(s.realUpstreams, s.serviceUpstreams...)
}

func (s *hybridUpstreamSnapshot) Clone() *hybridUpstreamSnapshot {
	return &hybridUpstreamSnapshot{
		realUpstreams:    s.realUpstreams.Clone(),
		serviceUpstreams: s.serviceUpstreams.Clone()}
}

func (s *hybridUpstreamSnapshot) Hash() uint64 {
	// Sort merged slice for consistent hashing
	usList := append(s.realUpstreams.Clone(), s.serviceUpstreams.Clone()...).Sort()
	return hashutils.HashAll(usList.AsInterfaces()...)
}
