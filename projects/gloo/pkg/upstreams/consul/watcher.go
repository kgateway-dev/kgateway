package consul

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/avast/retry-go"
	consulapi "github.com/hashicorp/consul/api"
	"github.com/mitchellh/hashstructure"
	glooconsul "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/consul"
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
	WatchServices(ctx context.Context, dataCenters []string, cm glooconsul.ConsulConsistencyModes) (<-chan []*ServiceMeta, <-chan error)
}

func NewConsulWatcher(client *consulapi.Client, dataCenters []string) (ConsulWatcher, error) {
	clientWrapper, err := NewConsulClient(client, dataCenters)
	if err != nil {
		return nil, err
	}
	return NewConsulWatcherFromClient(clientWrapper), nil
}

func NewConsulWatcherFromClient(client ConsulClient) ConsulWatcher {
	return &consulWatcher{client, make(map[string]*watchChannels), sync.Mutex{}}
}

var _ ConsulWatcher = &consulWatcher{}

type watchChannels struct {
	// list of subscribers to the parent channels
	servicesChans []chan []*ServiceMeta
	errChans      []chan error
}

type consulWatcher struct {
	ConsulClient
	serviceWatches map[string]*watchChannels
	lock           sync.Mutex
}

// Maps a data center name to the services (including tags) registered in it
type dataCenterServicesTuple struct {
	dataCenter string
	services   map[string][]string
}

func (c *consulWatcher) WatchServices(ctx context.Context, dataCenters []string, cm glooconsul.ConsulConsistencyModes) (<-chan []*ServiceMeta, <-chan error) {

	var (
		eg              errgroup.Group
		outputChan      = make(chan []*ServiceMeta)
		errorChan       = make(chan error)
		allServicesChan = make(chan *dataCenterServicesTuple)
	)

	// if all datacenters already have a watch, reuse to avoid duplicate watches
	// sort for idempotency
	sort.Strings(dataCenters)
	c.lock.Lock()
	key := strings.Join(dataCenters, ",")
	ctxHash, err := hashstructure.Hash(ctx, nil)
	if err != nil {
		c.lock.Unlock()
		errorChan <- err
		return outputChan, errorChan
	}
	key = fmt.Sprintf("%s-%v-%v", key, ctxHash, cm.Number())
	if watch, ok := c.serviceWatches[key]; ok {
		watch.servicesChans = append(watch.servicesChans, outputChan)
		watch.errChans = append(watch.errChans, errorChan)
		c.lock.Unlock()
		return outputChan, errorChan
	} else {
		c.serviceWatches[key] = &watchChannels{
			servicesChans: []chan []*ServiceMeta{outputChan},
			errChans:      []chan error{errorChan},
		}
	}
	c.lock.Unlock()

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
		c.lock.Lock()
		if watch, ok := c.serviceWatches[key]; ok {
			for _, errChan := range watch.errChans {
				close(errChan)
			}
		}
		c.lock.Unlock()
	}()
	servicesByDataCenter := make(map[string]*dataCenterServicesTuple)
	go func() {
		defer func() {
			c.lock.Lock()
			for _, serviceChan := range c.serviceWatches[key].servicesChans {
				close(serviceChan)
			}
			delete(c.serviceWatches, key)
			c.lock.Unlock()
		}()
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

				c.lock.Lock()
				for _, serviceChan := range c.serviceWatches[key].servicesChans {
					servicesMetaList := toServiceMetaSlice(services)
					serviceChan <- servicesMetaList
				}
				c.lock.Unlock()

			// case <-ctx.Done():

			// 	select {
			// 	case list, ok := <- servicesMetaList:

			// 		// case outputChan <- servicesMetaList:
			// 	case <-ctx.Done():
			// 		return
			// }
			case <-ctx.Done():
				return
			}
		}
	}()

	return outputChan, errorChan
}

// Honors the contract of Watch functions to open with an initial read.
func (c *consulWatcher) watchServicesInDataCenter(ctx context.Context, dataCenter string, cm glooconsul.ConsulConsistencyModes) (<-chan *dataCenterServicesTuple, <-chan error) {
	servicesChan := make(chan *dataCenterServicesTuple)
	errsChan := make(chan error)

	go func(dataCenter string) {
		defer close(servicesChan)
		defer close(errsChan)

		fmt.Printf("KDOROSH12 start watch for dc %v with ctx %v\n", dataCenter, ctx)
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
