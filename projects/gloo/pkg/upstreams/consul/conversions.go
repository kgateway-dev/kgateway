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
func toUpstreamList(forNamespace string, services []*ServiceMeta, consulConfig *v1.Settings_ConsulConfiguration) v1.UpstreamList {
	var upstreams v1.UpstreamList
	for _, svc := range services {
		us := CreateUpstreamsFromService(svc, consulConfig)
		for _, upstream := range us {
			if forNamespace != "" && upstream.Metadata.Namespace != forNamespace {
				continue
			}
			upstreams = append(upstreams, upstream)
		}
	}
	return upstreams.Sort()
}

// This function normally returns 1 upstream. It returns two upstreams if
// both automatic tls discovery and service-splitting is on for consul,
// and this service's tag list contains the tag specified by the tlsTagName config.
// In this case, it returns 2 upstreams that are identical save for the presence of
// the tlsTagName and noTlsTagNmae values in the InstanceTags arrays for the
// tls and non-tls upstreams respectively. Splitting or not, '-tls' is always
// appended to the tls upstreams metadata name.
func CreateUpstreamsFromService(service *ServiceMeta, consulConfig *v1.Settings_ConsulConfiguration) []*v1.Upstream {
	var result []*v1.Upstream
	// if config isn't nil, then it's assumed then it's been validated in the consul plugin's init function
	// (or is properly formatted in testing).
	// if useTlsTagging is true, then check the consul service for the tls tag.
	if consulConfig != nil && consulConfig.GetUseTlsTagging() {
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
			var tlsInstanceTags []string
			if consulConfig.GetSplitTlsServices() {
				tlsInstanceTags = []string{consulConfig.GetTlsTagName()}
			} else {
				tlsInstanceTags = []string{}
			}
			result = append(result, &v1.Upstream{
				Metadata: core.Metadata{
					Name:      fakeUpstreamName(service.Name + "-tls"),
					Namespace: defaults.GlooSystem,
				},
				SslConfig: &v1.UpstreamSslConfig{
					SslSecrets: &v1.UpstreamSslConfig_SecretRef{
						SecretRef: &core.ResourceRef{
							Name:      consulConfig.GetRootCaName(),
							Namespace: consulConfig.GetRootCaNamespace(),
						},
					},
				},
				UpstreamType: &v1.Upstream_Consul{
					Consul: &consulplugin.UpstreamSpec{
						ServiceName:  service.Name,
						DataCenters:  service.DataCenters,
						ServiceTags:  service.Tags,
						InstanceTags: tlsInstanceTags,
					},
				},
			})
			// just return the tls upstream unless we're splitting the upstream.
			if !consulConfig.GetSplitTlsServices() {
				return result
			}
		}
	}
	// if we're service splitting, and we've already created a tls upstream, add the no-tls tag to the
	// non-tls upstream.
	var noTlsInstanceTags []string
	if len(result) == 1 {
		noTlsInstanceTags = []string{consulConfig.GetNoTlsTagName()}
	} else {
		noTlsInstanceTags = []string{}
	}
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
				InstanceTags: noTlsInstanceTags,
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
