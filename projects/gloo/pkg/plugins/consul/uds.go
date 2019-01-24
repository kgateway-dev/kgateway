package consul

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	consulplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/consul"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/utils/contextutils"

	"github.com/hashicorp/consul/api"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/errors"
)

var _ discovery.DiscoveryPlugin = &plugin{}

type upstreamController struct {
	consul          *api.Client
	ctx             context.Context
	watchNamespaces []string
	writeNamespace  string

	lastIndex uint64
}

type consulService struct {
	name    string
	tagSets [][]string
	connect bool
}

func (p *plugin) DiscoverUpstreams(watchNamespaces []string, writeNamespace string, opts clients.WatchOpts, discOpts discovery.Opts) (chan v1.UpstreamList, chan error, error) {
	if err := p.tryGetClient(); err != nil {
		return nil, nil, err
	}

	c := upstreamController{
		consul:          p.client,
		ctx:             opts.Ctx,
		watchNamespaces: watchNamespaces,
		writeNamespace:  writeNamespace,
	}

	return c.discoverUpstreams()
}

func (c *upstreamController) discoverUpstreams() (chan v1.UpstreamList, chan error, error) {
	upstreamsChan := make(chan v1.UpstreamList)
	errs := make(chan error)

	go func() {
		defer close(upstreamsChan)
		defer close(errs)
		// watch should open up with an initial read
		for {
			upstreams, err := c.discoverUpstreamsOnce()
			if err != nil {
				if c.ctx.Err() != nil {
					return
				}
				errs <- err
			} else {
				upstreamsChan <- upstreams
			}

		}
	}()

	return upstreamsChan, errs, nil
}

func (c *upstreamController) discoverUpstreamsOnce() (v1.UpstreamList, error) {
	ctx := c.ctx

	var list v1.UpstreamList

	err := contextutils.NewExponentioalBackoff(contextutils.ExponentioalBackoff{}).Backoff(ctx, func(ctx context.Context) error {
		services, index, err := c.getNextUpdate(ctx, c.lastIndex)
		if err != nil {
			return errors.Wrapf(err, "getting next update for consul upstream failed")
		}
		c.lastIndex = index
		if len(services) == 0 {
			return nil
		}
		// get each unique set of tags
		// we will use this to generate an upstream for each unique set
		var consulServices []consulService

		// find all services
		var nonConnectServices []string
		for svcName := range services {
			serviceInstances, _, err := c.consul.Catalog().Service(svcName, "", &api.QueryOptions{RequireConsistent: true})
			if err != nil {
				return errors.Wrapf(err, "failed to get instances of service %s", svcName)
			}
			for _, inst := range serviceInstances {
				if inst.ServiceProxy != nil && len(inst.ServiceProxy.DestinationServiceName) != 0 {
					nonConnectServices = append(nonConnectServices, inst.ServiceProxy.DestinationServiceName)
				} else {
					nonConnectServices = append(nonConnectServices, svcName)
				}
			}
		}

		for _, svcName := range nonConnectServices {
			serviceInstances, _, err := c.consul.Catalog().Service(svcName, "", &api.QueryOptions{RequireConsistent: true})
			if err != nil {
				return errors.Wrapf(err, "failed to get instances of service %s", svcName)
			}
			var allTagSets [][]string
			for _, inst := range serviceInstances {
				allTagSets = append(allTagSets, inst.ServiceTags)
			}
			// add a service with no tags, so the service can be accessed regardless of tags.
			allTagSets = append(allTagSets, []string{})

			svc := consulService{
				name:    svcName,
				tagSets: uniqueTagSets(allTagSets),
			}

			proxyInstances, _, _ := c.consul.Catalog().Connect(svcName, "", &api.QueryOptions{RequireConsistent: true})

			if len(proxyInstances) > 0 {
				svc.connect = true
			}
			consulServices = append(consulServices, svc)

		}

		list = c.convertServices(consulServices)
		return nil
	})

	return list, err

}

func (c *upstreamController) getNextUpdate(ctx context.Context, lastIndex uint64) (map[string][]string, uint64, error) {
	opts := &api.QueryOptions{RequireConsistent: true, WaitIndex: lastIndex}
	opts = opts.WithContext(ctx)
	services, meta, err := c.consul.Catalog().Services(opts)
	if err != nil {
		return nil, lastIndex, errors.Wrapf(err, "failed to consul list services")
	}
	return services, meta.LastIndex, nil
}

func (c *upstreamController) convertServices(serviceList []consulService) []*v1.Upstream {
	var upstreams []*v1.Upstream
	for _, svc := range serviceList {
		for _, tags := range svc.tagSets {
			us := &v1.Upstream{
				Metadata: core.Metadata{
					Name:      upstreamName(svc.name, tags),
					Namespace: c.writeNamespace,
				},
				UpstreamSpec: &v1.UpstreamSpec{
					UpstreamType: &v1.UpstreamSpec_Consul{
						Consul: &consulplugin.UpstreamSpec{
							ServiceName:    svc.name,
							ServiceTags:    tags,
							ConnectEnabled: svc.connect,
						},
					},
				},
				DiscoveryMetadata: &v1.DiscoveryMetadata{},
			}

			upstreams = append(upstreams, us)
		}
	}
	return upstreams
}

func (p *plugin) UpdateUpstream(original, desired *v1.Upstream) (bool, error) {
	originalSpec, ok := original.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Consul)
	if !ok {
		return false, errors.Errorf("internal error: expected *v1.UpstreamSpec_Consul, got %v", reflect.TypeOf(original.UpstreamSpec.UpstreamType).Name())
	}
	desiredSpec, ok := desired.UpstreamSpec.UpstreamType.(*v1.UpstreamSpec_Consul)
	if !ok {
		return false, errors.Errorf("internal error: expected *v1.UpstreamSpec_Consul, got %v", reflect.TypeOf(original.UpstreamSpec.UpstreamType).Name())
	}
	// copy service spec, we don't want to overwrite that
	desiredSpec.Consul.ServiceSpec = originalSpec.Consul.ServiceSpec

	if originalSpec.Equal(desiredSpec) {
		return false, nil
	}

	return true, nil
}

func uniqueTagSets(allTagSets [][]string) [][]string {
	var uniqueTagSets [][]string
	for _, tags := range allTagSets {
		// sort for idempotency
		sort.SliceStable(tags, func(i, j int) bool {
			return tags[i] < tags[j]
		})

		// check if this set already exists in the list
		var duplicate bool
		for _, set := range uniqueTagSets {
			if sliceEqual(tags, set) {
				duplicate = true
				break
			}
		}
		// if this set is accounted for we don't want a new upstream for this instance
		if duplicate {
			continue
		}

		uniqueTagSets = append(uniqueTagSets, tags)
	}
	// sort the set of sets
	sort.SliceStable(uniqueTagSets, func(i, j int) bool {
		tags1 := uniqueTagSets[i]
		tags2 := uniqueTagSets[j]
		if len(tags1) != len(tags2) {
			return len(tags1) < len(tags2)
		}
		for i := range tags1 {
			if tags1[i] != tags2[i] {
				return tags1[i] < tags2[i]
			}
		}
		panic("they're equal!?! THIS SHOULD NOT HAVE HAPPENED")
	})
	return uniqueTagSets
}

func upstreamName(serviceName string, tags []string) string {
	if len(tags) < 1 {
		return serviceName
	}
	return fmt.Sprintf("%s-%s", serviceName, strings.Join(tags, "-"))
}

func sliceEqual(s1, s2 []string) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i := range s1 {
		if s1[i] != s2[i] {
			return false
		}
	}
	return true
}
