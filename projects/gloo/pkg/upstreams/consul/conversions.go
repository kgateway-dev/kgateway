package consul

import (
	"sort"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	consulplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/consul"
)

const UpstreamNamePrefix = "consul-svc:"
const TlsTag = "glooUseTls"

func IsConsulUpstream(upstreamName string) bool {
	return strings.HasPrefix(upstreamName, UpstreamNamePrefix)
}

func DestinationToUpstreamRef(consulDest *v1.ConsulServiceDestination) *core.ResourceRef {
	return &core.ResourceRef{
		Namespace: defaults.GlooSystem,
		Name:      fakeUpstreamName(consulDest.ServiceName),
	}
}

func fakeUpstreamName(consulSvcName string) string {
	return UpstreamNamePrefix + consulSvcName
}

// Creates an upstream for each service in the map
func toUpstreamList(forNamespace string, services []*ServiceMeta) v1.UpstreamList {
	var upstreams v1.UpstreamList
	for _, svc := range services {
		us := CreateUpstreamsFromService(svc)
		for _, upstream := range us {
			if forNamespace != "" && upstream.Metadata.Namespace != forNamespace {
				continue
			}
			upstreams = append(upstreams, upstream)
		}
	}
	return upstreams.Sort()
}

// This function normally returns 1 upstream. It instead returns two upstreams if
// automatic tls discovery is on for consul, and this service contains the designated
// useTls tag (which by default is glooUseTls).
// In this case, it returns 2 upstreams that are identical save for the presense of
// InstanceTags: []string{"glooUseTls"} in the upstream that'll use TLS.
func CreateUpstreamsFromService(service *ServiceMeta) []*v1.Upstream {
	var result []*v1.Upstream
	useTls := false
	for _, tag := range service.Tags {
		if tag == TlsTag {
			useTls = true
			break
		}
	}
	if useTls {
		result = append(result, &v1.Upstream{
			Metadata: core.Metadata{
				Name:      fakeUpstreamName(service.Name),
				Namespace: defaults.GlooSystem,
			},
			UpstreamType: &v1.Upstream_Consul{
				Consul: &consulplugin.UpstreamSpec{
					ServiceName:  service.Name,
					DataCenters:  service.DataCenters,
					ServiceTags:  service.Tags,
					InstanceTags: []string{"glooUseTls"},
				},
			},
		})
	}

	result = append(result, &v1.Upstream{
		Metadata: core.Metadata{
			Name:      fakeUpstreamName(service.Name),
			Namespace: defaults.GlooSystem,
		},
		UpstreamType: &v1.Upstream_Consul{
			Consul: &consulplugin.UpstreamSpec{
				ServiceName: service.Name,
				DataCenters: service.DataCenters,
				ServiceTags: service.Tags,
			},
		},
	})
	return result
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
