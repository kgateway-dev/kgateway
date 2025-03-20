package waypointquery

import (
	"context"
	"fmt"

	"istio.io/api/label"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/slices"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

const (
	// IstioUseWaypointLabel is the label used to specify which waypoint should be used for a given pod, service, etc...
	// `istio.io/use-waypoint: none` means skipping using any waypoint specified from higher scope, namespace/service, etc...
	IstioUseWaypointLabel = "istio.io/use-waypoint"
	// IstioUseWaypointNamespaceLabel is a label used to indicate the namespace of the waypoint (referred to by AmbientUseWaypointLabel).
	// This allows cross-namespace waypoint references. If unset, the same namespace is assumed.
	IstioUseWaypointNamespaceLabel = "istio.io/use-waypoint-namespace"
)

type WaypointQueries interface {
	// GetWaypointServices returns all Services that are marked as using the Gateway
	// via istio.io/use-waypoint (and possibly istio.io/use-waypoint-namespace).
	GetWaypointServices(kctx krt.HandlerContext, ctx context.Context, gw *gwv1.Gateway) []Service

	// GetHTTPRoutesForService fetches HTTPRoutes that have the given Service in parentRefs.
	GetHTTPRoutesForService(kctx krt.HandlerContext, ctx context.Context, svc *Service) []query.RouteInfo

	HasSynced() bool
}

func NewQueries(
	commonCols *common.CommonCollections,
	gwQueries query.GatewayQueries,
) WaypointQueries {
	waypointedServices, servicesByWaypoint := waypointAttachmentIndex(commonCols)
	return &waypointQueries{
		queries:            gwQueries,
		commonCols:         commonCols,
		waypointedServices: waypointedServices,
		servicesByWaypoint: servicesByWaypoint,
	}
}

type waypointQueries struct {
	queries    query.GatewayQueries
	commonCols *common.CommonCollections

	waypointedServices krt.Collection[WaypointedService]
	servicesByWaypoint krt.Index[types.NamespacedName, WaypointedService]
}

func (w *waypointQueries) HasSynced() bool {
	return w.waypointedServices.HasSynced()
}

func (w *waypointQueries) GetHTTPRoutesForService(
	kctx krt.HandlerContext,
	ctx context.Context,
	svc *Service,
) []query.RouteInfo {
	nns := types.NamespacedName{
		Namespace: svc.GetNamespace(),
		Name:      svc.GetName(),
	}
	routes := w.commonCols.Routes.RoutesFor(kctx, nns, wellknown.ServiceGVK.Group, wellknown.ServiceGVK.Kind)
	// resolve delegation
	out := slices.MapFilter(routes, func(route ir.Route) *query.RouteInfo {
		pRef := findParentRef(
			svc,
			route.GetNamespace(),
			route.GetParentRefs(),
			svc.GroupKind,
		)
		if pRef == nil {
			return nil
		}
		return w.queries.GetRouteChain(kctx, ctx, route, nil, *pRef)
	})
	return out
}

// findParentRef that targets the given object
func findParentRef(
	svc *Service,
	routeNs string,
	parentRefs []gwv1.ParentReference,
	gk schema.GroupKind,
) *gwv1.ParentReference {
	// TODO peering will need to consider original and simulated GK
	matchingParentRefs := findParentRefsForType(parentRefs, gk.Group, gk.Kind)
	for _, pr := range matchingParentRefs {
		// default to routes's own ns if not specified on the ref
		ns := routeNs
		if pr.Namespace != nil {
			ns = string(*pr.Namespace)
		}
		if string(pr.Name) == svc.GetName() && ns == svc.GetNamespace() {
			return pr
		}
	}
	return nil
}

func findParentRefsForType(refs []gwv1.ParentReference, targetGroup, targetKind string) []*gwv1.ParentReference {
	var matchingParentRefs []*gwv1.ParentReference
	for _, pr := range refs {
		prGroup := wellknown.GatewayGVK.Group
		prKind := wellknown.GatewayGVK.Kind
		if pr.Group != nil {
			prGroup = string(*pr.Group)
		}
		if pr.Kind != nil {
			prKind = string(*pr.Kind)
		}
		if compareCanonicalGroup(prGroup, targetGroup) && prKind == targetKind {
			matchingParentRefs = append(matchingParentRefs, &pr)
		}
	}
	return matchingParentRefs
}

func compareCanonicalGroup(a, b string) bool {
	if a == "core" {
		a = ""
	}
	if b == "core" {
		b = ""
	}
	return a == b
}

func (w *waypointQueries) GetWaypointServices(kctx krt.HandlerContext, ctx context.Context, gw *gwv1.Gateway) []Service {
	attached := krt.Fetch(kctx, w.waypointedServices, krt.FilterIndex(w.servicesByWaypoint, types.NamespacedName{
		Name:      gw.GetName(),
		Namespace: gw.GetNamespace(),
	}))
	return slices.Map(attached, func(e WaypointedService) Service {
		return e.Service
	})
}

type WaypointedService struct {
	Waypoint types.NamespacedName
	Service  Service
}

func (wa WaypointedService) ResourceName() string {
	// TODO this also needs to be the original (non-peering)
	// group/kind/name/namesspace

	// TODO seems like this returns empty?
	gk := wa.Service.GroupKind

	return fmt.Sprintf("%s/%s(%s/%s)[%s/%s]",
		wa.Service.GetName(), wa.Service.GetNamespace(),
		gk.Group, gk.Kind,
		wa.Waypoint.Namespace, wa.Waypoint.Name,
	)
}

func waypointAttachmentIndex(
	commonCols *common.CommonCollections,
) (
	krt.Collection[WaypointedService],
	krt.Index[types.NamespacedName, WaypointedService],
) {
	// TODO we may want to expand the "logical Service" concept outside of this
	// package to capture both Service and ServiceEntry this will help de-dupe
	// and de-risk handling each of those for peering and waypoint
	// purposes
	serviceWithWaypoint := krt.NewCollection(commonCols.Services, func(ctx krt.HandlerContext, svc *corev1.Service) *WaypointedService {
		// direct attachment
		waypoint, waypointNone := getUseWaypoint(svc.GetLabels(), svc.GetNamespace())
		if waypointNone {
			// explicitly don't want it
			return nil
		}

		// try Namespace attachment
		if waypoint == nil {
			nsMeta := krt.FetchOne(ctx, commonCols.Namespaces, krt.FilterKey(svc.GetNamespace()))
			if nsMeta != nil {
				waypoint, waypointNone = getUseWaypoint(nsMeta.Labels, nsMeta.Name)
				if waypointNone {
					// explicitly don't want it
					return nil
				}
			}
		}

		// no waypoint labels found
		if waypoint == nil {
			return nil
		}

		return &WaypointedService{
			Waypoint: *waypoint,
			Service:  FromService(svc),
		}
	}, commonCols.KrtOpts.ToOptions("KubeServiceToWaypoints")...)

	attachments := krt.JoinCollection([]krt.Collection[WaypointedService]{
		serviceWithWaypoint,
		// TODO serviceentry
	}, commonCols.KrtOpts.ToOptions("ServiceWaypoints")...)
	byGateway := krt.NewIndex(attachments, func(o WaypointedService) []types.NamespacedName {
		return []types.NamespacedName{o.Waypoint}
	})
	return attachments, byGateway
}

// getUseWaypoint returns the NamespacedName of the waypoint the given object uses.
// It also returns a bool that indicates we specifically want NO Waypoint.
func getUseWaypoint(labels map[string]string, defaultNamespace string) (named *types.NamespacedName, isNone bool) {
	if labelValue, ok := labels[label.IoIstioUseWaypoint.Name]; ok {
		if labelValue == "none" {
			return nil, true
		}
		namespace := defaultNamespace
		if override, f := labels[label.IoIstioUseWaypointNamespace.Name]; f {
			namespace = override
		}
		return &types.NamespacedName{
			Name:      labelValue,
			Namespace: namespace,
		}, false
	}
	return nil, false
}
