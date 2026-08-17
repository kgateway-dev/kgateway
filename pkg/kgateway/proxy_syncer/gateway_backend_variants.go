package proxy_syncer

import (
	"cmp"
	"context"
	"slices"

	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/gatewaytls"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// gatewayScopedBackend is a backend rewritten for one Gateway's client certificate.
// A Service referenced from two Gateways with different client certificates has to
// become two distinct Envoy clusters, so each variant is a separate row carrying the
// resource name of the backend it was derived from.
type gatewayScopedBackend struct {
	baseResourceName string
	backend          *ir.BackendObjectIR
}

func (v gatewayScopedBackend) ResourceName() string {
	if v.backend == nil {
		return v.baseResourceName
	}
	return v.backend.ResourceName()
}

func (v gatewayScopedBackend) Equals(other gatewayScopedBackend) bool {
	if v.baseResourceName != other.baseResourceName {
		return false
	}
	if v.backend == nil || other.backend == nil {
		return v.backend == other.backend
	}
	return v.backend.Equals(*other.backend)
}

// newGatewayBackendVariants derives a [gatewayScopedBackend] for every backend
// reachable from a Gateway that configures a backend client certificate. Gateways
// without one contribute nothing, which is the common case.
//
// Rows are emitted in resource-name order so the collection is stable across
// recomputes.
func newGatewayBackendVariants(
	ctx context.Context,
	krtopts krtutil.KrtOptions,
	queries query.GatewayQueries,
	gateways krt.Collection[ir.Gateway],
) krt.Collection[gatewayScopedBackend] {
	return krt.NewManyCollection(gateways, func(kctx krt.HandlerContext, gateway ir.Gateway) []gatewayScopedBackend {
		// Translation resolves and reports backend client certificate errors on
		// Gateway status. Keep the collection quiet here because it may recompute
		// frequently and would otherwise emit duplicate log noise for the same
		// user-facing error.
		clientCertificate, err := gatewaytls.ResolveForGateway(kctx, ctx, queries, &gateway)
		if err != nil || clientCertificate == nil {
			return nil
		}

		// Resolve again inside the KRT collection even though translation also
		// does so. This makes the backend and endpoint variants depend directly on
		// the referenced Secret, so Secret updates recompute the collection.
		routesForGw, err := queries.GetRoutesForGateway(kctx, ctx, &gateway)
		if err != nil {
			logger.Error("failed to get routes for gateway backend variants", "gateway", gateway.ResourceName(), "error", err)
			return nil
		}

		gatewayScopedBackends := query.BuildGatewayBackendClientCertificateVariants(routesForGw, &gateway, clientCertificate)
		result := make([]gatewayScopedBackend, 0, len(gatewayScopedBackends))
		for baseResourceName, backend := range gatewayScopedBackends {
			result = append(result, gatewayScopedBackend{
				baseResourceName: baseResourceName,
				backend:          backend,
			})
		}

		slices.SortFunc(result, func(a, b gatewayScopedBackend) int {
			return cmp.Compare(a.ResourceName(), b.ResourceName())
		})

		return result
	}, krtopts.ToOptions("GatewayBackendClientCertificateVariants")...)
}

// newGatewayBackendVariantEndpoints gives each backend variant its own endpoint row,
// since a variant is a distinct Envoy cluster and EDS is keyed by cluster name. The
// endpoints themselves are identical to the base backend's, so the row reuses the
// base's protos and equality hash rather than recomputing either.
func newGatewayBackendVariantEndpoints(
	krtopts krtutil.KrtOptions,
	variants krt.Collection[gatewayScopedBackend],
	baseEndpoints krt.Collection[ir.EndpointsForBackend],
) krt.Collection[ir.EndpointsForBackend] {
	return krt.NewCollection(variants, func(kctx krt.HandlerContext, variant gatewayScopedBackend) *ir.EndpointsForBackend {
		if variant.backend == nil {
			return nil
		}

		base := krt.FetchOne(kctx, baseEndpoints, krt.FilterKey(variant.baseResourceName))
		if base == nil {
			return nil
		}

		clone := ir.NewEndpointsForBackend(*variant.backend)
		// The endpoint protos are shared with the base backend; reuse the
		// precomputed equality hash as well to avoid re-marshaling every
		// LbEndpoint proto (HashProtoWithHasher) on each recompute.
		clone.ReuseEndpointsFrom(base)

		return clone
	}, krtopts.ToOptions("GatewayBackendClientCertificateVariantEndpoints")...)
}
