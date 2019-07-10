package consul

import (
	"context"
	"sort"
	"time"

	"github.com/avast/retry-go"
	consulapi "github.com/hashicorp/consul/api"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/errutils"
	"golang.org/x/sync/errgroup"
)

type ServiceMeta struct {
	Name        string
	DataCenters []string
}

type Service struct {
	ServiceMeta
	Tags []string
}

type ConsulWatcher interface {
	ConsulClient
	WatchServices(ctx context.Context, dataCenters []string) (<-chan []ServiceMeta, <-chan error)
}

func NewConsulWatcher(settings *v1.Settings) (ConsulWatcher, error) {
	client, err := NewConsulClient(settings)
	if err != nil {
		return nil, err
	}
	return &consulWatcher{client}, nil
}

func NewConsulWatcherFromClient(client ConsulClient) ConsulWatcher {
	return &consulWatcher{client}
}

type consulWatcher struct {
	ConsulClient
}

// Maps a data center name to the services registered in it
type dataCenterToServicesMap map[string][]string

// Maps a service name to the data centers in which the service is registered
type serviceToDataCentersMap map[string][]string

// This returns only the names
func (c *consulWatcher) WatchServices(ctx context.Context, dataCenters []string) (<-chan []ServiceMeta, <-chan error) {

	var (
		eg              errgroup.Group
		outputChan      = make(chan []ServiceMeta)
		errorChan       = make(chan error)
		allServicesChan = make(chan *dataCenterServicesTuple)
	)

	for _, dataCenter := range dataCenters {

		// Alias before passing to goroutines
		dcName := dataCenter

		dataCenterServicesChan, errChan := c.watchServicesInDataCenter(ctx, dcName)

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

	dcToSvcMap := make(dataCenterToServicesMap)
	go func() {
		for {
			select {
			case dataCenterServices, ok := <-allServicesChan:
				if ok {
					dcToSvcMap[dataCenterServices.dataCenter] = dataCenterServices.serviceNames

					services := toServiceMetaSlice(dcToSvcMap)

					outputChan <- services
				}
			case <-ctx.Done():
				close(outputChan)

				// Wait for the aggregation routines to shut down to avoid writing to closed channels
				_ = eg.Wait() // will never error
				close(allServicesChan)
				close(errorChan)
				return
			}
		}
	}()
	return outputChan, errorChan
}

// Honors the contract of Watch functions to open with an initial read.
func (c *consulWatcher) watchServicesInDataCenter(ctx context.Context, dataCenter string) (<-chan *dataCenterServicesTuple, <-chan error) {
	servicesChan := make(chan *dataCenterServicesTuple)
	errsChan := make(chan error)

	go func(dataCenter string) {
		lastIndex := uint64(0)

		for {
			select {
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
						services, queryMeta, err = c.Services((&consulapi.QueryOptions{
							Datacenter:        dataCenter,
							RequireConsistent: true,
							WaitIndex:         lastIndex,
						}).WithContext(ctx))

						return err
					},
					retry.Attempts(5),
					retry.Delay(1*time.Second),
					retry.DelayType(retry.BackOffDelay),
				)

				if err != nil {
					errsChan <- err
					continue
				}

				// If index is the same, there have been no changes since last query
				if queryMeta.LastIndex == lastIndex {
					continue
				}

				newServices := make([]string, 0, len(services))
				for serviceName := range services {
					newServices = append(newServices, serviceName)
				}
				sort.Strings(newServices)

				servicesChan <- &dataCenterServicesTuple{
					dataCenter:   dataCenter,
					serviceNames: newServices,
				}

				// Update the last index
				lastIndex = queryMeta.LastIndex

			case <-ctx.Done():
				close(servicesChan)
				close(errsChan)
				return
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

func toServiceMetaSlice(dcToSvcMap dataCenterToServicesMap) []ServiceMeta {
	var result []ServiceMeta
	for serviceName, dataCenters := range indexByService(dcToSvcMap) {
		sort.Strings(dataCenters)
		result = append(result, ServiceMeta{Name: serviceName, DataCenters: dataCenters})
	}
	return result
}

func indexByService(dcToSvcMap dataCenterToServicesMap) serviceToDataCentersMap {
	result := make(map[string][]string)
	for dataCenter, services := range dcToSvcMap {
		for _, svc := range services {
			result[svc] = append(result[svc], dataCenter)
		}
	}
	return result
}
