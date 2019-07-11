package consul

import (
	"strings"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"

	"sort"

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
func toUpstreamList(services serviceToDataCentersMap) v1.UpstreamList {
	var upstreams v1.UpstreamList
	for serviceName, dataCenters := range services {
		sort.Strings(dataCenters)
		upstreams = append(upstreams, toUpstream(serviceName, dataCenters))
	}
	return upstreams
}

func toUpstream(serviceName string, dataCenters []string) *v1.Upstream {
	sort.Strings(dataCenters)
	return &v1.Upstream{
		Metadata: core.Metadata{
			Name:      fakeUpstreamName(serviceName),
			Namespace: "", // no namespace
		},
		UpstreamSpec: &v1.UpstreamSpec{
			UpstreamType: &v1.UpstreamSpec_Consul{
				Consul: &consulplugin.UpstreamSpec{
					ServiceName: serviceName,
					DataCenters: dataCenters,
				},
			},
		},
	}
}
