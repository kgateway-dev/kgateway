package consul

import (
	consulapi "github.com/hashicorp/consul/api"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	skclients "github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

const notImplementedErrMsg = "this operation is not supported by this client"

// This client can list and watch Consul services. A Gloo upstream will be generated for each unique
// Consul service name. The Consul EDS will discover and characterize all endpoints for each one of
// these upstreams across the available data centers.
//
// NOTE: any method except List and Watch will panic!
func NewConsulUpstreamClient(consul ConsulWatcher) v1.UpstreamClient {
	return &consulUpstreamClient{consul: consul}
}

type consulUpstreamClient struct {
	consul ConsulWatcher
}

func (*consulUpstreamClient) BaseClient() skclients.ResourceClient {
	panic(notImplementedErrMsg)
}

func (*consulUpstreamClient) Register() error {
	panic(notImplementedErrMsg)
}

func (*consulUpstreamClient) Read(namespace, name string, opts skclients.ReadOpts) (*v1.Upstream, error) {
	panic(notImplementedErrMsg)
}

func (*consulUpstreamClient) Write(resource *v1.Upstream, opts skclients.WriteOpts) (*v1.Upstream, error) {
	panic(notImplementedErrMsg)
}

func (*consulUpstreamClient) Delete(namespace, name string, opts skclients.DeleteOpts) error {
	panic(notImplementedErrMsg)
}

// Represents the services registered in a data center
type dataCenterServicesTuple struct {
	dataCenter   string
	serviceNames []string
}

func (c *consulUpstreamClient) List(_ string, opts skclients.ListOpts) (v1.UpstreamList, error) {

	// Get a list of the available data centers
	dataCenters, err := c.consul.DataCenters()
	if err != nil {
		return nil, err
	}

	dcToSvcMap := make(dataCenterToServicesMap)
	for _, dataCenter := range dataCenters {

		// Get the names of all services in the data center
		serviceNamesAndTags, _, err := c.consul.Services(&consulapi.QueryOptions{Datacenter: dataCenter, RequireConsistent: true})
		if err != nil {
			return nil, err
		}

		// Ignore the tags
		for serviceName := range serviceNamesAndTags {
			dcToSvcMap[dataCenter] = append(dcToSvcMap[dataCenter], serviceName)
		}
	}

	return toUpstreamList(toServiceMetaSlice(dcToSvcMap)), nil
}

func (c *consulUpstreamClient) Watch(_ string, opts skclients.WatchOpts) (<-chan v1.UpstreamList, <-chan error, error) {
	dataCenters, err := c.consul.DataCenters()
	if err != nil {
		return nil, nil, err
	}

	servicesChan, errorChan := c.consul.WatchServices(opts.Ctx, dataCenters)

	upstreamsChan := make(chan v1.UpstreamList)
	go func() {
		for {
			select {
			case services, ok := <-servicesChan:
				if ok {
					//  Transform to upstreams
					upstreams := toUpstreamList(services)
					upstreamsChan <- upstreams
				}
			case <-opts.Ctx.Done():
				close(upstreamsChan)
				return
			}
		}
	}()

	return upstreamsChan, errorChan, nil
}
