package proxy_syncer

import (
	"context"
	"fmt"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/solo-io/gloo/projects/gateway2/endpoints"
	extensionsplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"
	"github.com/solo-io/gloo/projects/gateway2/ir"
	"github.com/solo-io/gloo/projects/gateway2/utils/krtutil"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/control-plane/resource"
	"go.uber.org/zap"
	"istio.io/istio/pkg/kube/krt"
)

type EndpointResources struct {
	Endpoints            envoycache.Resource
	EndpointsVersion     uint64
	UpstreamResourceName string
}

func (c EndpointResources) ResourceName() string {
	return c.UpstreamResourceName
}

func (c EndpointResources) Equals(in EndpointResources) bool {
	return c.UpstreamResourceName == in.UpstreamResourceName && c.EndpointsVersion == in.EndpointsVersion
}

// TODO: this is needed temporary while we don't have the per-upstream translation done.
// once the plugins are fixed to support it, we can have the proxy translation skip upstreams/endpoints and remove this collection
func newEnvoyEndpoints(glooEndpoints krt.Collection[ir.EndpointsForUpstream], dbg *krt.DebugHandler) krt.Collection[EndpointResources] {
	clas := krt.NewCollection(glooEndpoints, func(_ krt.HandlerContext, ep ir.EndpointsForUpstream) *EndpointResources {
		return TransformEndpointToResources(ep)
	}, krt.WithDebugging(dbg), krt.WithName("EnvoyEndpoints"))
	return clas
}

func TransformEndpointToResources(ep ir.EndpointsForUpstream) *EndpointResources {
	cla := prioritize(ep)
	return &EndpointResources{
		Endpoints:            resource.NewEnvoyResource(cla),
		EndpointsVersion:     ep.LbEpsEqualityHash,
		UpstreamResourceName: ep.UpstreamResourceName,
	}
}

func prioritize(ep ir.EndpointsForUpstream) *envoy_config_endpoint_v3.ClusterLoadAssignment {
	cla := &envoy_config_endpoint_v3.ClusterLoadAssignment{
		ClusterName: ep.ClusterName,
	}
	for loc, eps := range ep.LbEps {
		var l *envoy_config_core_v3.Locality
		if loc != (ir.PodLocality{}) {
			l = &envoy_config_core_v3.Locality{
				Region:  loc.Region,
				Zone:    loc.Zone,
				SubZone: loc.Subzone,
			}
		}

		lbeps := make([]*envoy_config_endpoint_v3.LbEndpoint, 0, len(eps))
		for _, ep := range eps {
			lbeps = append(lbeps, ep.LbEndpoint)
		}

		endpoint := &envoy_config_endpoint_v3.LocalityLbEndpoints{
			LbEndpoints: lbeps,
			Locality:    l,
		}

		cla.Endpoints = append(cla.GetEndpoints(), endpoint)
	}

	// In theory we want to run endpoint plugins here.
	// we only have one endpoint plugin - and it also does failover... so might be simpler to not support it in ggv2 and
	// deprecating the functionality. it's not easy to do as with krt we no longer have gloo 'Endpoint' objects
	return cla
}

type UccWithEndpoints struct {
	Client        ir.UniqlyConnectedClient
	Endpoints     envoycache.Resource
	EndpointsHash uint64
	endpointsName string
}

func (c UccWithEndpoints) ResourceName() string {
	return fmt.Sprintf("%s/%s", c.Client.ResourceName(), c.endpointsName)
}

func (c UccWithEndpoints) Equals(in UccWithEndpoints) bool {
	return c.Client.Equals(in.Client) && c.EndpointsHash == in.EndpointsHash
}

type PerClientEnvoyEndpoints struct {
	endpoints krt.Collection[UccWithEndpoints]
	index     krt.Index[string, UccWithEndpoints]
}

func (ie *PerClientEnvoyEndpoints) FetchEndpointsForClient(kctx krt.HandlerContext, ucc ir.UniqlyConnectedClient) []UccWithEndpoints {
	return krt.Fetch(kctx, ie.endpoints, krt.FilterIndex(ie.index, ucc.ResourceName()))
}

func NewPerClientEnvoyEndpoints(logger *zap.Logger, krtopts krtutil.KrtOptions, uccs krt.Collection[ir.UniqlyConnectedClient],
	glooEndpoints krt.Collection[ir.EndpointsForUpstream],
	plugins []extensionsplug.EndpointPlugin,
) PerClientEnvoyEndpoints {
	clas := krt.NewManyCollection(glooEndpoints, func(kctx krt.HandlerContext, ep ir.EndpointsForUpstream) []UccWithEndpoints {
		uccs := krt.Fetch(kctx, uccs)
		uccWithEndpointsRet := make([]UccWithEndpoints, 0, len(uccs))
		for _, ucc := range uccs {

			// check if we have a plugin to do it
			cla, additionalHash := proccessWithPlugins(plugins, kctx, context.TODO(), ucc, ep)
			if cla != nil {
				uccWithEp := UccWithEndpoints{
					Client:        ucc,
					Endpoints:     resource.NewEnvoyResource(cla),
					EndpointsHash: ep.LbEpsEqualityHash ^ additionalHash,
					endpointsName: ep.ResourceName(),
				}

				uccWithEndpointsRet = append(uccWithEndpointsRet, uccWithEp)
			} else {
				cla := endpoints.PrioritizeEndpoints(logger, nil, ep, ucc)
				uccWithEp := UccWithEndpoints{
					Client:        ucc,
					Endpoints:     resource.NewEnvoyResource(cla),
					EndpointsHash: ep.LbEpsEqualityHash,
					endpointsName: ep.ResourceName(),
				}
				uccWithEndpointsRet = append(uccWithEndpointsRet, uccWithEp)
			}
		}
		return uccWithEndpointsRet
	}, krtopts.ToOptions("PerClientEnvoyEndpoints")...)
	idx := krt.NewIndex(clas, func(ucc UccWithEndpoints) []string {
		return []string{ucc.Client.ResourceName()}
	})

	return PerClientEnvoyEndpoints{
		endpoints: clas,
		index:     idx,
	}
}

func proccessWithPlugins(plugins []extensionsplug.EndpointPlugin, kctx krt.HandlerContext, ctx context.Context, ucc ir.UniqlyConnectedClient, in ir.EndpointsForUpstream) (*envoy_config_endpoint_v3.ClusterLoadAssignment, uint64) {
	for _, processEnddpoints := range plugins {
		cla, additionalHash := processEnddpoints(kctx, context.TODO(), ucc, in)
		if cla != nil {
			return cla, additionalHash
		}
	}
	return nil, 0
}
