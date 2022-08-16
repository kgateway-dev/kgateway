package consul

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/avast/retry-go"
	consulapi "github.com/hashicorp/consul/api"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/errutils"
	"golang.org/x/sync/errgroup"
)

//go:generate mockgen -destination ./mocks/mock_watcher.go -source watcher.go -aux_files github.com/solo-io/gloo/projects/gloo/pkg/upstreams/consul=./consul_client.go

// Data for a single consul service (not serviceInstance)
type ServiceMeta struct {
	Name        string
	DataCenters []string
	Tags        []string
}

type ConsulWatcher interface {
	ConsulClient
	WatchServices(ctx context.Context, dataCenters []string, cm v1.Settings_ConsulUpstreamDiscoveryConfiguration_ConsulConsistencyModes) (<-chan []*ServiceMeta, <-chan error)
}

func NewConsulWatcher(client *consulapi.Client, dataCenters []string) (ConsulWatcher, error) {
	clientWrapper, err := NewConsulClient(client, dataCenters)
	if err != nil {
		return nil, err
	}
	return NewConsulWatcherFromClient(clientWrapper), nil
}

func NewConsulWatcherFromClient(client ConsulClient) ConsulWatcher {
	return &consulWatcher{client, make(map[string]watchChannels)}
}

var _ ConsulWatcher = &consulWatcher{}

type watchChannels struct {
	// parent channel. each read off this channel will be duplicated and sent to each subscriber
	servicesChan <-chan []*ServiceMeta
	errChan      <-chan error
	// list of subscribers to the parent channels
	childServicesChans []chan []*ServiceMeta
	childErrChans      []chan error
}

type consulWatcher struct {
	ConsulClient
	serviceWatches map[string]watchChannels
}

// Maps a data center name to the services (including tags) registered in it
type dataCenterServicesTuple struct {
	dataCenter string
	services   map[string][]string
}

