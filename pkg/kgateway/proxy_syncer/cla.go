package proxy_syncer

import (
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/endpoints"
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
	// resourceName is precomputed at construction: krt calls ResourceName
	// repeatedly per object and these objects exist per (client, endpoints) pair.
	// Derived from Client and endpointsName, which are compared.
	// +noKrtEquals
	resourceName string
}

func (c UccWithEndpoints) ResourceName() string {
	if c.resourceName != "" {
		return c.resourceName
	}
	return c.Client.ResourceName() + "/" + c.endpointsName
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
	translateEndpoints func(kctx krt.HandlerContext, uccs []ir.UniquelyConnectedClient, ep ir.EndpointsForBackend) []endpoints.ClaPerClient,
) PerClientEnvoyEndpoints {
	eps := krt.NewManyCollection(kgatewayEndpoints, func(kctx krt.HandlerContext, ep ir.EndpointsForBackend) []UccWithEndpoints {
		uccs := krt.Fetch(kctx, uccs)
		epName := ep.ResourceName()
		uccWithEndpointsRet := make([]UccWithEndpoints, 0, len(uccs))
		for _, result := range translateEndpoints(kctx, uccs, ep) {
			u := UccWithEndpoints{
				Client:        result.Client,
				Endpoints:     result.Cla,
				EndpointsHash: ep.LbEpsEqualityHash ^ result.AdditionalHash,
				endpointsName: epName,
				resourceName:  result.Client.ResourceName() + "/" + epName,
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
