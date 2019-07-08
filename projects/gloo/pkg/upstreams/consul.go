package upstreams

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/avast/retry-go"
	consulapi "github.com/hashicorp/consul/api"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	consulplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/consul"
	"github.com/solo-io/go-utils/errutils"
	skclients "github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const ConsulUpstreamNamePrefix = "consul-svc:"

// This client can list and watch Consul services. A Gloo upstream will be generated for each unique
// Consul service name. The Consul EDS will discover and characterize all endpoints for each one of
// these upstreams across the available data centers.
//
// NOTE: any method except List and Watch will panic!
func NewConsulUpstreamClient(consul ConsulClient) v1.UpstreamClient {
	return &consulUpstreamClient{consul: consul}
}

type consulUpstreamClient struct {
	consul ConsulClient
}

func (*consulUpstreamClient) BaseClient() skclients.ResourceClient {
	panic("this operation is not supported by this client")
}

func (*consulUpstreamClient) Register() error {
	panic("this operation is not supported by this client")
}

func (*consulUpstreamClient) Read(namespace, name string, opts skclients.ReadOpts) (*v1.Upstream, error) {
	panic("this operation is not supported by this client")
}

func (*consulUpstreamClient) Write(resource *v1.Upstream, opts skclients.WriteOpts) (*v1.Upstream, error) {
	panic("this operation is not supported by this client")
}

func (*consulUpstreamClient) Delete(namespace, name string, opts skclients.DeleteOpts) error {
	panic("this operation is not supported by this client")
}

// Represents the services registered in a data center
type dataCenterServicesTuple struct {
	dataCenter   string
	serviceNames []string
}

// Maps service names to the data centers in which the service is registered
type serviceToDataCentersMap map[string][]string

// Maps a data center name to thee services registered in it
type dataCenterToServicesMap map[string][]string

func (c *consulUpstreamClient) List(_ string, opts skclients.ListOpts) (v1.UpstreamList, error) {

	// TODO: add consul.dataCenters option to settings to restrict which data centers to consider
	// Get a list of the available data centers
	dataCenters, err := c.consul.DataCenters()
	if err != nil {
		return nil, err
	}

	servicesWithDc := make(serviceToDataCentersMap)
	for _, dataCenter := range dataCenters {

		// Get the names of all services in the data center
		serviceNamesAndTags, _, err := c.consul.Services(&consulapi.QueryOptions{Datacenter: dataCenter, RequireConsistent: true})
		if err != nil {
			return nil, err
		}

		// Ignore the tags
		for serviceName := range serviceNamesAndTags {
			servicesWithDc[serviceName] = append(servicesWithDc[serviceName], dataCenter)
		}
	}

	return toUpstreamList(servicesWithDc), nil
}

func (c *consulUpstreamClient) Watch(_ string, opts skclients.WatchOpts) (<-chan v1.UpstreamList, <-chan error, error) {

	dataCenters, err := c.consul.DataCenters()
	if err != nil {
		return nil, nil, err
	}

	upstreamsChan := make(chan v1.UpstreamList)
	errorChan := make(chan error)
	allServicesChan := make(chan *dataCenterServicesTuple)

	var wg sync.WaitGroup
	for _, dataCenter := range dataCenters {

		dataCenterServicesChan, errChan := c.watchServicesInDataCenter(opts.Ctx, dataCenter)

		// Collect services
		wg.Add(1)
		go func() {
			defer wg.Done()
			aggregateServices(opts.Ctx, allServicesChan, dataCenterServicesChan)
		}()

		// Collect errors
		wg.Add(1)
		go func(dataCenter string) {
			defer wg.Done()
			errutils.AggregateErrs(opts.Ctx, errorChan, errChan, "data center: "+dataCenter)
		}(dataCenter)
	}

	dcToSvcMap := make(dataCenterToServicesMap)
	go func() {
		for {
			select {
			case dataCenterServices, ok := <-allServicesChan:
				if ok {
					dcToSvcMap[dataCenterServices.dataCenter] = dataCenterServices.serviceNames

					//  Transform to upstreams
					serviceMap := indexByService(dcToSvcMap)
					upstreams := toUpstreamList(serviceMap)

					upstreamsChan <- upstreams
				}
			case <-opts.Ctx.Done():
				close(upstreamsChan)

				// Wait for the aggregation routines to shut down to avoid writing to closed channels
				wg.Wait()
				close(allServicesChan)
				close(errorChan)
				return
			}
		}
	}()

	return upstreamsChan, errorChan, nil
}

// Honors the contract of Watch functions to open with an initial read.
func (c *consulUpstreamClient) watchServicesInDataCenter(ctx context.Context, dataCenter string) (<-chan *dataCenterServicesTuple, <-chan error) {
	servicesChan := make(chan *dataCenterServicesTuple)
	errors := make(chan error)

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
						services, queryMeta, err = c.consul.Services((&consulapi.QueryOptions{
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
					errors <- err
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
				close(errors)
				return
			}
		}
	}(dataCenter)

	return servicesChan, errors
}

// Create an upstream for each service in the map
func toUpstreamList(services serviceToDataCentersMap) v1.UpstreamList {
	var upstreams v1.UpstreamList
	for serviceName, dataCenters := range services {
		sort.Strings(dataCenters)
		upstreams = append(upstreams, &v1.Upstream{
			Metadata: core.Metadata{
				Name:      ConsulUpstreamNamePrefix + serviceName,
				Namespace: "", // no namespace
			},
			UpstreamSpec: &v1.UpstreamSpec{
				UpstreamType: &v1.UpstreamSpec_Consul{
					Consul: &consulplugin.UpstreamSpec{
						ServiceName: serviceName,
						DataCenters: dataCenters,
					},
				},
			},
		})
	}
	return upstreams
}

func indexByService(dcToSvcMap dataCenterToServicesMap) serviceToDataCentersMap {
	result := make(serviceToDataCentersMap)
	for dataCenter, services := range dcToSvcMap {
		for _, svc := range services {
			result[svc] = append(result[svc], dataCenter)
		}
	}
	return result
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
