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

// UccWithEndpoints is one client's view of one backend's endpoints: the
// ClusterLoadAssignment that client should receive, keyed by (client, backend).
// Clients that resolve identically share a single interned CLA, so the row count is
// per-pair but the proto count is per distinct result.
type UccWithEndpoints struct {
	Client ir.UniquelyConnectedClient
	// Endpoints is shared across clients and compared through ContentHash.
	// +noKrtEquals compared through ContentHash
	Endpoints sharedproto.Shared[*envoyendpointv3.ClusterLoadAssignment]
	// EndpointsHash tracks translation inputs, including changes that leave the
	// built assignment unchanged.
	EndpointsHash uint64
	// ContentHash digests the built assignment for EDS versioning. Both hashes
	// participate in row equality; only published content drives EDS updates.
	ContentHash   uint64
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
		c.ContentHash == in.ContentHash &&
		c.endpointsName == in.endpointsName
}

// claRetainer carries interned CLAs from one recomputation of the endpoints
// collection to the next, per backend.
//
// Without it, interning only shares among the clients present in a single pass,
// which is not where the sharing has to happen. Every client that connects
// re-runs this transform for every backend, and KRT keeps the object it already
// stored whenever [UccWithEndpoints.Equals] reports no change — which it does,
// because the CLA is deliberately not part of that comparison. So the already
// connected clients keep the proto from the pass they joined in, the newcomer
// keeps the one built in this pass, and clients that resolve identically end up
// holding one proto each. Clients connect one at a time, so that is the normal
// case, not a corner: a fleet coming up after a rolling restart would intern
// nothing at all.
//
// Seeding the next pass with what the last one handed out fixes that at the
// source: the rebuilt candidate finds the proto the stored rows already point
// at, so every client converges on one instance and KRT's "nothing changed"
// becomes true in the strong sense.
//
// Convergence holds as long as the retained entry is never dropped while rows
// still reference it. Two things could drop it: a pass whose rows reference a
// different instance (only after a content change, when KRT replaces the rows
// too), and the delete handler, which therefore forgets an entry only when the
// backend is absent from the source collection at the time the delete is
// handled; see forgetIfAbsent. Should the entry ever be lost anyway, the cost
// is bounded and never wrong bytes: the stored rows keep one instance, the
// next pass hands out another, and the split heals on the next content change.
//
// The mirror image is an entry that outlives its backend. The transform and
// the delete handler run on different goroutines, so a pass can store its
// entry after the backend was deleted again and after the delete handler
// already ran and found nothing to forget. Nothing would ever drop that
// entry, since a gone backend produces no more events. keepIfPresent closes
// that window by re-checking presence after the store: whichever of the two
// looks last sees the backend gone and drops the entry.
//
// Lifetime is bounded by construction rather than by policy. What is retained is
// replaced after every pass with exactly the set the returned rows reference, so
// it can never hold more than the live distinct results for that backend, and a
// superseded generation is released as soon as the rows that referenced it are.
// That matters here specifically: the bucket key is a content hash, so endpoint
// churn mints a new key on every change, and an interner that merely accumulated
// would pin one CLA per change forever — strictly worse than not interning.
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
	// Distinct results per backend is a small number - one per load-balancing
	// context, so in practice one per locality - which is why a linear scan
	// beats a map here.
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

// keepIfPresent is keep followed by a presence re-check. present must read the
// source collection's current state, not the event that started this pass: by
// the time the pass stores its entry the backend can already be deleted again,
// with the delete handler having run while the entry was not yet there. Checking
// after the store means one of the two observers always sees the deletion.
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

// forgetIfAbsent is forget guarded by whether the backend is still in the source
// collection at the time the delete event is handled. Delete handlers run on
// their own goroutine, so a backend deleted and re-added in quick succession can
// have its re-add pass seed from and replace the entry before the delete is
// observed here; dropping the entry then would split the fleet between the
// stored rows' proto and the next pass's fresh one until the next endpoint
// change. When the backend is present again, its own transform owns the entry.
func (r *claRetainer) forgetIfAbsent(backend string, absent bool) {
	if !absent {
		return
	}
	r.forget(backend)
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
// whose built CLAs are equal are handed the same read-only proto. Resolution hashes
// narrow the equality checks to a bucket but are not themselves proof of equality.
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
		// Intern equal CLAs across UCCs. The resolved-input hash selects a small
		// candidate bucket; the interner confirms protobuf equality so a 64-bit
		// collision cannot make different clients share the wrong assignment.
		//
		// Seeded with what the previous pass handed out, so a client that connects
		// later converges on the proto the already-stored rows point at rather than
		// starting a generation of its own. See [claRetainer].
		// Candidates in one bucket alias their LbEndpoint protos through the
		// endpoint IR, and the pinned protobuf-go's proto.Equal walks every
		// nested field regardless, so equality uses an identity-aware
		// comparison that agrees with proto.Equal (see clusterLoadAssignmentsEqual).
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
				ContentHash:   contentHashOf(candidate),
				endpointsName: epName,
				resourceName:  uccEndpointsResourceName(ucc, epName),
			}
			uccWithEndpointsRet = append(uccWithEndpointsRet, u)
		}
		retainer.keepIfPresent(epName, uccWithEndpointsRet, func() bool { return kgatewayEndpoints.GetKey(epName) != nil })
		return uccWithEndpointsRet
	}, krtopts.ToOptions("PerClientEnvoyEndpoints")...)
	// A deleted backend never runs the transform again, so its retained CLAs have
	// to be dropped here or they outlive every row that referenced them.
	//
	// This handler runs on its own goroutine, not on the transform's queue, so
	// a delete followed quickly by a re-add can be observed here after the
	// re-add's pass has already seeded from and replaced the entry. Forgetting
	// in that order would discard the live generation: the next pass would
	// build a fresh proto, KRT would keep the already-stored rows (equal hash),
	// and the fleet would sit on two instances until the next endpoint change.
	// So the entry is dropped only when the backend is really gone. If it has
	// been re-added, its own transform owns the entry and keeps it current.
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

// contentHashOf digests a built assignment; nil has a stable zero digest.
func contentHashOf(cla *envoyendpointv3.ClusterLoadAssignment) uint64 {
	if cla == nil {
		return 0
	}
	return utils.HashProto(cla)
}