func (c *consulWatcher) WatchServices(ctx context.Context, dataCenters []string, cm v1.Settings_ConsulUpstreamDiscoveryConfiguration_ConsulConsistencyModes) (<-chan []*ServiceMeta, <-chan error) {

	var (
		eg              errgroup.Group
		outputChan      = make(chan []*ServiceMeta)
		errorChan       = make(chan error)
		allServicesChan = make(chan *dataCenterServicesTuple)
	)

	// if all datacenters already have a watch, reuse to avoid duplicate watches
	key := strings.Join(dataCenters, ",")
	if watch, ok := c.serviceWatches[key]; ok {
		// create a new channel for the new subscriber
		newServicesChan := make(chan []*ServiceMeta)
		newErrChan := make(chan error)
		watch.childServicesChans = append(watch.childServicesChans, newServicesChan)
		watch.childErrChans = append(watch.childErrChans, newErrChan)
		return newServicesChan, newErrChan
	}

	for _, dataCenter := range dataCenters {
		// Copy before passing to goroutines!
		dcName := dataCenter

		dataCenterServicesChan, errChan := c.watchServicesInDataCenter(ctx, dcName, cm)

		// Collect services
		eg.Go(func() error {
			aggregateServices(ctx, allServicesChan, dataCenterServicesChan)
			return nil
		})

		// Collect errors
		eg.Go(func() error {
			errutils.AggregateErrs(ctx, errorChan, errChan, "data center: "+dcName)
			return nil
		})
	}

	go func() {
		// Wait for the aggregation routines to shut down to avoid writing to closed channels
		_ = eg.Wait() // will never error
		close(allServicesChan)
		close(errorChan)
	}()
	servicesByDataCenter := make(map[string]*dataCenterServicesTuple)
	go func() {
		defer close(outputChan)
		for {
			select {
			case dataCenterServices, ok := <-allServicesChan:
				if !ok {
					return
				}
				servicesByDataCenter[dataCenterServices.dataCenter] = dataCenterServices

				var services []*dataCenterServicesTuple
				for _, s := range servicesByDataCenter {
					services = append(services, s)
				}

				servicesMetaList := toServiceMetaSlice(services)

				select {
				case outputChan <- servicesMetaList:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	newServicesChan := make(chan []*ServiceMeta)
	newErrChan := make(chan error)

	c.serviceWatches[key] = watchChannels{
		servicesChan:       outputChan,
		errChan:            errorChan,
		childServicesChans: []chan []*ServiceMeta{newServicesChan},
		childErrChans:      []chan error{newErrChan},
	}

	go func() {
		for {
			select {
			case services, ok := <-outputChan:
				if !ok {
					return
				}
				for _, childChan := range c.serviceWatches[key].childServicesChans {
					copy := services
					childChan <- copy
				}
			case err, ok := <-errorChan:
				if !ok {
					return
				}
				for _, childChan := range c.serviceWatches[key].childErrChans {
					copy := err
					childChan <- copy
				}
			case <-ctx.Done():
				// Wait for the aggregation routines to shut down to avoid writing to closed channels
				_ = eg.Wait() // will never error
				for _, childChan := range c.serviceWatches[key].childServicesChans {
					close(childChan)
				}
				for _, childChan := range c.serviceWatches[key].childErrChans {
					close(childChan)
				}
				return
			}
		}
	}()

	return newServicesChan, newErrChan
}

// Honors the contract of Watch functions to open with an initial read.
func (c *consulWatcher) watchServicesInDataCenter(ctx context.Context, dataCenter string, cm v1.Settings_ConsulUpstreamDiscoveryConfiguration_ConsulConsistencyModes) (<-chan *dataCenterServicesTuple, <-chan error) {
	servicesChan := make(chan *dataCenterServicesTuple)
	errsChan := make(chan error)

	go func(dataCenter string) {
		defer close(servicesChan)
		defer close(errsChan)

		fmt.Printf("KDOROSH12 start watch for dc %v\n", dataCenter)
		defer fmt.Printf("KDOROSH12 done with watch for dc %v\n", dataCenter)

		lastIndex := uint64(0)

		for {
			select {

			case <-ctx.Done():
				// fmt.Printf("KDOROSH12 shut down outer, now %v\n", time.Now())
				return

			default:

				var (
					services  map[string][]string
					queryMeta *consulapi.QueryMeta
				)

				// Use a back-off retry strategy to avoid flooding the error channel
				err := retry.Do(
					func() error {
						var err error

						// This is a blocking query (see [here](https://www.consul.io/api/features/blocking.html) for more info)
						// The first invocation (with lastIndex equal to zero) will return immediately
						// fmt.Printf("KDOROSH12 before lastindex %v now %v\n", lastIndex, time.Now())
						queryOpts := NewConsulQueryOptions(dataCenter, cm)
						queryOpts.WaitIndex = lastIndex

						if ctx.Err() != nil {
							// ctx dead, return
							fmt.Printf("KDOROSH12 ctx dead, now %v\n", time.Now())
							return nil
						}

						services, queryMeta, err = c.Services(queryOpts.WithContext(ctx))
						// fmt.Printf("KDOROSH12 after lastindex %v now %v\n", lastIndex, time.Now())
						return err
					},
					retry.Attempts(6),
					//  Last delay is 2^6 * 100ms = 3.2s
					retry.Delay(100*time.Millisecond),
					retry.DelayType(retry.BackOffDelay),
				)

				if ctx.Err() != nil {
					return
				}

				if err != nil {
					errsChan <- err
					continue
				}

				// If index is the same, there have been no changes since last query
				if queryMeta.LastIndex == lastIndex {
					continue
				}
				tuple := &dataCenterServicesTuple{
					dataCenter: dataCenter,
					services:   services,
				}

				select {
				case servicesChan <- tuple:
				case <-ctx.Done():
					fmt.Printf("KDOROSH12 shut down inner, now %v\n", time.Now())
					return
				}
				// Update the last index
				if queryMeta.LastIndex < lastIndex {
					// update if index goes backwards per consul blocking query docs
					// this can happen e.g. KV list operations where item with highest index is deleted
					lastIndex = 0
				} else {
					lastIndex = queryMeta.LastIndex
				}
			}
		}
	}(dataCenter)

	return servicesChan, errsChan
}

func aggregateServices(ctx context.Context, dest chan *dataCenterServicesTuple, src <-chan *dataCenterServicesTuple) {
	for {
		select {
		case services, ok := <-src:
			if !ok {
				return
			}
			select {
			case <-ctx.Done():
				return
			case dest <- services:
			}
		case <-ctx.Done():
			return
		}
	}
}
