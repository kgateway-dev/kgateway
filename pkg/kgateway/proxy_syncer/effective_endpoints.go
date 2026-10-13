package proxy_syncer

import (
	"cmp"
	"hash/fnv"
	"slices"
	"strconv"

	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// newFinalBackendEndpoints rebuilds endpoint IR from the policy-attached backend
// view so EDS follows the same backend lifecycle as CDS and routes.
func newFinalBackendEndpoints(
	krtopts krtutil.KrtOptions,
	finalBackends krt.Collection[*ir.BackendObjectIR],
	rawEndpoints krt.Collection[ir.EndpointsForBackend],
) krt.Collection[ir.EndpointsForBackend] {
	return krt.NewCollection(finalBackends, func(kctx krt.HandlerContext, backend *ir.BackendObjectIR) *ir.EndpointsForBackend {
		raw := krt.FetchOne(kctx, rawEndpoints, krt.FilterKey(backend.ResourceName()))
		return FinalBackendEndpoints(backend, raw)
	}, krtopts.ToOptions("FinalBackendEndpoints")...)
}

// FinalBackendEndpoints projects a backend's raw endpoint IR onto the
// policy-attached backend view, returning the EDS input the endpoint plugins
// and the endpoint translator actually run on. Returns nil when the backend
// contributes no endpoints.
//
// This is the per-backend body of the FinalBackendEndpoints collection. It is
// exported because the raw endpoint collections are contributed by backend
// plugins and are built before policies are attached: anything that translates
// endpoints outside the syncer — the translator golden tests, for one — has to
// go through here or every endpoint plugin silently sees no policies.
func FinalBackendEndpoints(backend *ir.BackendObjectIR, raw *ir.EndpointsForBackend) *ir.EndpointsForBackend {
	if backend == nil || raw == nil {
		return nil
	}

	final := raw.EmptyCopy()
	final.AttachedPolicies = backend.AttachedPolicies
	final.ClusterName = backend.ClusterName()
	final.UpstreamResourceName = backend.ResourceName()
	// Reuse the endpoint protos AND their precomputed equality hash instead
	// of re-Adding every endpoint: Add re-marshals each LbEndpoint proto
	// (HashProtoWithHasher), which is a major allocation source at scale.
	final.ReuseEndpointsFrom(raw)
	// A same-named EDS cluster can still re-warm when policy changes CDS.
	// Bump only the endpoint version so Envoy receives a fresh CLA response.
	if policyHash := backendEndpointVersionHash(backend); policyHash != 0 {
		final.FoldVersion(policyHash)
	}
	return &final
}

// backendEndpointVersionHash hashes attached policies so policy-only endpoint
// changes invalidate KRT rows. It returns zero for no policies, which callers
// treat as "contributes nothing".
func backendEndpointVersionHash(backend *ir.BackendObjectIR) uint64 {
	if backend == nil || len(backend.AttachedPolicies.Policies) == 0 {
		return 0
	}

	hasher := fnv.New64a()
	groupKinds := make([]schema.GroupKind, 0, len(backend.AttachedPolicies.Policies))
	for groupKind := range backend.AttachedPolicies.Policies {
		groupKinds = append(groupKinds, groupKind)
	}
	slices.SortFunc(groupKinds, func(a, b schema.GroupKind) int {
		if a.Group != b.Group {
			return cmp.Compare(a.Group, b.Group)
		}
		return cmp.Compare(a.Kind, b.Kind)
	})

	for _, groupKind := range groupKinds {
		utils.HashStringField(hasher, groupKind.Group)
		utils.HashStringField(hasher, groupKind.Kind)
		for _, policy := range backend.AttachedPolicies.Policies[groupKind] {
			utils.HashStringField(hasher, ir.PolicyRefString(policy.PolicyRef))
			utils.HashStringField(hasher, strconv.FormatInt(policy.Generation, 10))
			for _, err := range policy.Errors {
				if err != nil {
					utils.HashStringField(hasher, err.Error())
				}
			}
			if policy.PolicyIr == nil {
				continue
			}
			if hashable, ok := policy.PolicyIr.(ir.PolicyHashIR); ok {
				utils.HashUint64(hasher, hashable.PolicyHash())
				continue
			}
			utils.HashStringField(hasher, strconv.FormatInt(policy.PolicyIr.CreationTime().UnixNano(), 10))
		}
	}

	return hasher.Sum64()
}
