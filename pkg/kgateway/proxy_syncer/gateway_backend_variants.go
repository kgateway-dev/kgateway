package proxy_syncer

import (
	"cmp"
	"context"
	"slices"

	"istio.io/istio/pkg/kube/krt"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/gatewaytls"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

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

func newGatewayBackendVariants(
	ctx context.Context,
	krtopts krtutil.KrtOptions,
	commonCols *collections.CommonCollections,
	queries query.GatewayQueries,
	gateways krt.Collection[ir.Gateway],
) krt.Collection[gatewayScopedBackend] {
	return krt.NewManyCollection(gateways, func(kctx krt.HandlerContext, gateway ir.Gateway) []gatewayScopedBackend {
		// Translation resolves and reports backend client certificate errors on
		// Gateway status. Keep the collection quiet here because it may recompute
		// frequently and would otherwise emit duplicate log noise for the same
		// user-facing error.
		clientCertificate, err := gatewaytls.ResolveBackendClientCertificate(&gateway, func(secretRef gwv1.SecretObjectReference) (*ir.Secret, error) {
			return commonCols.Secrets.GetSecret(kctx, krtcollections.From{
				GroupKind: gateway.GetGroupKind(),
				Namespace: gateway.GetNamespace(),
			}, secretRef)
		})
		if err != nil {
			return nil
		}
		if clientCertificate == nil {
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
		for locality, endpoints := range base.LbEps {
			for _, endpoint := range endpoints {
				clone.Add(locality, endpoint)
			}
		}

		return clone
	}, krtopts.ToOptions("GatewayBackendClientCertificateVariantEndpoints")...)
}
