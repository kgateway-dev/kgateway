package consul

import (
	"sort"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	consulplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/consul"
)

const UpstreamNamePrefix = "consul-svc:"

func IsConsulUpstream(upstreamName string) bool {
	return strings.HasPrefix(upstreamName, UpstreamNamePrefix)
}

func DestinationToUpstreamRef(consulDest *v1.ConsulServiceDestination) *core.ResourceRef {
	return &core.ResourceRef{
		Namespace: defaults.GlooSystem,
		Name:      fakeUpstreamName(consulDest.GetServiceName()),
	}
}

func fakeUpstreamName(consulSvcName string) string {
	return UpstreamNamePrefix + consulSvcName
}

// Creates an upstream for each service in the map
func toUpstreamList(forNamespace string, services []*ServiceMeta, consulConfig *v1.Settings_ConsulUpstreamDiscoveryConfiguration) v1.UpstreamList {
	var results v1.UpstreamList
	for _, svc := range services {
		upstreams := CreateUpstreamsFromService(svc, consulConfig)
		for _, upstream := range upstreams {
			if forNamespace != "" && upstream.GetMetadata().GetNamespace() != forNamespace {
				continue
			}
			results = append(results, upstream)
		}
	}
	return results.Sort()
}

// This function normally returns 1 upstream. It instead returns two upstreams if
// both automatic tls discovery and service-splitting is on for consul,
// and this service's tag list contains the tag specified by the tlsTagName config.
// In this case, it returns 2 upstreams that are identical save for the presence of
// the tlsTagName in the instanceTags array for the tls upstream, and the same value
// in the instanceBlacklistTags array for the non-tls upstream.
func CreateUpstreamsFromService(service *ServiceMeta, consulConfig *v1.Settings_ConsulUpstreamDiscoveryConfiguration) []*v1.Upstream {
	var result []*v1.Upstream
	// if config isn't nil, then it's assumed then it's been validated in the consul plugin's init function
	// (or is properly formatted in testing).
	// if useTlsTagging is true, then check the consul service for the tls tag.
	var tlsInstanceTags []string
	if consulConfig.GetUseTlsTagging() {
		tlsTagFound := false
		for _, tag := range service.Tags {
			if tag == consulConfig.GetTlsTagName() {
				tlsTagFound = true
				break
			}
		}
		// if the tls tag is found create an upstream with an ssl config.
		if tlsTagFound {
			// additionally include the tls tag in the upstream's instanceTags if we're service splitting.
			if consulConfig.GetSplitTlsServices() {
				tlsInstanceTags = []string{consulConfig.GetTlsTagName()}
			}
			result = append(result, &v1.Upstream{
				Metadata: &core.Metadata{
					Name:      fakeUpstreamName(service.Name + "-tls"),
					Namespace: defaults.GlooSystem,
				},
				SslConfig: &ssl.UpstreamSslConfig{
					SslSecrets: &ssl.UpstreamSslConfig_SecretRef{
						SecretRef: &core.ResourceRef{
							Name:      consulConfig.GetRootCa().GetName(),
							Namespace: consulConfig.GetRootCa().GetNamespace(),
						},
					},
				},
				UpstreamType: &v1.Upstream_Consul{
					Consul: &consulplugin.UpstreamSpec{
						ServiceName:     service.Name,
						DataCenters:     service.DataCenters,
						ServiceTags:     service.Tags,
						InstanceTags:    tlsInstanceTags,
						ConsistencyMode: consulplugin.ConsulConsistencyModes(consulConfig.GetConsistencyMode()),
						QueryOptions:    consulConfig.GetQueryOptions(),
					},
				},
			})
			// Only return the tls upstream unless we're splitting the upstream.
			if !consulConfig.GetSplitTlsServices() {
				return result
			}
		}
	}
	result = append(result, &v1.Upstream{
		Metadata: &core.Metadata{
			Name:      fakeUpstreamName(service.Name),
			Namespace: defaults.GlooSystem,
		},
		UpstreamType: &v1.Upstream_Consul{
			Consul: &consulplugin.UpstreamSpec{
				ServiceName:           service.Name,
				DataCenters:           service.DataCenters,
				ServiceTags:           service.Tags,
				InstanceBlacklistTags: tlsInstanceTags, // Set blacklist on non-tls upstreams to the tls tag.
				ConsistencyMode:       consulplugin.ConsulConsistencyModes(consulConfig.GetConsistencyMode()),
				QueryOptions:          consulConfig.GetQueryOptions(),
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
