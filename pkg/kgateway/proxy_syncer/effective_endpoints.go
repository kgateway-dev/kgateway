package proxy_syncer

import (
	"istio.io/istio/pkg/kube/krt"

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
		// A same-named EDS cluster can still re-warm when policy changes CDS, so
		// the endpoint version has to move with policy content. Setting the
		// policy view is enough: LbEpsEqualityHash derives from it, and the
		// ordering against Add no longer matters.
		final.SetAttachedPolicies(backend.AttachedPolicies)
		final.ClusterName = backend.ClusterName()
		final.UpstreamResourceName = backend.ResourceName()
		for locality, endpoints := range raw.LbEps {
			for _, endpoint := range endpoints {
				final.Add(locality, endpoint)
			}
		}
		return &final
	}, krtopts.ToOptions("FinalBackendEndpoints")...)
}
