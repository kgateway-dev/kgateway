package proxy_syncer

import (
	"hash/fnv"
	"slices"
	"sync"

	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/proxy_syncer/sharedproto"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

// UccWithEndpoints holds a CLA keyed by (client, backend). Equal results share
// one interned proto across clients.
type UccWithEndpoints struct {
	Client ir.UniquelyConnectedClient
	// Endpoints holds the interned, read-only CLA. EndpointsHash combines endpoint
	// content, plugin contributions, and load-balancing context into a 64-bit hash.
	// KRT equality and EDS versioning assume no collisions; a collision across row
	// revisions can leave stale endpoints. Interning separately confirms content equality.
	// +noKrtEquals EndpointsHash is a 64-bit content hash standing in for the proto; collision-freedom is assumed, see above
	Endpoints     sharedproto.Shared[*envoyendpointv3.ClusterLoadAssignment]
	EndpointsHash uint64
	endpointsName string
	// resourceName caches the key used by KRT, avoiding an allocation
	// per lookup for each client/backend pair.
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

// uccEndpointsResourceName builds the cached (client, backend) key.
func uccEndpointsResourceName(client ir.UniquelyConnectedClient, endpointsName string) string {
	return client.ResourceName() + "/" + endpointsName
}

func (c UccWithEndpoints) Equals(in UccWithEndpoints) bool {
	return c.Client.Equals(in.Client) &&
		c.EndpointsHash == in.EndpointsHash &&
		c.endpointsName == in.endpointsName
}

// claRetainer preserves each backend's interned CLAs across recomputations.
// KRT keeps already-stored rows, so a client connecting later must receive the proto
// those rows already hold, or each arrival cohort ends up with its own instance.
// Each pass replaces the retained set with the distinct CLAs its rows reference.
//
// Sharing converges while retained entries remain available to live rows. If an
// entry is lost, old rows and new clients can hold separate equal protos until
// the next content change; endpoint content remains correct.
//
// The delete handler and transform run on separate goroutines. forgetIfAbsent
// preserves entries for backends re-added before the handler runs. keepIfPresent
// removes entries stored after deletion, including when the handler ran first.
type claRetainer struct {
	mu sync.Mutex
	// byBackend holds, per backend resource name, the distinct CLAs that
	// backend's rows currently reference.
	byBackend map[string][]retainedCLA
}

// retainedCLA is one interned CLA together with the bucket it was interned
// under, which is what makes it findable again next pass.
type retainedCLA struct {
	hash uint64
	cla  sharedproto.Shared[*envoyendpointv3.ClusterLoadAssignment]
}

func newCLARetainer() *claRetainer {
	return &claRetainer{byBackend: make(map[string][]retainedCLA)}
}

// seed primes interner with the CLAs backend's rows are already holding.
func (r *claRetainer) seed(backend string, interner *sharedproto.Interner[*envoyendpointv3.ClusterLoadAssignment]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, retained := range r.byBackend[backend] {
		interner.Adopt(retained.cla, retained.hash)
	}
}

// keep replaces what is retained for backend with exactly the distinct CLAs rows
// reference, which is what bounds retention to the live set.
func (r *claRetainer) keep(backend string, rows []UccWithEndpoints) {
	// Each backend usually has few distinct results (one per locality), so use a linear scan.
	var retained []retainedCLA
	for _, row := range rows {
		if row.Endpoints.IsNil() {
			continue
		}
		if slices.ContainsFunc(retained, func(e retainedCLA) bool {
			return e.hash == row.EndpointsHash && sharedproto.Same(e.cla, row.Endpoints)
		}) {
			continue
		}
		retained = append(retained, retainedCLA{hash: row.EndpointsHash, cla: row.Endpoints})
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if len(retained) == 0 {
		delete(r.byBackend, backend)
		return
	}
	r.byBackend[backend] = retained
}

// keepIfPresent stores the CLAs, then checks the source collection's current
// state. Checking after the store removes entries created after the delete
// handler ran. present must not use the event that started the pass.
func (r *claRetainer) keepIfPresent(backend string, rows []UccWithEndpoints, present func() bool) {
	r.keep(backend, rows)
	if !present() {
		r.forget(backend)
	}
}

// forget drops a deleted backend's retained CLAs. The transform is not run for
// an input that no longer exists, so without this the entry would outlive the
// rows it was holding protos for.
func (r *claRetainer) forget(backend string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byBackend, backend)
}

// forgetIfAbsent drops retained CLAs only if the backend is currently absent.
// A delayed delete event must not remove a re-added backend's live entry.
func (r *claRetainer) forgetIfAbsent(backend string, absent bool) {
	if !absent {
		return
	}
	r.forget(backend)
}

// PerClientEnvoyEndpoints indexes [UccWithEndpoints] rows by client for EDS
// assembly. Backend and local-cluster endpoint collections both use this shape.
type PerClientEnvoyEndpoints struct {
	endpoints krt.Collection[UccWithEndpoints]
	index     krt.Index[string, UccWithEndpoints]
}

// FetchEndpointsForClient returns every CLA belonging to ucc, registering a KRT
// dependency narrowed to that client's rows.
func (ie *PerClientEnvoyEndpoints) FetchEndpointsForClient(kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient) []UccWithEndpoints {
	return krt.Fetch(kctx, ie.endpoints, krt.FilterIndex(ie.index, ucc.ResourceName()))
}

// NewPerClientEnvoyEndpoints resolves endpoints for each (client, backend) pair
// and builds its CLA. Equal results share a read-only proto; hashes select
// candidate buckets and content equality determines reuse.
// resolveEndpoints and buildClusterLoadAssignment are injected rather than called
// directly so this collection can be built against a test double.
func NewPerClientEnvoyEndpoints(
	krtopts krtutil.KrtOptions,
	uccs krt.Collection[ir.UniquelyConnectedClient],
	kgatewayEndpoints krt.Collection[ir.EndpointsForBackend],
	resolveEndpoints func(kctx krt.HandlerContext, ucc ir.UniquelyConnectedClient, ep ir.EndpointsForBackend) translator.ResolvedEndpoints,
	buildClusterLoadAssignment func(ucc ir.UniquelyConnectedClient, resolved translator.ResolvedEndpoints) *envoyendpointv3.ClusterLoadAssignment,
) PerClientEnvoyEndpoints {
	retainer := newCLARetainer()
	eps := krt.NewManyCollection(kgatewayEndpoints, func(kctx krt.HandlerContext, ep ir.EndpointsForBackend) []UccWithEndpoints {
		uccs := krt.Fetch(kctx, uccs)
		uccWithEndpointsRet := make([]UccWithEndpoints, 0, len(uccs))
		// Loop-invariant: every row in this transform shares the same backend.
		epName := ep.ResourceName()
		// Seed from retained CLAs so later clients reuse existing instances.
		// Hashes select candidates; clusterLoadAssignmentsEqual confirms content
		// and skips traversal of shared LbEndpoint pointers.
		claInterner := sharedproto.Interner[*envoyendpointv3.ClusterLoadAssignment]{Equal: clusterLoadAssignmentsEqual}
		retainer.seed(epName, &claInterner)
		for _, ucc := range uccs {
			resolved := resolveEndpoints(kctx, ucc, ep)
			endpointsHash := combineEndpointHash(resolved.Inputs.EndpointsForBackend.LbEpsEqualityHash, resolved.AdditionalHash, resolved.LoadBalancingHash)
			candidate := buildClusterLoadAssignment(ucc, resolved)
			cla := claInterner.Intern(candidate, endpointsHash)
			u := UccWithEndpoints{
				Client:        ucc,
				Endpoints:     cla,
				EndpointsHash: endpointsHash,
				endpointsName: epName,
				resourceName:  uccEndpointsResourceName(ucc, epName),
			}
			uccWithEndpointsRet = append(uccWithEndpointsRet, u)
		}
		retainer.keepIfPresent(epName, uccWithEndpointsRet, func() bool { return kgatewayEndpoints.GetKey(epName) != nil })
		return uccWithEndpointsRet
	}, krtopts.ToOptions("PerClientEnvoyEndpoints")...)
	// Deletes do not run the transform. Remove retained CLAs here, unless
	// the backend was re-added before this handler ran.
	kgatewayEndpoints.RegisterBatch(func(events []krt.Event[ir.EndpointsForBackend]) {
		for _, e := range events {
			if e.Event == controllers.EventDelete && e.Old != nil {
				name := e.Old.ResourceName()
				retainer.forgetIfAbsent(name, kgatewayEndpoints.GetKey(name) == nil)
			}
		}
	}, false)
	idx := krtpkg.UnnamedIndex(eps, func(ucc UccWithEndpoints) []string {
		return []string{ucc.Client.ResourceName()}
	})

	return PerClientEnvoyEndpoints{
		endpoints: eps,
		index:     idx,
	}
}

// combineEndpointHash combines endpoint, plugin, and load-balancing hashes.
func combineEndpointHash(parts ...uint64) uint64 {
	hasher := fnv.New64a()
	for _, part := range parts {
		utils.HashUint64(hasher, part)
	}
	return hasher.Sum64()
}
