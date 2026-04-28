package proxy_syncer

import (
	"hash/fnv"
	"sort"
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
		if raw == nil {
			return nil
		}

		final := raw.EmptyCopy()
		final.ClusterName = backend.ClusterName()
		final.UpstreamResourceName = backend.ResourceName()
		for locality, endpoints := range raw.LbEps {
			for _, endpoint := range endpoints {
				final.Add(locality, endpoint)
			}
		}
		// A same-named EDS cluster can still re-warm when policy changes CDS.
		// Bump only the endpoint version so Envoy receives a fresh CLA response.
		if policyHash := backendEndpointVersionHash(backend); policyHash != 0 {
			final.LbEpsEqualityHash = combineEndpointHashes(final.LbEpsEqualityHash, policyHash)
		}
		return &final
	}, krtopts.ToOptions("FinalBackendEndpoints")...)
}

func backendEndpointVersionHash(backend *ir.BackendObjectIR) uint64 {
	if backend == nil || len(backend.AttachedPolicies.Policies) == 0 {
		return 0
	}

	hasher := fnv.New64a()
	groupKinds := make([]schema.GroupKind, 0, len(backend.AttachedPolicies.Policies))
	for groupKind := range backend.AttachedPolicies.Policies {
		groupKinds = append(groupKinds, groupKind)
	}
	sort.Slice(groupKinds, func(i, j int) bool {
		if groupKinds[i].Group == groupKinds[j].Group {
			return groupKinds[i].Kind < groupKinds[j].Kind
		}
		return groupKinds[i].Group < groupKinds[j].Group
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

func combineEndpointHashes(endpointHash, policyHash uint64) uint64 {
	hasher := fnv.New64a()
	utils.HashUint64(hasher, endpointHash)
	utils.HashUint64(hasher, policyHash)
	return hasher.Sum64()
}
