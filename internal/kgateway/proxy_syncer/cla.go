package proxy_syncer

import (
	"fmt"

	envoy_config_endpoint_v3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
)

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
	krtopts krtutil.KrtOptions,
	uccs krt.Collection[ir.UniqlyConnectedClient],
	glooEndpoints krt.Collection[ir.EndpointsForBackend],
	translateEndpoints func(kctx krt.HandlerContext, ucc ir.UniqlyConnectedClient, ep ir.EndpointsForBackend) (*envoy_config_endpoint_v3.ClusterLoadAssignment, uint64),
) PerClientEnvoyEndpoints {
	eps := krt.NewManyCollection(glooEndpoints, func(kctx krt.HandlerContext, ep ir.EndpointsForBackend) []UccWithEndpoints {
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
	idx := krt.NewIndex(eps, func(ucc UccWithEndpoints) []string {
		return []string{ucc.Client.ResourceName()}
	})

	return PerClientEnvoyEndpoints{
		endpoints: eps,
		index:     idx,
	}
}
