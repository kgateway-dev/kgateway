package endpointdiscovery

import "github.com/solo-io/gloo/pkg/api/types/v1"

// groups endpoints by their respective upstream name
type EndpointGroups map[string][]Endpoint

type Endpoint struct {
	Address string
	Port    int32
}

type Interface interface {
	// starts the discovery service
	Run(stop <-chan struct{})

	// tells the discovery to track endpoints for the given upstreams
	TrackUpstreams(upstreams []*v1.Upstream)

	// endpoint groups are pushed here whenever they are updated
	Endpoints() <-chan EndpointGroups

	// should show valid if the most recent update passed, otherwise a useful error
	Error() <-chan error
}

func Less(e1, e2 *Endpoint) bool {
	if e1.Address < e2.Address {
		return true
	}
	if e1.Port < e2.Port {
		return true
	}
	return false
}
