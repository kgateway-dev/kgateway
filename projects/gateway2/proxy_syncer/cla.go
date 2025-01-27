package proxy_syncer

import (
	"fmt"

	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"github.com/kgateway-dev/kgateway/projects/gateway2/ir"
	"github.com/kgateway-dev/kgateway/projects/gateway2/utils/krtutil"
	"go.uber.org/zap"
	"istio.io/istio/pkg/kube/krt"
)

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
	Endpoints     *envoy_config_endpoint_v3.ClusterLoadAssignment
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

func NewPerClientEnvoyEndpoints(
	logger *zap.Logger,
	krtopts krtutil.KrtOptions,
	uccs krt.Collection[ir.UniqlyConnectedClient],
	glooEndpoints krt.Collection[ir.EndpointsForUpstream],
	translateEndpoints func(kctx krt.HandlerContext, ucc ir.UniqlyConnectedClient, ep ir.EndpointsForUpstream) (*envoy_config_endpoint_v3.ClusterLoadAssignment, uint64),
) PerClientEnvoyEndpoints {
	clas := krt.NewManyCollection(glooEndpoints, func(kctx krt.HandlerContext, ep ir.EndpointsForUpstream) []UccWithEndpoints {
		uccs := krt.Fetch(kctx, uccs)
		uccWithEndpointsRet := make([]UccWithEndpoints, 0, len(uccs))
		for _, ucc := range uccs {
			cla, additionalHash := translateEndpoints(kctx, ucc, ep)
			u := UccWithEndpoints{
				Client:        ucc,
				Endpoints:     cla,
				EndpointsHash: ep.LbEpsEqualityHash ^ additionalHash,
				endpointsName: ep.ResourceName(),
			}
			uccWithEndpointsRet = append(uccWithEndpointsRet, u)
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
