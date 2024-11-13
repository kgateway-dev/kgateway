package upstreams

import (
	"context"

	errors "github.com/rotisserie/eris"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/hashutils"
)

// Groups upstreams from different sources into a single snapshot
type hybridUpstreamSnapshot struct {
	upstreamsBySource map[string]v1.UpstreamList
}

func (s *hybridUpstreamSnapshot) setUpstreams(source string, upstreams v1.UpstreamList) {
	s.upstreamsBySource[source] = upstreams
}

func (s *hybridUpstreamSnapshot) toList() v1.UpstreamList {
	var allUpstreams v1.UpstreamList
	for _, upstreams := range s.upstreamsBySource {
		allUpstreams = append(allUpstreams, upstreams...)
	}
	return allUpstreams
}

func (s *hybridUpstreamSnapshot) clone() *hybridUpstreamSnapshot {
	clone := make(map[string]v1.UpstreamList)
	for source, upstreams := range s.upstreamsBySource {
		clone[source] = upstreams.Clone()
	}

	return &hybridUpstreamSnapshot{
		upstreamsBySource: clone,
	}
}

func (s *hybridUpstreamSnapshot) hash() (uint64, error) {
	var allUpstreams v1.UpstreamList
	for _, upstreams := range s.upstreamsBySource {
		allUpstreams = append(allUpstreams, upstreams...)
	}

	// Sort merged slice for consistent hashing
	allUpstreams.Sort()
	hash, err := hashutils.HashAllSafe(nil, allUpstreams.AsInterfaces()...)
	if err != nil {
		contextutils.LoggerFrom(context.Background()).DPanic("this error should never happen, as it is in a safe hasher")
		return 0, errors.New("this error should never happen, as this is safe hasher")
	}

	return hash, nil
}
