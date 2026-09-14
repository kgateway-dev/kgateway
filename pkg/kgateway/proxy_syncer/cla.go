package proxy_syncer

import (
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

type UccWithEndpoints struct {
	Client ir.UniquelyConnectedClient
	// +krtEqualsTodo compare load assignments when equality matters
	Endpoints     *envoyendpointv3.ClusterLoadAssignment
	EndpointsHash uint64
	endpointsName string
	// resourceName caches the KRT identity key, which KRT recomputes for every row on
	// every event. Rows are per-client x per-backend, so computing it on each call is a
	// per-call allocation multiplied by both dimensions.
	// +noKrtEquals derived from Client and endpointsName, both of which are compared
	resourceName string
}

func (c UccWithEndpoints) ResourceName() string {
	// Fall back for rows built as bare struct literals (tests) that skip the cache.
	if c.resourceName == "" {
		return uccEndpointsResourceName(c.Client, c.endpointsName)
	}
	return c.resourceName
}

func uccEndpointsResourceName(client ir.UniquelyConnectedClient, endpointsName string) string {
	return client.ResourceName() + "/" + endpointsName
}

func (c UccWithEndpoints) Equals(in UccWithEndpoints) bool {
	return c.Client.Equals(in.Client) &&
		c.EndpointsHash == in.EndpointsHash &&
		c.endpointsName == in.endpointsName
}

type PerClientEnvoyEndpoints struct {
	endpoints krt.Collection[UccWithEndpoints]
	index     krt.Index[string, UccWithEndpoints]
}

func (ie *PerClientEnvoyEndpoints) FetchEndpointsForClient(kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient) []UccWithEndpoints {
	return krt.Fetch(kctx, ie.endpoints, krt.FilterIndex(ie.index, ucc.ResourceName()))
}

func NewPerClientEnvoyEndpoints(
	krtopts krtutil.KrtOptions,
	uccs krt.Collection[ir.UniquelyConnectedClient],
	kgatewayEndpoints krt.Collection[ir.EndpointsForBackend],
	translateEndpoints func(kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient, ep ir.EndpointsForBackend) (*envoyendpointv3.ClusterLoadAssignment, uint64, uint64),
) PerClientEnvoyEndpoints {
	eps := krt.NewManyCollection(kgatewayEndpoints, func(kctx krt.HandlerContext, ep ir.EndpointsForBackend) []UccWithEndpoints {
		uccs := krt.Fetch(kctx, uccs)
		uccWithEndpointsRet := make([]UccWithEndpoints, 0, len(uccs))
		for _, ucc := range uccs {
			cla, resolvedEndpointsHash, additionalHash := translateEndpoints(kctx, ucc, ep)
			epName := ep.ResourceName()
			u := UccWithEndpoints{
				Client:        ucc,
				Endpoints:     cla,
				EndpointsHash: resolvedEndpointsHash ^ additionalHash,
				endpointsName: epName,
				resourceName:  uccEndpointsResourceName(ucc, epName),
			}
			uccWithEndpointsRet = append(uccWithEndpointsRet, u)
		}
		return uccWithEndpointsRet
	}, krtopts.ToOptions("PerClientEnvoyEndpoints")...)
	idx := krtpkg.UnnamedIndex(eps, func(ucc UccWithEndpoints) []string {
		return []string{ucc.Client.ResourceName()}
	})

	return PerClientEnvoyEndpoints{
		endpoints: eps,
		index:     idx,
	}
}
