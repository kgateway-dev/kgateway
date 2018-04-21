package consul

import (
	"fmt"

	"github.com/hashicorp/consul/api"
	"github.com/mitchellh/hashstructure"
	"github.com/pkg/errors"

	"context"

	"sort"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/backoff"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/pkg/log"
)

type endpointController struct {
	endpoints chan endpointdiscovery.EndpointGroups
	errs      chan error
	consul    *api.Client
	ctx       context.Context

	// map of upstream + spec  hash to cancel func
	upstreamCancelFuncs map[string]context.CancelFunc

	lastSeen         uint64
	upstreamsToTrack chan []*v1.Upstream
}

func newEndpointController(cfg *api.Config) (*endpointController, error) {
	client, err := api.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create consul client: %v", err)
	}
	c := &endpointController{
		endpoints:           make(chan endpointdiscovery.EndpointGroups),
		errs:                make(chan error),
		upstreamsToTrack:    make(chan []*v1.Upstream, 1),
		upstreamCancelFuncs: make(map[string]context.CancelFunc),
		consul:              client,
	}

	return c, nil
}

type endpointsTuple struct {
	usName string
	eps    []endpointdiscovery.Endpoint
}

func (c *endpointController) Run(stop <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.ctx = ctx
	discoveredEndpoints := make(chan endpointsTuple)
	cachedEndpointGroups := make(endpointdiscovery.EndpointGroups)
	for {
		select {
		case <-stop:
			log.Printf("consul eds stopped")
			return
		case newUpstreamList := <-c.upstreamsToTrack:
			c.startWatch(newUpstreamList, discoveredEndpoints)
		case tuple := <-discoveredEndpoints:
			cachedEndpointGroups[tuple.usName] = tuple.eps
			newEndpointGroups := make(endpointdiscovery.EndpointGroups)
			for usName, eps := range cachedEndpointGroups {
				newEndpointGroups[usName] = eps
			}
			newHash, _ := hashstructure.Hash(cachedEndpointGroups, nil)
			if newHash == c.lastSeen {
				continue
			}
			c.lastSeen = newHash
			c.endpoints <- newEndpointGroups
		}
	}
}

func (c *endpointController) TrackUpstreams(upstreams []*v1.Upstream) {
	c.upstreamsToTrack <- upstreams
}

func (c *endpointController) Endpoints() <-chan endpointdiscovery.EndpointGroups {
	return c.endpoints
}

func (c *endpointController) Error() <-chan error {
	return c.errs
}

func hashUpstream(us *v1.Upstream) string {
	h, _ := hashstructure.Hash(*us.Spec, nil)
	return fmt.Sprintf("%v-%v", us.Name, h)
}

func (c *endpointController) startWatch(upstreams []*v1.Upstream, discoveredEndpoints chan endpointsTuple) {
	// cancel watches for upstreams that are deceased
	for usHash, cancel := range c.upstreamCancelFuncs {
		var found bool
		for _, us := range upstreams {
			if hashUpstream(us) == usHash {
				found = true
				break
			}
		}
		if !found {
			cancel()
			delete(c.upstreamCancelFuncs, usHash)
		}
	}
	for _, us := range upstreams {
		usHash := hashUpstream(us)
		// nothing to do
		if _, ok := c.upstreamCancelFuncs[usHash]; ok {
			continue
		}
		ctx, cancel := context.WithCancel(c.ctx)
		c.upstreamCancelFuncs[usHash] = cancel
		// start goroutine for this upstream
		go c.beginTrackingUpstream(ctx, us, discoveredEndpoints)
	}
}

func (c *endpointController) beginTrackingUpstream(ctx context.Context, us *v1.Upstream, discoveredEndpoints chan endpointsTuple) {
	spec, err := DecodeUpstreamSpec(us.Spec)
	if err != nil {
		c.errs <- errors.Wrapf(err, "failed to parse spec for upstream %s, cannot discover endpoints for it", us.Name)
		return
	}
	var lastIndex uint64
	for {
		select {
		case <-ctx.Done():
			return
		default:
			backoff.UntilSuccess(func() error {
				eps, index, err := c.getNextUpdateForUpstream(ctx, spec, lastIndex)
				if err != nil {
					return errors.Wrapf(err, "getting next endpoints for consul upstream failed")
				}
				lastIndex = index
				if len(eps) == 0 {
					return nil
				}
				// idempotency
				sort.SliceStable(eps, func(i, j int) bool {
					return endpointdiscovery.Less(eps[i], eps[j])
				})
				discoveredEndpoints <- endpointsTuple{usName: us.Name, eps: eps}
				return nil
			}, ctx)
		}
	}
}

func (c *endpointController) getNextUpdateForUpstream(ctx context.Context, spec *UpstreamSpec, lastIndex uint64) ([]endpointdiscovery.Endpoint, uint64, error) {
	opts := &api.QueryOptions{RequireConsistent: true, WaitIndex: lastIndex}
	opts = opts.WithContext(ctx)
	instances, meta, err := c.consul.Catalog().Service(spec.ServiceName, "", opts)
	if err != nil {
		return nil, lastIndex, errors.Wrapf(err, "failed to find %v in service catalog", spec.ServiceName)
	}
	if len(instances) < 1 {
		log.Warnf("no healthy instances found for service name %s, EDS will not get endpoints for it", spec.ServiceName)
	}
	var eps []endpointdiscovery.Endpoint
	for _, inst := range instances {
		if !hasRequiredTags(inst.ServiceTags, spec.ServiceTags) {
			continue
		}
		if inst.ServiceAddress == "" || inst.ServicePort == 0 {
			continue
		}
		ep := endpointdiscovery.Endpoint{
			Address: inst.ServiceAddress,
			Port:    int32(inst.ServicePort),
		}
		eps = append(eps, ep)
	}
	return eps, meta.LastIndex, nil
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
