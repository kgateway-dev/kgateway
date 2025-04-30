package waypointquery

import (
	"context"
	"fmt"

	"istio.io/api/label"
	authcr "istio.io/client-go/pkg/apis/security/v1"
	"istio.io/istio/pkg/config/schema/gvr"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/kubetypes"
	"istio.io/istio/pkg/slices"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"

	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
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

	// GetServiceWaypoint returns the waypoint for the given object (Service or ServiceEntry).
	// Returns nil if no waypoint is found.
	GetServiceWaypoint(kctx krt.HandlerContext, ctx context.Context, obj metav1.Object) *types.NamespacedName

	// GetHTTPRoutesForService fetches HTTPRoutes that have the given Service in parentRefs.
	GetHTTPRoutesForService(kctx krt.HandlerContext, ctx context.Context, svc *Service) []query.RouteInfo

	// GetAuthorizationPoliciesForGateway returns policies targeting a specific gateway
	GetAuthorizationPoliciesForGateway(kctx krt.HandlerContext, ctx context.Context, gateway *gwv1.Gateway, rootNamespace string) []*authcr.AuthorizationPolicy

	// GetAuthorizationPoliciesForService returns policies targeting a specific service
	GetAuthorizationPoliciesForService(kctx krt.HandlerContext, ctx context.Context, svc *Service) []*authcr.AuthorizationPolicy

	HasSynced() bool
}

func NewQueries(
	commonCols *common.CommonCollections,
	gwQueries query.GatewayQueries,
) WaypointQueries {
	waypointedServices, servicesByWaypoint, waypointByService := waypointAttachmentIndex(commonCols)

	// Watch authz policies changes in the cluster.
	authzInformer := kclient.NewDelayedInformer[*authcr.AuthorizationPolicy](
		commonCols.Client,
		gvr.AuthorizationPolicy,
		kubetypes.StandardInformer,
		kclient.Filter{ObjectFilter: commonCols.Client.ObjectFilter()},
	)
	authzPolicies := krt.WrapClient(authzInformer, commonCols.KrtOpts.ToOptions("AuthorizationPolicies")...)
	byNamespace := krt.NewIndex(authzPolicies, func(p *authcr.AuthorizationPolicy) []string {
		return []string{p.GetNamespace()}
	})
	// Build Authz policies targetRefKey index
	byTargetRefKey := buildAuthzTargetIndex(authzPolicies, commonCols.Settings.IstioNamespace)

	return &waypointQueries{
		queries:            gwQueries,
		commonCols:         commonCols,
		waypointedServices: waypointedServices,
		servicesByWaypoint: servicesByWaypoint,
		waypointByService:  waypointByService,
		authzPolicies:      authzPolicies,
		byNamespace:        byNamespace,
		byTargetRefKey:     byTargetRefKey,
	}
}

// Helper function for determining effective namespace
func getEffectiveNamespace(targetNs, policyNs string) string {
	if targetNs != "" {
		return targetNs
	}
	return policyNs
}

type waypointQueries struct {
	queries    query.GatewayQueries
	commonCols *common.CommonCollections

	waypointedServices krt.Collection[WaypointedService]
	servicesByWaypoint krt.Index[types.NamespacedName, WaypointedService]
	waypointByService  krt.Index[string, WaypointedService]
	authzPolicies      krt.Collection[*authcr.AuthorizationPolicy]
	byNamespace        krt.Index[string, *authcr.AuthorizationPolicy]
	byTargetRefKey     krt.Index[targetRefKey, *authcr.AuthorizationPolicy]
}

func (w *waypointQueries) HasSynced() bool {
	waypointSync := w.waypointedServices.HasSynced()
	authzSync := w.authzPolicies.HasSynced()
	return waypointSync && authzSync
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

	targetGK := svc.GroupKind
	routes := w.commonCols.Routes.RoutesFor(kctx, nns, targetGK.Group, targetGK.Kind)

	// also allow route attachment via Hostname kind
	// this behavior is unique to Waypoint, as parentRef to Service[Entry] is a GAMMA-only pattern.
	hostnameGK := wellknown.HostnameGVK.GroupKind()
	for _, host := range svc.Hostnames {
		routes = append(routes, w.commonCols.Routes.RoutesFor(kctx, types.NamespacedName{
			Name: host,
			// TODO currently the routes parentRef index must have namespace
			// this means currently, we can only find namespace-local policies like this
			// OR policies that use Hostname and specify this Service[Entry]'s namespace.
			Namespace: nns.Namespace,
		}, hostnameGK.Group, hostnameGK.Kind)...)
	}

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

	hostnameGK := wellknown.HostnameGVK.GroupKind()
	hostnameParentRefs := findParentRefsForType(parentRefs, hostnameGK.Group, hostnameGK.Kind)
	for _, host := range svc.Hostnames {
		for _, pr := range hostnameParentRefs {
			if string(pr.Name) == host {
				return pr
			}
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

func (w *waypointQueries) GetServiceWaypoint(kctx krt.HandlerContext, ctx context.Context, obj metav1.Object) *types.NamespacedName {
	key := ServiceKeyFromObject(obj)
	if key == "" {
		return nil
	}
	attached := krt.FetchOne(kctx, w.waypointedServices, krt.FilterIndex(w.waypointByService, key))
	if attached == nil {
		return nil
	}
	return &attached.Waypoint
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

func doWaypointAttachment(
	ctx krt.HandlerContext,
	commonCols *common.CommonCollections,
	svc Service,
) *WaypointedService {
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
		Service:  svc,
	}
}

func waypointAttachmentIndex(
	commonCols *common.CommonCollections,
) (
	krt.Collection[WaypointedService],
	krt.Index[types.NamespacedName, WaypointedService],
	krt.Index[string, WaypointedService],
) {
	// TODO we may want to expand the "logical Service" concept outside of this
	// package so it can be used for all policy attachment, peering, routing, etc.

	// do basic attachment logic
	waypointServiceAttachments := krt.JoinCollection(
		[]krt.Collection[WaypointedService]{
			krt.NewCollection(commonCols.Services, func(ctx krt.HandlerContext, kubeSvc *corev1.Service) *WaypointedService {
				return doWaypointAttachment(ctx, commonCols, FromService(kubeSvc))
			}, commonCols.KrtOpts.ToOptions("WaypointKubeServices")...),
			krt.NewCollection(commonCols.ServiceEntries, func(ctx krt.HandlerContext, istioSE *networkingclient.ServiceEntry) *WaypointedService {
				return doWaypointAttachment(ctx, commonCols, FromServiceEntry(istioSE))
			}, commonCols.KrtOpts.ToOptions("WaypointServiceEntries")...),
		},
		commonCols.KrtOpts.ToOptions("WaypointLogicalServices")...,
	)

	// enable lookup by gateway
	byWaypointGateway := krt.NewIndex(waypointServiceAttachments, func(o WaypointedService) []types.NamespacedName {
		return []types.NamespacedName{o.Waypoint}
	})

	waypointAttachmentsByService := krt.NewIndex(waypointServiceAttachments, func(o WaypointedService) []string {
		return []string{o.Service.String()}
	})

	return waypointServiceAttachments, byWaypointGateway, waypointAttachmentsByService
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
