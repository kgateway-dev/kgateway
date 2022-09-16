package consul

import (
	"context"
	"time"

	"github.com/avast/retry-go"
	consulapi "github.com/hashicorp/consul/api"
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
	ClientWrapper
	WatchServices(ctx context.Context, dataCenters []string, cm glooconsul.ConsulConsistencyModes, queryOpts *glooconsul.QueryOptions) (<-chan []*ServiceMeta, <-chan error)
}

func NewConsulWatcher(client *consulapi.Client, dataCenters []string, serviceTagsAllowlist []string) (ConsulWatcher, error) {

	clientWrapper, err := NewFilteredConsulClient(NewConsulClientWrapper(client), dataCenters, serviceTagsAllowlist)
	if err != nil {
		return nil, err
	}
	return NewConsulWatcherFromClient(clientWrapper), nil
}

func NewConsulWatcherFromClient(client ClientWrapper) ConsulWatcher {
	return &consulWatcher{client}
}

var _ ConsulWatcher = &consulWatcher{}

type consulWatcher struct {
	ClientWrapper
}

// Maps a data center name to the services (including tags) registered in it
type dataCenterServicesTuple struct {
	dataCenter string
	services   map[string][]string
}

func (c *consulWatcher) WatchServices(ctx context.Context, dataCenters []string, cm glooconsul.ConsulConsistencyModes, queryOpts *glooconsul.QueryOptions) (<-chan []*ServiceMeta, <-chan error) {

	var (
		eg              errgroup.Group
		outputChan      = make(chan []*ServiceMeta)
		errorChan       = make(chan error)
		allServicesChan = make(chan *dataCenterServicesTuple)
	)

	for _, dataCenter := range dataCenters {
		// Copy before passing to goroutines!
		dcName := dataCenter

		dataCenterServicesChan, errChan := c.watchServicesInDataCenter(ctx, dcName, cm, queryOpts)

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

	return outputChan, errorChan
}

// Honors the contract of Watch functions to open with an initial read.
func (c *consulWatcher) watchServicesInDataCenter(ctx context.Context, dataCenter string, cm glooconsul.ConsulConsistencyModes, queryOpts *glooconsul.QueryOptions) (<-chan *dataCenterServicesTuple, <-chan error) {
	servicesChan := make(chan *dataCenterServicesTuple)
	errsChan := make(chan error)

	go func(dataCenter string) {
		defer close(servicesChan)
		defer close(errsChan)

		lastIndex := uint64(0)

		for {
			select {
			case <-ctx.Done():
				return
			default:

				var (
					services  map[string][]string
					queryMeta *consulapi.QueryMeta
				)

				// This is a blocking query (see [here](https://www.consul.io/api/features/blocking.html) for more info)
				// The first invocation (with lastIndex equal to zero) will return immediately
				queryOpts := NewConsulServicesQueryOptions(dataCenter, cm, queryOpts)
				queryOpts.WaitIndex = lastIndex

				ctxDead := false

				// Use a back-off retry strategy to avoid flooding the error channel
				err := retry.Do(
					func() error {
						var err error
						if ctx.Err() != nil {
							// intentionally return early if context is already done
							// this is a backoff loop; by the time we get here ctx may be done
							ctxDead = true
							return nil
						}
						services, queryMeta, err = c.Services(queryOpts.WithContext(ctx))
						return err
					},
					retry.Attempts(6),
					//  Last delay is 2^6 * 100ms = 3.2s
					retry.Delay(100*time.Millisecond),
					retry.DelayType(retry.BackOffDelay),
				)

				if ctxDead {
					return
				}

				if err != nil {
					errsChan <- err
					continue
				}

				// If index is the same, there have been no changes since last query
				// since this follows the raft index, this can also change even if the services / tags do not;
				// in fact, we depend on this (which is tested in "fires service watch even if catalog service is the only update")
				if queryMeta.LastIndex == lastIndex {
					continue
				}
				tuple := &dataCenterServicesTuple{
					dataCenter: dataCenter,
					services:   services,
				}

				// Update the last index
				if queryMeta.LastIndex < lastIndex {
					// update if index goes backwards per consul blocking query docs
					// this can happen e.g. KV list operations where item with highest index is deleted
					// for more, see https://www.consul.io/api-docs/features/blocking#implementation-details
					lastIndex = 0
				} else {
					lastIndex = queryMeta.LastIndex
				}

				servicesChan <- tuple
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
