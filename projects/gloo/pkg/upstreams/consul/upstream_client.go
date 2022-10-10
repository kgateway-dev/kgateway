package consul

import (
	"context"
	"fmt"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams/NoOpUpstreamClient"
	"github.com/solo-io/go-utils/contextutils"
	skclients "github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

const notImplementedErrMsg = "this operation is not supported by this client"

// This client can list and watch Consul services. A Gloo upstream will be generated for each unique
// Consul service name. The Consul EDS will discover and characterize all endpoints for each one of
// these upstreams across the available data centers.
//
// NOTE: any method except List and Watch will panic!
func NewConsulUpstreamClient(consul ConsulWatcher, consulDiscoveryConfig *v1.Settings_ConsulUpstreamDiscoveryConfiguration) v1.UpstreamClient {
	return &consulUpstreamClient{consul: consul, consulUpstreamDiscoveryConfig: consulDiscoveryConfig}
}

type consulUpstreamClient struct {
	consul                        ConsulWatcher
	consulUpstreamDiscoveryConfig *v1.Settings_ConsulUpstreamDiscoveryConfiguration
}

func (*consulUpstreamClient) BaseClient() skclients.ResourceClient {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return &NoOpUpstreamClient.NoOpUpstreamClient{}
}

func (*consulUpstreamClient) Register() error {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return fmt.Errorf(notImplementedErrMsg)
}

func (*consulUpstreamClient) Read(namespace, name string, opts skclients.ReadOpts) (*v1.Upstream, error) {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return nil, fmt.Errorf(notImplementedErrMsg)
}

func (*consulUpstreamClient) Write(resource *v1.Upstream, opts skclients.WriteOpts) (*v1.Upstream, error) {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return nil, fmt.Errorf(notImplementedErrMsg)
}

func (*consulUpstreamClient) Delete(namespace, name string, opts skclients.DeleteOpts) error {
	contextutils.LoggerFrom(context.Background()).DPanic(notImplementedErrMsg)
	return fmt.Errorf(notImplementedErrMsg)
}

func (c *consulUpstreamClient) List(namespace string, opts skclients.ListOpts) (v1.UpstreamList, error) {
	// Get a list of the available data centers
	dataCenters, err := c.consul.DataCenters()
	if err != nil {
		return nil, err
	}

	var services []*dataCenterServicesTuple
	for _, dataCenter := range dataCenters {

		cm := c.consulUpstreamDiscoveryConfig.GetConsistencyMode()
		qopts := c.consulUpstreamDiscoveryConfig.GetQueryOptions()
		queryOpts := NewConsulServicesQueryOptions(dataCenter, cm, qopts)

		serviceNamesAndTags, _, err := c.consul.Services(queryOpts.WithContext(opts.Ctx))
		if err != nil {
			return nil, err
		}

		services = append(services, &dataCenterServicesTuple{
			dataCenter: dataCenter,
			services:   serviceNamesAndTags,
		})
	}

	return toUpstreamList(namespace, toServiceMetaSlice(services), c.consulUpstreamDiscoveryConfig), nil
}

func (c *consulUpstreamClient) Watch(namespace string, opts skclients.WatchOpts) (<-chan v1.UpstreamList, <-chan error, error) {
	dataCenters, err := c.consul.DataCenters()
	if err != nil {
		return nil, nil, err
	}

	upstreamDiscoveryConfig := c.consulUpstreamDiscoveryConfig
	qopts := c.consulUpstreamDiscoveryConfig.GetQueryOptions()
	servicesChan, errorChan := c.consul.WatchServices(opts.Ctx, dataCenters, upstreamDiscoveryConfig.GetConsistencyMode(), qopts)

	upstreamsChan := make(chan v1.UpstreamList)
	go func() {
		for {
			select {
			case services, ok := <-servicesChan:
				if ok {
					//  Transform to upstreams
					upstreams := toUpstreamList(namespace, services, upstreamDiscoveryConfig)
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
