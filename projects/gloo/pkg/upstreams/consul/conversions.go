package consul

import (
	"sort"
	"strings"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	consulplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/consul"
)

const upstreamNamePrefix = "consul-svc:"

func IsConsulUpstream(upstreamName string) bool {
	return strings.HasPrefix(upstreamName, upstreamNamePrefix)
}

func DestinationToUpstreamRef(consulDest *v1.ConsulServiceDestination) *core.ResourceRef {
	return &core.ResourceRef{
		Namespace: "",
		Name:      fakeUpstreamName(consulDest.ServiceName),
	}
}

func fakeUpstreamName(consulSvcName string) string {
	return upstreamNamePrefix + consulSvcName
}

// Creates an upstream for each service in the map
func toUpstreamList(services []*ServiceMeta) v1.UpstreamList {
	var upstreams v1.UpstreamList
	for _, svc := range services {
		upstreams = append(upstreams, ToUpstream(svc))
	}
	return upstreams
}

func ToUpstream(service *ServiceMeta) *v1.Upstream {
	return &v1.Upstream{
		Metadata: core.Metadata{
			Name:      fakeUpstreamName(service.Name),
			Namespace: "", // no namespace
		},
		UpstreamSpec: &v1.UpstreamSpec{
			UpstreamType: &v1.UpstreamSpec_Consul{
				Consul: &consulplugin.UpstreamSpec{
					ServiceName: service.Name,
					DataCenters: service.DataCenters,
					ServiceTags: service.Tags,
				},
			},
		},
	}
}

func toServiceMetaSlice(dcToSvcMap []*dataCenterServicesTuple) []*ServiceMeta {
	serviceMap := make(map[string]*ServiceMeta)
	for _, services := range dcToSvcMap {
		for serviceName, tags := range services.services {

			if serviceMeta, ok := serviceMap[serviceName]; !ok {
				serviceMap[serviceName] = &ServiceMeta{
					Name:        serviceName,
					DataCenters: []string{services.dataCenter},
					Tags:        tags,
				}
			} else {
				serviceMeta.DataCenters = append(serviceMeta.DataCenters, services.dataCenter)
				serviceMeta.Tags = mergeTags(serviceMeta.Tags, tags)
			}
		}
	}

	var result []*ServiceMeta
	for _, serviceMeta := range serviceMap {
		sort.Strings(serviceMeta.DataCenters)
		sort.Strings(serviceMeta.Tags)

		// Set this explicitly so return values are consistent
		// (otherwise they might be nil or []string{}, depending on the input)
		if len(serviceMeta.Tags) == 0 {
			serviceMeta.Tags = nil
		}

		result = append(result, serviceMeta)
	}
	return result
}

func mergeTags(existingTags []string, newTags []string) []string {

	// Index tags to avoid O(n^2)
	tagMap := make(map[string]bool)
	for _, tag := range existingTags {
		tagMap[tag] = true
	}

	// Add only missing tags
	for _, newTag := range newTags {
		if _, ok := tagMap[newTag]; !ok {
			existingTags = append(existingTags, newTag)
		}
	}
	return existingTags
}
