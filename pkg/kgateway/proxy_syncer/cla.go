package proxy_syncer

import (
	"hash/fnv"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

// UccWithEndpoints is one client's view of one backend's endpoints: the
// ClusterLoadAssignment that client should receive, keyed by (client, backend).
// Clients that resolve identically share a single interned CLA, so the row count is
// per-pair but the proto count is per distinct result.
type UccWithEndpoints struct {
	Client ir.UniquelyConnectedClient
	// Endpoints is wrapped so consumers cannot mutate the CLA interned across
	// every UCC that resolved identically; see package sharedproto. Content
	// equality is carried by EndpointsHash, which combines the resolved endpoint
	// content, the endpoint plugins' contributions, and the load-balancing
	// context — exactly the inputs BuildClusterLoadAssignment reads — so the two
	// fields cannot disagree.
	// +noKrtEquals EndpointsHash is a content hash over the same inputs
	Endpoints     sharedproto.Shared[*envoyendpointv3.ClusterLoadAssignment]
	EndpointsHash uint64
	endpointsName string
	// resourceName caches the KRT identity key, which KRT recomputes for every row
	// on every recompute (slices.GroupUnique over the transform output) and again on
	// the event path. This is the one collection still fanned out per client x per
	// backend, so building the key on each call multiplies a format-string parse and
	// three allocations by both dimensions. Nothing is retained that KRT wasn't
	// already keeping: the same string is a map key in the collection state, so
	// caching just makes the field and those keys share one allocation.
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

// uccEndpointsResourceName builds the (client, backend) identity key. Callers cache
// the result on the row; see the resourceName field for why that is worth doing.
func uccEndpointsResourceName(client ir.UniquelyConnectedClient, endpointsName string) string {
	return client.ResourceName() + "/" + endpointsName
}

func (c UccWithEndpoints) Equals(in UccWithEndpoints) bool {
	return c.Client.Equals(in.Client) &&
		c.EndpointsHash == in.EndpointsHash &&
		c.endpointsName == in.endpointsName
}

// PerClientEnvoyEndpoints is the endpoint half of per-client xDS: [UccWithEndpoints]
// rows indexed by client, so assembling one client's EDS payload does not scan the
// other clients' rows. Both [NewPerClientEnvoyEndpoints] (backend endpoints) and
// [NewPerClientLocalClusterEndpoints] (the gateway's own local cluster) produce this
// shape, and snapshot assembly consumes them the same way.
type PerClientEnvoyEndpoints struct {
	endpoints krt.Collection[UccWithEndpoints]
	index     krt.Index[string, UccWithEndpoints]
}

// FetchEndpointsForClient returns every CLA belonging to ucc, registering a KRT
// dependency narrowed to that client's rows.
func (ie *PerClientEnvoyEndpoints) FetchEndpointsForClient(kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient) []UccWithEndpoints {
	return krt.Fetch(kctx, ie.endpoints, krt.FilterIndex(ie.index, ucc.ResourceName()))
}

// NewPerClientEnvoyEndpoints builds a [UccWithEndpoints] row for every (client,
// backend) pair by resolving each backend's endpoints from that client's
// perspective — locality, labels, and any plugin-applied priority — and turning the
// result into a ClusterLoadAssignment.
//
// resolveEndpoints and buildClusterLoadAssignment are injected rather than called
// directly so this collection can be built against a test double.
//
// Endpoints must vary per client (that is what locality-aware routing means), so
// unlike clusters there is no base to share. What is shared is the output: clients
// whose resolution hashes agree are handed the same read-only CLA proto, which is
// why a large fleet in one locality costs about as much as a single client.
func NewPerClientEnvoyEndpoints(
	krtopts krtutil.KrtOptions,
	uccs krt.Collection[ir.UniquelyConnectedClient],
	kgatewayEndpoints krt.Collection[ir.EndpointsForBackend],
	resolveEndpoints func(kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient, ep ir.EndpointsForBackend) translator.ResolvedEndpoints,
	buildClusterLoadAssignment func(ucc ir.UniquelyConnectedClient, resolved translator.ResolvedEndpoints) *envoyendpointv3.ClusterLoadAssignment,
) PerClientEnvoyEndpoints {
	eps := krt.NewManyCollection(kgatewayEndpoints, func(kctx krt.HandlerContext, ep ir.EndpointsForBackend) []UccWithEndpoints {
		uccs := krt.Fetch(kctx, uccs)
		uccWithEndpointsRet := make([]UccWithEndpoints, 0, len(uccs))
		// Loop-invariant: every row in this transform shares the same backend.
		epName := ep.ResourceName()
		// Intern CLAs across UCCs that resolve identically. The CLA varies per
		// client only through PrioritizeEndpoints (locality, labels) and the
		// plugin-applied PriorityInfo; UCCs sharing those hashes get one shared
		// read-only proto instead of a freshly built copy each.
		sharedClas := map[uint64]sharedproto.Shared[*envoyendpointv3.ClusterLoadAssignment]{}
		for _, ucc := range uccs {
			resolved := resolveEndpoints(kctx, ucc, ep)
			endpointsHash := combineEndpointHash(resolved.Inputs.EndpointsForBackend.LbEpsEqualityHash, resolved.AdditionalHash, resolved.LoadBalancingHash)
			cla, ok := sharedClas[endpointsHash]
			if !ok {
				// Wrap captures the tripwire hash once per distinct CLA; rows
				// that reuse the interned CLA copy the wrapper.
				cla = sharedproto.Wrap(buildClusterLoadAssignment(ucc, resolved))
				sharedClas[endpointsHash] = cla
			}
			u := UccWithEndpoints{
				Client:        ucc,
				Endpoints:     cla,
				EndpointsHash: endpointsHash,
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

// combineEndpointHash folds the endpoint-equality, plugin, and load-balancing
// hashes into a single key. It replaces the prior LbEpsEqualityHash ^ additionalHash
// (which omitted the load-balancing context) so UCCs that differ only in locality
// or priority labels no longer collide on the same key.
func combineEndpointHash(parts ...uint64) uint64 {
	hasher := fnv.New64a()
	for _, part := range parts {
		utils.HashUint64(hasher, part)
	}
	return hasher.Sum64()
}
