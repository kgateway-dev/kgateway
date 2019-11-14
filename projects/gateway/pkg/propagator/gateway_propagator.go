package propagator

import (
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/propagator"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
)

type Propagator struct {
	controller  string
	gwClient    v1.GatewayClient
	vsClient    v1.VirtualServiceClient
	proxyClient gloov1.ProxyClient
	writeErrs   chan error
}

func NewPropagator(controller string, gwClient v1.GatewayClient, vsClient v1.VirtualServiceClient, proxyClient gloov1.ProxyClient, writeErrs chan error) *Propagator {
	return &Propagator{
		controller:  controller,
		gwClient:    gwClient,
		vsClient:    vsClient,
		proxyClient: proxyClient,
		writeErrs:   writeErrs,
	}
}

func (p *Propagator) PropagateStatuses(snap *v1.ApiSnapshot,
	proxy *gloov1.Proxy,
	opts clients.WatchOpts) error {
	parents := append(snap.Gateways.AsInputResources(), snap.VirtualServices.AsInputResources()...)
	rcs := make(clients.ResourceClients)
	// this is where buggy things happen
	// would generics really solved this problem?
	rcs.Add(p.gwClient.BaseClient(), p.vsClient.BaseClient(), p.proxyClient.BaseClient())
	return propagator.NewPropagator(p.controller, parents, resources.InputResourceList{proxy}, rcs, p.writeErrs).PropagateStatuses(opts)
}
