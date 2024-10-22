package proxy_syncer

import (
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/solo-io/gloo/projects/gateway2/krtcollections"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/slices"
	"k8s.io/apimachinery/pkg/types"
)

type EndpointResources struct {
	endpoints        envoycache.Resource
	endpointsVersion uint64
	upstreamRef      types.NamespacedName
}

func (c EndpointResources) ResourceName() string {
	return c.upstreamRef.String()
}

func (c EndpointResources) Equals(in EndpointResources) bool {
	return c.upstreamRef == in.upstreamRef && c.endpointsVersion == in.endpointsVersion
}

func newEnvoyEndpoints(glooEndpoints krt.Collection[EndpointsForUpstream]) krt.Collection[EndpointResources] {

	clas := krt.NewCollection(glooEndpoints, func(kctx krt.HandlerContext, ep EndpointsForUpstream) *EndpointResources {
		cla := prioritize(ep)
		return &EndpointResources{
			endpoints:        resource.NewEnvoyResource(cla),
			endpointsVersion: ep.lbEpsEqualityHash,
			upstreamRef:      ep.UpstreamRef,
		}
	})

	return clas
}

func prioritize(ep EndpointsForUpstream) *envoy_config_endpoint_v3.ClusterLoadAssignment {
	cla := &envoy_config_endpoint_v3.ClusterLoadAssignment{
		ClusterName: ep.clusterName,
	}
	for loc, eps := range ep.LbEps {
		var l *envoy_config_core_v3.Locality
		if loc != (krtcollections.PodLocality{}) {
			l = &envoy_config_core_v3.Locality{
				Region:  loc.Region,
				Zone:    loc.Zone,
				SubZone: loc.Subzone,
			}
		}

		endpoints := []*envoy_config_endpoint_v3.LocalityLbEndpoints{{
			LbEndpoints: slices.Map(eps, func(e EndpointWithMd) *envoy_config_endpoint_v3.LbEndpoint { return e.LbEndpoint }),
		}}
		for _, ep := range endpoints {
			ep.Locality = l
		}

		cla.Endpoints = append(cla.Endpoints, endpoints...)
	}

	// In theory we want to run endpoint plugins here.
	// we only have one endpoint plugin - and it also does failover... so might be simpler to not support it in ggv2 and
	// deprecating the functionality. it's not easy to do as with krt we no longer have gloo 'Endpoint' objects
	return cla
}
