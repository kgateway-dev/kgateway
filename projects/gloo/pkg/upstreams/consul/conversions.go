package consul

import (
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
func toUpstreamList(services []ServiceMeta) v1.UpstreamList {
	var upstreams v1.UpstreamList
	for _, svc := range services {
		upstreams = append(upstreams, toUpstream(svc))
	}
	return upstreams
}

func toUpstream(service ServiceMeta) *v1.Upstream {
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
				},
			},
		},
	}
}
