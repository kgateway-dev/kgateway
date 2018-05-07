package endpointswatcher

import (
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/pkg/plugins"
	"github.com/solo-io/gloo/pkg/log"
	"sync"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/bootstrap"
)

// watches all the endpoint discoveries registered by various plugins and aggregates them into a single stream
type endpointsAggregator struct {
	endpointDiscoveries []endpointdiscovery.Interface
	endpointsByDiscovery chan endpointTuple
	aggregatedEndpoints chan endpointdiscovery.EndpointGroups
	errors chan error
}

func NewEndpointsWatcher(opts bootstrap.Options, endpointDiscoveryPlugins ... plugins.EndpointDiscoveryPlugin) endpointdiscovery.Interface {
	var endpointDiscoveries []endpointdiscovery.Interface
	for _, edPlugin := range endpointDiscoveryPlugins {
		discovery, err := edPlugin.SetupEndpointDiscovery(opts)
		if err != nil {
			log.Warnf("Starting endpoint discovery failed: %v, endpoints will not be discovered for this "+
				"upstream type", err)
			continue
		}
		endpointDiscoveries = append(endpointDiscoveries, discovery)
	}
	return &endpointsAggregator{
		endpointDiscoveries: endpointDiscoveries,
		endpointsByDiscovery: make(chan endpointTuple),
		aggregatedEndpoints: make(chan endpointdiscovery.EndpointGroups),
		errors: make(chan error),
	}
}

func (e *endpointsAggregator) Run(stop <-chan struct{}) {
	wg := &sync.WaitGroup{}
	e.startEndpointDiscoveries(wg, stop)
	e.watchEdsForEndpoints(wg)
	e.aggregateReceivedEndpoints(wg, stop)
	e.aggregateErrors(wg)
	wg.Wait()
}

// start all the eds
func (e *endpointsAggregator) startEndpointDiscoveries(wg *sync.WaitGroup, stop <-chan struct{}) {
	for _, eds := range e.endpointDiscoveries {
		wg.Add(1)
		go func(stop <-chan struct{}) {
			defer wg.Done()
			eds.Run(stop)
		}(stop)
	}
}

// start a watcher goroutine for each endpoint discovery
func (e *endpointsAggregator) watchEdsForEndpoints(wg *sync.WaitGroup) {
	for _, ed := range e.endpointDiscoveries {
		wg.Add(1)
		go func(endpointDisc endpointdiscovery.Interface) {
			defer wg.Done()
			for endpoints := range endpointDisc.Endpoints() {
				e.endpointsByDiscovery <- endpointTuple{
					endpoints:    endpoints,
					discoveredBy: endpointDisc,
				}
			}
		}(ed)
	}
}

// start a goroutine that takes every new endpoints group received from an ed
// and aggregates them to a single set of EndpointGroups which get passed to our own channel
func (e *endpointsAggregator) aggregateReceivedEndpoints(wg *sync.WaitGroup, stop <-chan struct{}) {
	wg.Add(1)
	go func() {
		// watch eds for new sets of endpoints
		endpointsByDiscovery := make(map[endpointdiscovery.Interface]endpointdiscovery.EndpointGroups)
		defer wg.Done()
		for {
			select {
			case <-stop:
				log.Printf("stopping endpoints watcher")
				return
			case tuple := <- e.endpointsByDiscovery:
				// overwrite the last endpoints we got from this discovery
				endpointsByDiscovery[tuple.discoveredBy] = tuple.endpoints

				// aggregate all different endpoint groups
				aggregatedEndpointGroups := make(endpointdiscovery.EndpointGroups)
				for _, group := range endpointsByDiscovery {
					for upstreamName, endpointSet := range group {
						aggregatedEndpointGroups[upstreamName] = endpointSet
					}
				}

				// pass the finished endpoints down the channel
				e.aggregatedEndpoints <- aggregatedEndpointGroups
			}
		}
	}()
}

// aggregate all the errors from each ed
func (e *endpointsAggregator) aggregateErrors(wg *sync.WaitGroup) {
	for _, ed := range e.endpointDiscoveries {
		wg.Add(1)
		go func(endpointDisc endpointdiscovery.Interface) {
			defer wg.Done()
			for err := range endpointDisc.Error() {
				e.errors <- errors.Wrapf(err, "error in endpoint discovery %v", endpointDisc)
			}
		}(ed)
	}
}

func (e *endpointsAggregator) TrackUpstreams(upstreams []*v1.Upstream) {
	wg := &sync.WaitGroup{}
	for _, discovery := range e.endpointDiscoveries {
		wg.Add(1)
		go func(epd endpointdiscovery.Interface) {
			defer wg.Done()
			epd.TrackUpstreams(upstreams)
		}(discovery)
	}
	wg.Wait()
}

func (e *endpointsAggregator) Endpoints() <-chan endpointdiscovery.EndpointGroups {
	return e.aggregatedEndpoints
}

func (e *endpointsAggregator) Error() <-chan error {
	return e.errors
}

type endpointTuple struct {
	discoveredBy endpointdiscovery.Interface
	endpoints    endpointdiscovery.EndpointGroups
}
