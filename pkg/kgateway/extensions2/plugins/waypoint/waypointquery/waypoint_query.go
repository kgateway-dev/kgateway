package waypointquery

import (
	"context"
	"fmt"

	"istio.io/api/label"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/util/sets"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
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
	// GetServiceWaypoint returns the waypoint for the given object (Service or ServiceEntry).
	// Returns nil if no waypoint is found.
	GetServiceWaypoint(kctx krt.HandlerContext, ctx context.Context, obj metav1.Object) *types.NamespacedName

	HasSynced() bool
}

func NewQueries(commonCols *collections.CommonCollections) WaypointQueries {
	waypointedServices, waypointByService := waypointAttachmentIndex(commonCols)
	return &waypointQueries{
		waypointedServices: waypointedServices,
		waypointByService:  waypointByService,
	}
}

type waypointQueries struct {
	waypointedServices krt.Collection[WaypointedService]
	waypointByService  krt.Index[string, WaypointedService]
}

func (w *waypointQueries) HasSynced() bool {
	return w.waypointedServices.HasSynced()
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

	waypointAttachmentsByService := krtutil.UnnamedIndex(waypointServiceAttachments, func(o WaypointedService) []string {
		return []string{o.Service.String()}
	})

	return waypointServiceAttachments, waypointAttachmentsByService
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
		Group:     new(gwv1.Group(objSrc.Group)),
		Kind:      new(gwv1.Kind(objSrc.Kind)),
		Name:      gwv1.ObjectName(objSrc.Name),
		Namespace: new(gwv1.Namespace(objSrc.Namespace)),
		Port:      new(gwv1.PortNumber(se.Spec.GetPorts()[0].GetNumber())), //nolint:gosec // G115: ServiceEntry port number is uint32, safe to convert to PortNumber (int32)
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
