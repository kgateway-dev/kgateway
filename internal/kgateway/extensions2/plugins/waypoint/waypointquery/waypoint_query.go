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
	"istio.io/istio/pkg/util/sets"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"

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
	commonCols *collections.CommonCollections,
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
	byNamespace := krtutil.UnnamedIndex(authzPolicies, func(p *authcr.AuthorizationPolicy) []string {
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
	commonCols *collections.CommonCollections

	waypointedServices krt.Collection[WaypointedService]
	servicesByWaypoint krt.Index[types.NamespacedName, WaypointedService]
	waypointByService  krt.Index[string, WaypointedService]
	authzPolicies      krt.Collection[*authcr.AuthorizationPolicy]
	byNamespace        krt.Index[string, *authcr.AuthorizationPolicy]
	byTargetRefKey     krt.Index[ir.ObjectSource, *authcr.AuthorizationPolicy]
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
	var out []query.RouteInfo
	seen := sets.New[types.NamespacedName]()
	for _, key := range svc.Keys() {
		nns := types.NamespacedName{
			// TODO  routes index requires a namespace
			// meaning global references (such as Hostname) are effectively namespace-local
			Namespace: getEffectiveNamespace(key.GetNamespace(), svc.GetNamespace()),
			Name:      key.GetName(),
		}
		routes := w.commonCols.Routes.RoutesFor(kctx, nns, key.Group, key.Kind)
		out = append(out, slices.MapFilter(routes, func(route ir.Route) *query.RouteInfo {
			if seen.InsertContains(types.NamespacedName{
				Namespace: route.GetNamespace(),
				Name:      route.GetName(),
			}) {
				return nil
			}

			// resolve delegation
			pRef := findParentRef(
				key,
				route.GetNamespace(),
				route.GetParentRefs(),
				key.GetGroupKind(),
			)
			if pRef == nil {
				return nil
			}
			return w.queries.GetRouteChain(kctx, ctx, route, nil, *pRef)
		})...)
	}

	return out
}

// findParentRef that targets the given object
func findParentRef(
	key ir.ObjectSource,
	routeNs string,
	parentRefs []gwv1.ParentReference,
	gk schema.GroupKind,
) *gwv1.ParentReference {
	// TODO peering will need to consider original and simulated GK
	matchingParentRefs := findParentRefsForType(parentRefs, gk.Group, gk.Kind)
	for _, pr := range matchingParentRefs {
		if string(pr.Name) != key.GetName() {
			continue
		}

		// global key, no namespace
		if key.GetNamespace() == "" && pr.Namespace == nil {
			return pr
		}

		// default to routes's own ns if not specified on the ref
		ns := routeNs
		if pr.Namespace != nil {
			ns = string(*pr.Namespace)
		}
		if key.GetNamespace() == ns {
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
	commonCols *collections.CommonCollections,
	svc Service,
) *WaypointedService {
	// look at aliases and the actual object ns
	// NOTE: we respect aliases over the original object (see svc.Keys())
	seenNs := sets.New[string]()
	for _, k := range svc.Keys() {
		ns := k.GetNamespace()
		if ns == "" || seenNs.InsertContains(ns) {
			continue
		}

		// direct attachment - object's own labels
		waypoint, waypointNone := getUseWaypoint(svc.GetLabels(), ns)
		if waypointNone {
			// explicitly don't want it
			return nil
		}

		// try Namespace attachment
		if waypoint == nil {
			nsMeta := krt.FetchOne(ctx, commonCols.Namespaces, krt.FilterKey(ns))
			if nsMeta != nil {
				waypoint, waypointNone = getUseWaypoint(nsMeta.Labels, nsMeta.Name)
				if waypointNone {
					// explicitly don't want it
					return nil
				}
			}
		}

		if waypoint != nil {
			return &WaypointedService{
				Waypoint: *waypoint,
				Service:  svc,
			}
		}
	}
	return nil
}

func waypointAttachmentIndex(
	commonCols *collections.CommonCollections,
) (
	krt.Collection[WaypointedService],
	krt.Index[types.NamespacedName, WaypointedService],
	krt.Index[string, WaypointedService],
) {
	// do basic attachment logic
	waypointServiceAttachments := krt.JoinCollection(
		[]krt.Collection[WaypointedService]{
			krt.NewCollection(commonCols.Services, func(ctx krt.HandlerContext, kubeSvc *corev1.Service) *WaypointedService {
				return doWaypointAttachment(ctx, commonCols, FromService(kubeSvc))
			}, commonCols.KrtOpts.ToOptions("WaypointKubeServices")...),
			krt.NewCollection(commonCols.ServiceEntries, func(ctx krt.HandlerContext, istioSE *networkingclient.ServiceEntry) *WaypointedService {
				aliases := getAliases(ctx, commonCols, istioSE)
				return doWaypointAttachment(ctx, commonCols, FromServiceEntry(istioSE, aliases))
			}, commonCols.KrtOpts.ToOptions("WaypointServiceEntries")...),
		},
		commonCols.KrtOpts.ToOptions("WaypointLogicalServices")...,
	)

	// enable lookup by gateway
	byWaypointGateway := krtutil.UnnamedIndex(waypointServiceAttachments, func(o WaypointedService) []types.NamespacedName {
		return []types.NamespacedName{o.Waypoint}
	})

	waypointAttachmentsByService := krtutil.UnnamedIndex(waypointServiceAttachments, func(o WaypointedService) []string {
		return []string{o.Service.String()}
	})

	return waypointServiceAttachments, byWaypointGateway, waypointAttachmentsByService
}

func getAliases(
	ctx krt.HandlerContext,
	commonCols *collections.CommonCollections,
	se *networkingclient.ServiceEntry,
) []ir.ObjectSource {
	if len(se.Spec.GetPorts()) == 0 {
		// require a port since we find aliases via BackendIndex
		// this is fine b/c a ServiceEntry with no ports isn't reachable via waypoint
		return nil
	}
	objSrc := ir.ObjectSource{
		Group:     wellknown.ServiceEntryGVK.Group,
		Kind:      wellknown.ServiceEntryGVK.Kind,
		Namespace: se.GetNamespace(),
		Name:      se.GetName(),
	}
	be, _ := commonCols.BackendIndex.GetBackendFromRef(ctx, objSrc, gwv1.BackendObjectReference{
		Group:     ptr.To(gwv1.Group(objSrc.Group)),
		Kind:      ptr.To(gwv1.Kind(objSrc.Kind)),
		Name:      gwv1.ObjectName(objSrc.Name),
		Namespace: ptr.To(gwv1.Namespace(objSrc.Namespace)),
		Port:      ptr.To(gwv1.PortNumber(se.Spec.GetPorts()[0].GetNumber())), //nolint:gosec // G115: ServiceEntry port number is uint32, safe to convert to PortNumber (int32)
	})
	if be == nil {
		return nil
	}
	return be.Aliases
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
