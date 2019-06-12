package upstreams

import (
	"github.com/solo-io/go-utils/errors"
	"sync"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/errutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
)

var UnableToRetrieveErr = func(err error, namespace, name string) error {
	return errors.Wrapf(err, "unable to retrieve service-derived upstream %s.%s", namespace, name)
}

func NewHybridUpstreamClient(
	upstreamClient v1.UpstreamClient,
	serviceClient skkube.ServiceClient) (v1.UpstreamClient, error) {

	return &hybridUpstreamClient{
		upstreamClient: upstreamClient,
		serviceClient:  serviceClient,
	}, nil
}

type hybridUpstreamClient struct {
	upstreamClient v1.UpstreamClient
	serviceClient  skkube.ServiceClient
}

func (c *hybridUpstreamClient) BaseClient() clients.ResourceClient {
	// We need this modified base client to build reporters, which require generic clients.ResourceClient
	return NewReadOnlyBaseClient(c.upstreamClient.BaseClient())
}

func (c *hybridUpstreamClient) Register() error {
	return nil
}

func (c *hybridUpstreamClient) Read(namespace, name string, opts clients.ReadOpts) (*v1.Upstream, error) {
	if isRealUpstream(name) {
		return c.upstreamClient.Read(namespace, name, opts)
	}

	serviceName, _, err := reconstructServiceName(name)
	if err != nil {
		return nil, err
	}
	service, err := c.serviceClient.Read(namespace, serviceName, opts)
	if err != nil {
		return nil, UnableToRetrieveErr(err, namespace, name)
	}
	for _, us := range servicesToUpstreams(skkube.ServiceList{service}) {
		if us.Metadata.Name == name {
			return us, nil
		}
	}
	return nil, UnableToRetrieveErr(err, namespace, name)
}

func (c *hybridUpstreamClient) Write(resource *v1.Upstream, opts clients.WriteOpts) (*v1.Upstream, error) {
	if isRealUpstream(resource.Metadata.Name) {
		return c.upstreamClient.Write(resource, opts)
	}
	return resource, nil
}

func (c *hybridUpstreamClient) Delete(namespace, name string, opts clients.DeleteOpts) error {
	if isRealUpstream(name) {
		return c.upstreamClient.Delete(namespace, name, opts)
	}
	return nil
}

func (c *hybridUpstreamClient) List(namespace string, opts clients.ListOpts) (v1.UpstreamList, error) {
	realUpstreams, err := c.upstreamClient.List(namespace, opts)
	if err != nil {
		return nil, err
	}
	services, err := c.serviceClient.List(namespace, opts)
	if err != nil {
		return nil, err
	}
	return append(realUpstreams, servicesToUpstreams(services)...), nil
}

func (c *hybridUpstreamClient) Watch(namespace string, opts clients.WatchOpts) (<-chan v1.UpstreamList, <-chan error, error) {
	opts = opts.WithDefaults()
	ctx := opts.Ctx

	// Start watching upstreams
	usChan, usErrChan, initErr := c.upstreamClient.Watch(namespace, opts)
	if initErr != nil {
		return nil, nil, initErr
	}

	// Start watching services
	svcChan, svcErrChan, initErr := c.serviceClient.Watch(namespace, opts)
	if initErr != nil {
		return nil, nil, initErr
	}

	// Aggregate errors
	var done sync.WaitGroup
	errs := make(chan error)

	done.Add(1)
	go func() {
		defer done.Done()
		errutils.AggregateErrs(ctx, errs, usErrChan, "upstreams")
	}()

	done.Add(1)
	go func() {
		defer done.Done()
		errutils.AggregateErrs(ctx, errs, svcErrChan, "services")
	}()

	// Aggregate watches
	upstreamsOut := make(chan v1.UpstreamList)
	go func() {
		previous := hybridUpstreamSnapshot{}
		current := previous.Clone()
		syncFunc := func() {
			if current.Hash() == previous.Hash() {
				return
			}
			previous = current.Clone()
			toSend := current.Clone()
			upstreamsOut <- toSend.ToList()
		}

		for {
			select {
			case <-ctx.Done():
				close(upstreamsOut)
				done.Wait()
				close(errs)
				return
			case upstreamList, ok := <-usChan:
				if ok {
					current.SetRealUpstreams(upstreamList)
					syncFunc()
				}
			case serviceList, ok := <-svcChan:
				if ok {
					convertedUpstreams := servicesToUpstreams(serviceList)
					current.SetServiceUpstreams(convertedUpstreams)
					syncFunc()
				}
			}
		}
	}()

	return upstreamsOut, errs, nil
}
