package cloudfoundry

import (
	"context"
	"sort"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/copilot"
	copilotapi "code.cloudfoundry.org/copilot/api"

	"github.com/mitchellh/hashstructure"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/pkg/log"
)

type endpointDiscovery struct {
	resyncDuration time.Duration

	ctx    context.Context
	cancel context.CancelFunc

	client copilot.IstioClient

	errors           chan error
	endpoints        chan endpointdiscovery.EndpointGroups
	upstreamsToTrack atomic.Value

	lastSeen uint64

	kickChan chan struct{}
}

func NewEndpointDiscovery(ctx context.Context, client copilot.IstioClient, resyncDuration time.Duration) endpointdiscovery.Interface {
	ctx, cancel := context.WithCancel(ctx)
	return &endpointDiscovery{
		resyncDuration: resyncDuration,
		ctx:            ctx,
		cancel:         cancel,

		client: client,

		errors:    make(chan error, 100),
		endpoints: make(chan endpointdiscovery.EndpointGroups, 100),
		kickChan:  make(chan struct{}, 1),
	}
}

func (ed *endpointDiscovery) getUpstreamsToTrack() []*v1.Upstream {
	maybeUss := ed.upstreamsToTrack.Load()
	if maybeUss == nil {
		return nil
	}
	return maybeUss.([]*v1.Upstream)
}

func (ed *endpointDiscovery) setUpstreamsToTrack(u []*v1.Upstream) {
	ed.upstreamsToTrack.Store(u)
}

func (ed *endpointDiscovery) Run(stop <-chan struct{}) {
	ResyncLoopWithKick(ed.ctx, stop, ed.resync, ed.resyncDuration, ed.kickChan)
}

func (ed *endpointDiscovery) resync() {
	err := ed.resyncWithError()
	if err != nil {
		ed.errors <- err
	}
}

func (ed *endpointDiscovery) resyncWithError() error {
	resp, err := ed.client.Routes(ed.ctx, new(copilotapi.RoutesRequest))
	if err != nil {
		return err
	}
	endpointGroups := ed.responseToEndpointGroups(resp)

	ed.processEndpoint(endpointGroups)
	return nil
}

func (ed *endpointDiscovery) responseToEndpointGroups(resp *copilotapi.RoutesResponse) endpointdiscovery.EndpointGroups {

	endpointGroups := make(endpointdiscovery.EndpointGroups)
	for _, us := range ed.getUpstreamsToTrack() {
		eps, err := GetEndpointsFromResponse(resp, us)
		if err != nil {
			if err != WrongUpstreamType {
				ed.errors <- err
			}
			continue
		}
		endpointGroups[us.Name] = eps
	}
	return endpointGroups
}

func (ed *endpointDiscovery) processEndpoint(endpointGroups endpointdiscovery.EndpointGroups) {

	// sort for stability with the rest of gloo

	for upstreamName, epGroup := range endpointGroups {
		sort.SliceStable(epGroup, func(i, j int) bool {
			return endpointdiscovery.Less(&epGroup[i], &epGroup[j])
		})
		endpointGroups[upstreamName] = epGroup
	}

	newHash, err := hashstructure.Hash(endpointGroups, nil)
	if err != nil {
		log.Warnf("error in cloudfoundry endpoint controller: %v", err)
		return
	}

	ed.updateIfNeeded(endpointGroups, newHash)
}

func (ed *endpointDiscovery) updateIfNeeded(endpointGroups endpointdiscovery.EndpointGroups, newHash uint64) {

	lastSeen := atomic.LoadUint64(&ed.lastSeen)

	if newHash == lastSeen {
		return
	}
	swapped := atomic.CompareAndSwapUint64(&ed.lastSeen, lastSeen, newHash)
	if swapped {
		ed.endpoints <- endpointGroups
	}
	// swapped == false - no need to send the endpoints if someone else has done it..
}

func (ed *endpointDiscovery) TrackUpstreams(upstreams []*v1.Upstream) {
	ed.setUpstreamsToTrack(upstreams)
	// kick the sync loop to trigger an update now.
	select {
	case ed.kickChan <- struct{}{}:
	default:
	}
}

func (ed *endpointDiscovery) Endpoints() <-chan endpointdiscovery.EndpointGroups {
	return ed.endpoints
}
func (ed *endpointDiscovery) Error() <-chan error {
	return ed.errors
}
