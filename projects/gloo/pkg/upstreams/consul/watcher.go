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
	ConsulClient
	WatchServices(ctx context.Context, dataCenters []string, cm glooconsul.ConsulConsistencyModes, queryOpts *glooconsul.QueryOptions) (<-chan []*ServiceMeta, <-chan error)
}

func NewConsulWatcher(client *consulapi.Client, dataCenters []string) (ConsulWatcher, error) {
	clientWrapper, err := NewConsulClient(client, dataCenters)
	if err != nil {
		return nil, err
	}
	return NewConsulWatcherFromClient(clientWrapper), nil
}

func NewConsulWatcherFromClient(client ConsulClient) ConsulWatcher {
	return &consulWatcher{client}
}

var _ ConsulWatcher = &consulWatcher{}

type consulWatcher struct {
	ConsulClient
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
func (c *consulWatcher) watchServicesInDataCenter(ctx context.Context, dataCenter string, cm glooconsul.ConsulConsistencyModes, qopts *glooconsul.QueryOptions) (<-chan *dataCenterServicesTuple, <-chan error) {
	servicesChan := make(chan *dataCenterServicesTuple)
	errsChan := make(chan error)

	go func(dataCenter string) {
		defer close(servicesChan)
		defer close(errsChan)

		lastIndex := uint64(0)
		lastHardQuery := time.Now()

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
				queryOpts := NewConsulQueryOptions(dataCenter, cm, qopts)
				queryOpts.WaitIndex = lastIndex

				if queryOpts.UseCache {
					time.Sleep(1 * time.Second) // wait before making next cache query, to avoid spamming the cache
				}
				var retryIf retry.RetryIfFunc
				retryIf = func(err error) bool {
					return err != nil
				}
				var onRetry retry.OnRetryFunc
				onRetry = func(n uint, err error) {
					if queryOpts.UseCache {
						time.Sleep(1 * time.Second) // wait before making next cache query, to avoid spamming the cache
					}
				}

				// Use a back-off retry strategy to avoid flooding the error channel
				err := retry.Do(
					func() error {
						var err error
						if ctx.Err() != nil {
							// ctx dead, return
							return nil
						}

						if now := time.Now(); queryOpts.UseCache && now.Sub(lastHardQuery) > 5*time.Second {
							// seems to be a bug; let's hard refresh to ensure we have latest cached at least once every 5s
							queryOpts.UseCache = false
							lastHardQuery = now
						}

						services, queryMeta, err = c.Services(queryOpts.WithContext(ctx))
						return err
					},
					retry.RetryIf(retryIf),
					retry.OnRetry(onRetry),
					retry.Attempts(6),
					//  Last delay is 2^6 * 100ms = 3.2s
					retry.Delay(100*time.Millisecond),
					retry.DelayType(retry.BackOffDelay),
				)

				if ctx.Err() != nil {
					// ctx dead, return
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

				// Update the last index
				if queryMeta.LastIndex < lastIndex {
					// update if index goes backwards per consul blocking query docs
					// this can happen e.g. KV list operations where item with highest index is deleted
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
