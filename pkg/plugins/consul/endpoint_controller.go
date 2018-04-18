package consul

import (
	"fmt"
	"sort"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
	kubev1resources "k8s.io/api/core/v1"

	"github.com/mitchellh/hashstructure"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/pkg/log"
	"github.com/solo-io/kubecontroller"
)

type endpointController struct {
	endpoints     chan endpointdiscovery.EndpointGroups
	errors        chan error
	consul        *api.Client
	upstreamSpecs map[string]*UpstreamSpec
	runFunc       func(stop <-chan struct{})
	lastSeen      uint64
}

func newEndpointController(cfg *api.Config, syncPeriod time.Duration) (*endpointController, error) {
	client, err := api.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create consul client: %v", err)
	}
	c := &endpointController{
		endpoints: make(chan endpointdiscovery.EndpointGroups),
		errors:    make(chan error),
		consul:    client,
	}

	kubeController := kubecontroller.NewController("gloo-endpoints-controller",
		kubeClient,
		kubecontroller.NewSyncHandler(c.syncEndpoints),
		endpointInformer.Informer(),
		serviceInformer.Informer(),
		podInformer.Informer())

	c.runFunc = func(stop <-chan struct{}) {
		go informerFactory.Start(stop)
		go kubeController.Run(2, stop)
		// refresh every minute
		tick := time.Tick(time.Minute)
		go func() {
			for {
				select {
				case <-tick:
					c.syncEndpoints()
				case <-stop:
					return
				}
			}
		}()
		<-stop
		log.Printf("kube endpoint discovery stopped")
	}

	return c, nil
}

func (c *endpointController) Run(stop <-chan struct{}) {
	c.runFunc(stop)
}

// triggers an update
func (c *endpointController) TrackUpstreams(upstreams []*v1.Upstream) {
	//flush stale upstreams
	c.upstreamSpecs = make(map[string]*UpstreamSpec)
	for _, us := range upstreams {
		if us.Type != UpstreamTypeConsul {
			continue
		}
		spec, err := DecodeUpstreamSpec(us.Spec)
		if err != nil {
			log.Warnf("error in consul plugin endpoint controller: %v", err)
			continue
		}
		c.upstreamSpecs[us.Name] = spec
	}
	c.syncEndpoints()
}

func (c *endpointController) Endpoints() <-chan endpointdiscovery.EndpointGroups {
	return c.endpoints
}

func (c *endpointController) Error() <-chan error {
	return c.errors
}

// pushes EndpointGroups or error to channel
func (c *endpointController) syncEndpoints() {
	endpointGroups, err := c.getUpdatedEndpoints()
	if err != nil {
		c.errors <- err
		return
	}
	// ignore empty configs / no secrets to watch
	if len(endpointGroups) == 0 {
		return
	}
	c.endpoints <- endpointGroups
}

// retrieves secrets from kubernetes
func (c *endpointController) getUpdatedEndpoints() (endpointdiscovery.EndpointGroups, error) {
	for usName, spec := range c.upstreamSpecs {
		if len(spec.ServiceTags) > 0 {

		}
	}
	endpointGroups := make(endpointdiscovery.EndpointGroups)
	for upstreamName, spec := range c.upstreamSpecs {
		instances, meta, err := c.consul.Catalog().Service(spec.ServiceName, "", nil)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to find %v in service catalog", spec.ServiceName)
		}
		if len(instances) < 1 {
			log.Warnf("no healthy instances found for upstream %s with service name %s"+
				", EDS will not get endpoints for it", upstreamName, spec.ServiceName)
		}
		for _, inst := range instances {
			if !hasRequiredTags(inst.ServiceTags, spec.ServiceTags) {
				continue
			}
			ep := endpointdiscovery.Endpoint{
				Address: inst.ServiceAddress,
				Port:    int32(inst.ServicePort),
			}
			endpointGroups[upstreamName] = append(endpointGroups[upstreamName], ep)
		}
	}
	// sort for idempotency
	for upstreamName, epGroup := range endpointGroups {
		sort.SliceStable(epGroup, func(i, j int) bool {
			return epGroup[i].Address < epGroup[j].Address
		})
		endpointGroups[upstreamName] = epGroup
	}
	newHash, err := hashstructure.Hash(endpointGroups, nil)
	if err != nil {
		log.Warnf("error in consul endpoint controller: %v", err)
		return nil, nil
	}
	if newHash == c.lastSeen {
		return nil, nil
	}
	c.lastSeen = newHash
	return endpointGroups, nil
}

func hasRequiredTags(tags, required []string) bool {
	if len(required) == 0 {
		return true
	}
	for _, req := range required {
		var found bool
		for _, t := range tags {
			// found the required tag
			if t == req {
				found = true
				break
			}
		}
		// missing required tag
		if !found {
			return false
		}
	}
	return true
}
