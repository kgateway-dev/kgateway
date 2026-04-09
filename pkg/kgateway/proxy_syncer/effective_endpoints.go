package proxy_syncer

import (
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// newFinalBackendEndpoints remaps raw endpoint data onto the final backend view
// so EDS stays aligned with the cluster names and resource identities selected
// for routes and clusters after policy attachment.
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
		return &final
	}, krtopts.ToOptions("FinalBackendEndpoints")...)
}
