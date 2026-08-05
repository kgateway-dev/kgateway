package proxy_syncer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1a3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kgateway-dev/kgateway/v2/api/conditions"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	kmetrics "github.com/kgateway-dev/kgateway/v2/pkg/krtcollections/metrics"
	plug "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

// initStatusInfra reduces keyed translation contributions per status owner and
// registers both raw-object and reduced-report reconciliation sources. Desired
// Kubernetes status is still built just in time by the leader's writer.
func (s *ProxySyncer) initStatusInfra(ctx context.Context, krtopts krtutil.KrtOptions) {
	s.statusCollections = statussync.NewStatusCollections()
	s.statusWriters = map[schema.GroupVersionKind]statussync.ResourceStatusSyncer{}

	cl := s.apiClient
	f := kclient.Filter{ObjectFilter: cl.ObjectFilter()}
	controllerName := s.controllerName
	contributionsByTarget := krtpkg.UnnamedIndex(s.statusContributions, func(contribution reports.StatusContribution) []reports.StatusKey {
		return []reports.StatusKey{contribution.Target.Key()}
	})
	resourceFor := func(gvk schema.GroupVersionKind, object controllers.Object) statussync.Resource {
		return statussync.Resource{
			GroupVersionKind: gvk,
			NamespacedName:   types.NamespacedName{Namespace: object.GetNamespace(), Name: object.GetName()},
		}
	}
	resourceForObjectGVK := func(fallback schema.GroupVersionKind, object controllers.Object) statussync.Resource {
		gvk := object.GetObjectKind().GroupVersionKind()
		if gvk.Empty() {
			gvk = fallback
		}
		return resourceFor(gvk, object)
	}

	// Gateway
	gatewayReports := statussync.NewResourceReports(
		s.commonCols.RawGateways, s.statusContributions, contributionsByTarget,
		func(object *gwv1.Gateway) statussync.Resource { return resourceFor(wellknown.GatewayGVK, object) },
		krtopts.ToOptions("GatewayStatusReports")...,
	)
	statussync.RegisterResource(s.statusCollections, wellknown.GatewayGVK, s.commonCols.RawGateways)
	statussync.RegisterResourceReports(s.statusCollections, gatewayReports)
	s.statusWriters[wellknown.GatewayGVK] = statussync.Writer[*gwv1.Gateway, gwv1.GatewayStatus]{
		Name:   "gateway",
		Client: kclient.NewFilteredDelayed[*gwv1.Gateway](cl, wellknown.GatewayGVR, f),
		Desired: func(gw *gwv1.Gateway) (gwv1.GatewayStatus, bool) {
			report, ok := currentReport(gatewayReports, wellknown.GatewayGVK, types.NamespacedName{Namespace: gw.Namespace, Name: gw.Name})
			if !ok {
				return gwv1.GatewayStatus{}, false
			}
			status := reports.BuildGWStatus(ctx, report.Gateway, *gw, nil)
			if status == nil {
				return gwv1.GatewayStatus{}, false
			}
			return *status, true
		},
		Build: func(om metav1.ObjectMeta, st gwv1.GatewayStatus) *gwv1.Gateway {
			return &gwv1.Gateway{ObjectMeta: om, Status: st}
		},
		GetStatus: func(o *gwv1.Gateway) gwv1.GatewayStatus { return o.Status },
		Merge:     mergeGatewayStatusAddresses,
		OnSync:    gatewayStatusMetricsHook(),
	}

	httpRouteReports := statussync.NewResourceReports(
		s.commonCols.RawHTTPRoutes, s.statusContributions, contributionsByTarget,
		func(object *gwv1.HTTPRoute) statussync.Resource { return resourceFor(wellknown.HTTPRouteGVK, object) },
		krtopts.ToOptions("HTTPRouteStatusReports")...,
	)
	grpcRouteReports := statussync.NewResourceReports(
		s.commonCols.RawGRPCRoutes, s.statusContributions, contributionsByTarget,
		func(object *gwv1.GRPCRoute) statussync.Resource { return resourceFor(wellknown.GRPCRouteGVK, object) },
		krtopts.ToOptions("GRPCRouteStatusReports")...,
	)
	tcpGVK := wellknown.TCPRouteGVK
	if s.commonCols.TCPRouteWriteGVR == wellknown.TCPRouteV1GVR {
		tcpGVK = wellknown.TCPRouteV1GVK
	}
	tcpRouteReports := statussync.NewResourceReports(
		s.commonCols.RawTCPRoutes, s.statusContributions, contributionsByTarget,
		func(object *gwv1a2.TCPRoute) statussync.Resource { return resourceFor(tcpGVK, object) },
		krtopts.ToOptions("TCPRouteStatusReports")...,
	)
	tlsGVK := wellknown.TLSRouteGVK
	switch s.commonCols.TLSRouteWriteGVR {
	case wellknown.TLSRouteV1GVR:
		tlsGVK = wellknown.TLSRouteV1GVK
	case wellknown.TLSRouteV1Alpha3GVR:
		tlsGVK = wellknown.TLSRouteV1Alpha3GVK
	}
	tlsRouteReports := statussync.NewResourceReports(
		s.commonCols.RawTLSRoutes, s.statusContributions, contributionsByTarget,
		func(object *gwv1a2.TLSRoute) statussync.Resource { return resourceFor(tlsGVK, object) },
		krtopts.ToOptions("TLSRouteStatusReports")...,
	)
	statussync.RegisterResource(s.statusCollections, wellknown.HTTPRouteGVK, s.commonCols.RawHTTPRoutes)
	statussync.RegisterResource(s.statusCollections, wellknown.GRPCRouteGVK, s.commonCols.RawGRPCRoutes)
	statussync.RegisterResource(s.statusCollections, tcpGVK, s.commonCols.RawTCPRoutes)
	statussync.RegisterResource(s.statusCollections, tlsGVK, s.commonCols.RawTLSRoutes)
	statussync.RegisterResourceReports(s.statusCollections, httpRouteReports)
	statussync.RegisterResourceReports(s.statusCollections, grpcRouteReports)
	statussync.RegisterResourceReports(s.statusCollections, tcpRouteReports)
	statussync.RegisterResourceReports(s.statusCollections, tlsRouteReports)

	s.statusWriters[wellknown.HTTPRouteGVK] = routeWriter[*gwv1.HTTPRoute](ctx, cl, f, httpRouteReports, wellknown.HTTPRouteGVK, "httpRoute", wellknown.HTTPRouteGVR, wellknown.HTTPRouteKind, controllerName,
		func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.HTTPRoute {
			return &gwv1.HTTPRoute{ObjectMeta: om, Status: gwv1.HTTPRouteStatus{RouteStatus: st}}
		},
		func(o *gwv1.HTTPRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
		func(o *gwv1.HTTPRoute) []gwv1.ParentReference { return o.Spec.ParentRefs },
	)
	s.statusWriters[wellknown.GRPCRouteGVK] = routeWriter[*gwv1.GRPCRoute](ctx, cl, f, grpcRouteReports, wellknown.GRPCRouteGVK, "grpcRoute", wellknown.GRPCRouteGVR, wellknown.GRPCRouteKind, controllerName,
		func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.GRPCRoute {
			return &gwv1.GRPCRoute{ObjectMeta: om, Status: gwv1.GRPCRouteStatus{RouteStatus: st}}
		},
		func(o *gwv1.GRPCRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
		func(o *gwv1.GRPCRoute) []gwv1.ParentReference { return o.Spec.ParentRefs },
	)

	// TCP and TLS route statuses are written through whichever served API version was
	// resolved at startup; all versions share the same storage object.
	var tcpWriter statussync.ResourceStatusSyncer
	if s.commonCols.TCPRouteWriteGVR == wellknown.TCPRouteV1GVR {
		tcpWriter = routeWriter[*gwv1.TCPRoute](ctx, cl, f, tcpRouteReports, tcpGVK, "tcpRoute", wellknown.TCPRouteV1GVR, wellknown.TCPRouteKind, controllerName,
			func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.TCPRoute {
				return &gwv1.TCPRoute{ObjectMeta: om, Status: gwv1.TCPRouteStatus{RouteStatus: st}}
			},
			func(o *gwv1.TCPRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
			func(o *gwv1.TCPRoute) []gwv1.ParentReference { return o.Spec.ParentRefs },
		)
	} else {
		tcpWriter = routeWriter[*gwv1a2.TCPRoute](ctx, cl, f, tcpRouteReports, tcpGVK, "tcpRoute", wellknown.TCPRouteGVR, wellknown.TCPRouteKind, controllerName,
			func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1a2.TCPRoute {
				return &gwv1a2.TCPRoute{ObjectMeta: om, Status: gwv1a2.TCPRouteStatus{RouteStatus: st}}
			},
			func(o *gwv1a2.TCPRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
			func(o *gwv1a2.TCPRoute) []gwv1.ParentReference { return o.Spec.ParentRefs },
		)
	}
	s.statusWriters[wellknown.TCPRouteGVK] = tcpWriter
	s.statusWriters[wellknown.TCPRouteV1GVK] = tcpWriter

	var tlsWriter statussync.ResourceStatusSyncer
	switch s.commonCols.TLSRouteWriteGVR {
	case wellknown.TLSRouteV1GVR:
		tlsWriter = routeWriter[*gwv1.TLSRoute](ctx, cl, f, tlsRouteReports, tlsGVK, "tlsRoute", wellknown.TLSRouteV1GVR, wellknown.TLSRouteKind, controllerName,
			func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.TLSRoute {
				return &gwv1.TLSRoute{ObjectMeta: om, Status: gwv1.TLSRouteStatus{RouteStatus: st}}
			},
			func(o *gwv1.TLSRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
			func(o *gwv1.TLSRoute) []gwv1.ParentReference { return o.Spec.ParentRefs },
		)
	case wellknown.TLSRouteV1Alpha3GVR:
		tlsWriter = routeWriter[*gwv1a3.TLSRoute](ctx, cl, f, tlsRouteReports, tlsGVK, "tlsRoute", wellknown.TLSRouteV1Alpha3GVR, wellknown.TLSRouteKind, controllerName,
			func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1a3.TLSRoute {
				return &gwv1a3.TLSRoute{ObjectMeta: om, Status: gwv1.TLSRouteStatus{RouteStatus: st}}
			},
			func(o *gwv1a3.TLSRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
			func(o *gwv1a3.TLSRoute) []gwv1.ParentReference { return o.Spec.ParentRefs },
		)
	default:
		tlsWriter = routeWriter[*gwv1a2.TLSRoute](ctx, cl, f, tlsRouteReports, tlsGVK, "tlsRoute", wellknown.TLSRouteGVR, wellknown.TLSRouteKind, controllerName,
			func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1a2.TLSRoute {
				return &gwv1a2.TLSRoute{ObjectMeta: om, Status: gwv1a2.TLSRouteStatus{RouteStatus: st}}
			},
			func(o *gwv1a2.TLSRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
			func(o *gwv1a2.TLSRoute) []gwv1.ParentReference { return o.Spec.ParentRefs },
		)
	}
	s.statusWriters[wellknown.TLSRouteGVK] = tlsWriter
	s.statusWriters[wellknown.TLSRouteV1GVK] = tlsWriter
	s.statusWriters[wellknown.TLSRouteV1Alpha3GVK] = tlsWriter

	listenerSetReports := statussync.NewResourceReports(
		s.commonCols.RawListenerSets, s.statusContributions, contributionsByTarget,
		func(object *gwv1.ListenerSet) statussync.Resource {
			return resourceForObjectGVK(wellknown.ListenerSetGVK, object)
		},
		krtopts.ToOptions("ListenerSetStatusReports")...,
	)
	statussync.RegisterResourceByObjectGVK(s.statusCollections, wellknown.ListenerSetGVK, s.commonCols.RawListenerSets)
	statussync.RegisterResourceReports(s.statusCollections, listenerSetReports)
	lsWriter := &listenerSetStatusSyncer{
		col:      s.commonCols.RawListenerSets,
		promoted: kclient.NewFilteredDelayed[*gwv1.ListenerSet](cl, wellknown.ListenerSetGVR, f),
		client:   cl,
		reports:  listenerSetReports,
	}
	s.statusWriters[wellknown.ListenerSetGVK] = lsWriter
	s.statusWriters[wellknown.XListenerSetGVK] = lsWriter

	backendPlugin := s.plugins.ContributesBackends[wellknown.BackendGVK.GroupKind()]
	var backendReports krt.Collection[statussync.ResourceReports]
	if backendPlugin.RawBackends != nil {
		backendReports = statussync.NewResourceReports(
			backendPlugin.RawBackends, s.statusContributions, contributionsByTarget,
			func(object *kgateway.Backend) statussync.Resource { return resourceFor(wellknown.BackendGVK, object) },
			krtopts.ToOptions("BackendStatusReports")...,
		)
		statussync.RegisterResource(s.statusCollections, wellknown.BackendGVK, backendPlugin.RawBackends)
		statussync.RegisterResourceReports(s.statusCollections, backendReports)
	} else {
		logger.Error("backend plugin is missing RawBackends; Backend status reconciliation is disabled",
			"group_kind", wellknown.BackendGVK.GroupKind().String())
	}
	s.statusWriters[wellknown.BackendGVK] = statussync.Writer[*kgateway.Backend, kgateway.BackendStatus]{
		Name:   "backend",
		Client: kclient.NewFilteredDelayed[*kgateway.Backend](cl, wellknown.BackendGVR, f),
		Desired: func(be *kgateway.Backend) (kgateway.BackendStatus, bool) {
			report, ok := currentReport(backendReports, wellknown.BackendGVK, types.NamespacedName{Namespace: be.Namespace, Name: be.Name})
			if !ok {
				return kgateway.BackendStatus{}, false
			}
			status := reports.BuildBackendStatus(ctx, report.Backend, be.Status)
			if status == nil {
				return kgateway.BackendStatus{}, false
			}
			return *status, true
		},
		Build: func(om metav1.ObjectMeta, st kgateway.BackendStatus) *kgateway.Backend {
			return &kgateway.Backend{ObjectMeta: om, Status: st}
		},
		GetStatus: func(o *kgateway.Backend) kgateway.BackendStatus { return o.Status },
		OnSync:    simpleStatusMetricsHook[*kgateway.Backend, kgateway.BackendStatus]("BackendStatusSyncer", wellknown.BackendGVK.Kind),
	}

	policyStatusInputs := plug.PolicyStatusInputs{
		Collections:           s.statusCollections,
		StatusContributions:   s.statusContributions,
		ContributionsByTarget: contributionsByTarget,
		KrtOpts:               krtopts,
		RegisterResourceReports: func(reports krt.Collection[statussync.ResourceReports]) {
			statussync.RegisterResourceReports(s.statusCollections, reports)
			s.waitForSync = append(s.waitForSync, reports.HasSynced)
		},
		RegisterWriter: func(gvk schema.GroupVersionKind, syncer statussync.ResourceStatusSyncer) {
			s.statusWriters[gvk] = syncer
		},
	}
	for _, plugin := range s.plugins.ContributesPolicies {
		if plugin.RegisterPolicyStatus != nil {
			plugin.RegisterPolicyStatus(policyStatusInputs)
		}
	}

	s.waitForSync = append(s.waitForSync,
		gatewayReports.HasSynced,
		httpRouteReports.HasSynced,
		grpcRouteReports.HasSynced,
		tcpRouteReports.HasSynced,
		tlsRouteReports.HasSynced,
		listenerSetReports.HasSynced,
	)
	if backendReports != nil {
		s.waitForSync = append(s.waitForSync, backendReports.HasSynced)
	}
}

func currentReport(
	col krt.Collection[statussync.ResourceReports],
	gvk schema.GroupVersionKind,
	nn types.NamespacedName,
) (reports.StatusReport, bool) {
	if col == nil {
		return reports.StatusReport{}, false
	}
	target := reports.StatusKey{GroupKind: gvk.GroupKind(), NamespacedName: nn}
	current := col.GetKey(target.String())
	if current == nil {
		return reports.StatusReport{}, false
	}
	// StatusReport contains pointers into KRT-owned retained state. Builders must treat the
	// returned report as read-only so later KRT equality checks continue to observe changes.
	return current.Report, true
}

// routeWriter constructs the status writer for one route kind, wiring the multi-controller
// parent merge and the per-parent status sync metrics.
func routeWriter[T controllers.ComparableObject](
	ctx context.Context,
	cl apiclient.Client,
	f kclient.Filter,
	reportCol krt.Collection[statussync.ResourceReports],
	gvk schema.GroupVersionKind,
	name string,
	gvr schema.GroupVersionResource,
	kind string,
	controllerName string,
	build func(om metav1.ObjectMeta, st gwv1.RouteStatus) T,
	getStatus func(T) gwv1.RouteStatus,
	parentRefs func(T) []gwv1.ParentReference,
) statussync.Writer[T, gwv1.RouteStatus] {
	return statussync.Writer[T, gwv1.RouteStatus]{
		Name:   name,
		Client: kclient.NewFilteredDelayed[T](cl, gvr, f),
		Desired: func(current T) (gwv1.RouteStatus, bool) {
			nn := types.NamespacedName{Namespace: current.GetNamespace(), Name: current.GetName()}
			report, ok := currentReport(reportCol, gvk, nn)
			if !ok {
				return gwv1.RouteStatus{}, false
			}
			if report.Route == nil {
				// An empty desired status clears only this controller's stale parents in Merge.
				return gwv1.RouteStatus{}, true
			}
			status := reports.BuildRouteStatus(ctx, report.Route, current, controllerName)
			if status == nil {
				return gwv1.RouteStatus{}, true
			}
			return *status, true
		},
		Build:     build,
		GetStatus: getStatus,
		Merge: func(current T, desired gwv1.RouteStatus) gwv1.RouteStatus {
			desired.Parents = statussync.MergeRouteParentStatuses(controllerName, getStatus(current).Parents, desired.Parents)
			return desired
		},
		OnSync: routeStatusMetricsHook(kind, controllerName, parentRefs),
	}
}

// mergeGatewayStatusAddresses carries the live Gateway status addresses into the status we
// are about to write, verbatim and in their existing order.
//
// status.addresses is owned by the deployer (it derives them from the generated Service),
// not by translation. Two properties matter here:
//
//   - We must take them from current, not from desired, so a deployer address update that
//     races report rendering is never reverted.
//   - We must not reorder them. The deployer decides whether to write with an order-sensitive
//     slices.Equal against the live list (see updateGatewayAddresses), and it builds the list
//     in source order: LoadBalancer ingress order, then Service ClusterIPs order, then
//     spec.addresses order. Any normalization we apply here (e.g. sorting) makes that
//     comparison fail forever, so the deployer rewrites its order, we rewrite ours, and
//     status.addresses flip-flops with two redundant writes on every deployer reconcile.
func mergeGatewayStatusAddresses(current *gwv1.Gateway, desired gwv1.GatewayStatus) gwv1.GatewayStatus {
	desired.Addresses = current.Status.Addresses
	return desired
}

// gatewayStatusMetricsHook records status sync metrics for Gateways, deriving an error
// result from invalid Accepted/Programmed condition reasons like the previous syncer did.
func gatewayStatusMetricsHook() func(res statussync.Resource, current *gwv1.Gateway, status gwv1.GatewayStatus, took time.Duration, err error) {
	return func(res statussync.Resource, current *gwv1.Gateway, status gwv1.GatewayStatus, took time.Duration, err error) {
		statusErr := err
		for _, cond := range status.Conditions {
			if cond.Type != string(gwv1.GatewayConditionAccepted) &&
				cond.Type != string(gwv1.GatewayConditionProgrammed) {
				continue
			}
			if cond.Reason != string(gwv1.GatewayReasonAccepted) &&
				cond.Reason != string(gwv1.GatewayReasonProgrammed) &&
				cond.Reason != string(gwv1.GatewayReasonPending) {
				statusErr = errors.Join(statusErr, fmt.Errorf("invalid gateway condition"))
				break
			}
		}
		statussync.RecordStatusSync(statussync.SyncMetricLabels{
			Name:      res.Name,
			Namespace: res.Namespace,
			Syncer:    "GatewayStatusSyncer",
		}, took, statusErr)
		kmetrics.EndResourceStatusSync(kmetrics.ResourceSyncDetails{
			Namespace:    res.Namespace,
			Gateway:      res.Name,
			ResourceType: wellknown.GatewayKind,
			ResourceName: res.Name,
		})
	}
}

// routeStatusMetricsHook records per-parent-gateway status sync metrics for routes,
// deriving an error result from invalid route conditions like the previous syncer did.
func routeStatusMetricsHook[T controllers.ComparableObject](
	kind string,
	controllerName string,
	parentRefs func(T) []gwv1.ParentReference,
) func(res statussync.Resource, current T, status gwv1.RouteStatus, took time.Duration, err error) {
	return func(res statussync.Resource, current T, status gwv1.RouteStatus, took time.Duration, err error) {
		statusErrByGateway := map[string]error{}
		for _, ps := range status.Parents {
			// status is the merged status, so it also carries parents owned by other
			// controllers. Their conditions are not ours to report on.
			if string(ps.ControllerName) != controllerName {
				continue
			}
			gwName := string(ps.ParentRef.Name)
			for _, cond := range ps.Conditions {
				switch {
				case cond.Type == string(gwv1.RouteConditionPartiallyInvalid) && cond.Status == metav1.ConditionTrue:
					statusErrByGateway[gwName] = fmt.Errorf("partially invalid route condition")
				case cond.Type == conditions.KgatewayConditionProgrammed && cond.Status != metav1.ConditionTrue:
					statusErrByGateway[gwName] = fmt.Errorf("invalid route condition")
				case cond.Type == string(gwv1.RouteConditionAccepted) &&
					cond.Reason != string(gwv1.RouteReasonAccepted) &&
					cond.Reason != string(gwv1.RouteReasonPending):
					statusErrByGateway[gwName] = fmt.Errorf("invalid route condition")
				}
			}
		}

		gatewayNames := []string{}
		if !controllers.IsNil(current) {
			for _, pr := range parentRefs(current) {
				gatewayNames = append(gatewayNames, string(pr.Name))
			}
		}
		for _, gwName := range gatewayNames {
			statussync.RecordStatusSync(statussync.SyncMetricLabels{
				Name:      gwName,
				Namespace: res.Namespace,
				Syncer:    "RouteStatusSyncer",
			}, took, errors.Join(err, statusErrByGateway[gwName]))
			kmetrics.EndResourceStatusSync(kmetrics.ResourceSyncDetails{
				Namespace:    res.Namespace,
				Gateway:      gwName,
				ResourceType: kind,
				ResourceName: res.Name,
			})
		}
	}
}

// simpleStatusMetricsHook records status sync metrics keyed by the resource itself
// (used for kinds that are not parented by a Gateway).
func simpleStatusMetricsHook[T controllers.ComparableObject, S any](syncer, kind string) func(res statussync.Resource, current T, status S, took time.Duration, err error) {
	return func(res statussync.Resource, current T, status S, took time.Duration, err error) {
		statussync.RecordStatusSync(statussync.SyncMetricLabels{
			Name:      res.Name,
			Namespace: res.Namespace,
			Syncer:    syncer,
		}, took, err)
		kmetrics.EndResourceStatusSync(kmetrics.ResourceSyncDetails{
			Namespace:    res.Namespace,
			Gateway:      "",
			ResourceType: kind,
			ResourceName: res.Name,
		})
	}
}

// listenerSetStatusSyncer writes ListenerSet statuses. Promoted ListenerSets are written
// through the typed client; legacy XListenerSets are written through the dynamic client
// with the required per-listener port injected into the status payload.
type listenerSetStatusSyncer struct {
	col      krt.Collection[*gwv1.ListenerSet]
	promoted kclient.Client[*gwv1.ListenerSet]
	client   apiclient.Client
	reports  krt.Collection[statussync.ResourceReports]
}

func (s *listenerSetStatusSyncer) ApplyStatus(ctx context.Context, res statussync.Resource) {
	start := time.Now()

	var current *gwv1.ListenerSet
	var desired gwv1.ListenerSetStatus
	hasDesired := false
	// Retry transient write failures: after a failed write nothing changes on the
	// informer, so no event is guaranteed to re-enqueue this resource. Each attempt
	// re-reads the current object; conflicts and NotFound self-heal via re-enqueue.
	err := statussync.RetryStatusWrite(ctx, func() error {
		hasDesired = false
		cur := s.col.GetKey(res.Namespace + "/" + res.Name)
		if cur == nil || *cur == nil {
			logger.Debug("listener set not found, skipping status update", "resource", res.NamespacedName.String())
			return nil
		}
		current = *cur
		report, ok := currentReport(s.reports, res.GroupVersionKind, res.NamespacedName)
		if !ok {
			return nil
		}
		lsCopy := *current
		lsCopy.SetGroupVersionKind(res.GroupVersionKind)
		status := reports.BuildListenerSetStatus(ctx, report.ListenerSet, lsCopy)
		if status == nil {
			return nil
		}
		desired = *status
		hasDesired = true

		if krt.Equal(current.Status, desired) {
			return nil
		}

		if res.GroupVersionKind == wellknown.XListenerSetGVK {
			return s.patchLegacyStatus(ctx, res, current, desired)
		}

		_, err := s.promoted.UpdateStatus(&gwv1.ListenerSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:            res.Name,
				Namespace:       res.Namespace,
				ResourceVersion: current.ResourceVersion,
			},
			Status: desired,
		})
		if err != nil {
			if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
				// The raw collection re-enqueues once the informer delivers the newer object.
				logger.Debug("skipping stale listener set status update", "resource", res.NamespacedName.String(), "error", err)
				return nil
			}
			return err
		}
		return nil
	})
	if err != nil {
		logger.Error("error updating listener set status", "resource", res.NamespacedName.String(), "error", err)
	}
	if !hasDesired {
		return
	}

	statusErr := err
	for _, cond := range desired.Conditions {
		if cond.Type != string(gwv1.ListenerSetConditionAccepted) &&
			cond.Type != string(gwv1.ListenerSetConditionProgrammed) {
			continue
		}
		if cond.Reason != string(gwv1.ListenerSetReasonAccepted) &&
			cond.Reason != string(gwv1.ListenerSetReasonProgrammed) &&
			cond.Reason != string(gwv1.ListenerSetReasonPending) {
			statusErr = errors.Join(statusErr, fmt.Errorf("invalid listener condition"))
			break
		}
	}
	parentName := ""
	if current != nil {
		parentName = string(current.Spec.ParentRef.Name)
	}
	statussync.RecordStatusSync(statussync.SyncMetricLabels{
		Name:      parentName,
		Namespace: res.Namespace,
		Syncer:    "ListenerSetStatusSyncer",
	}, time.Since(start), statusErr)
	kmetrics.EndResourceStatusSync(kmetrics.ResourceSyncDetails{
		Namespace: res.Namespace,
		Gateway:   parentName,
		// TODO: Rename the legacy "XListenerSet" metrics label to "ListenerSet" in a
		// follow-up cleanup so dashboards, tests, and emitters can be updated together.
		ResourceType: "XListenerSet",
		ResourceName: res.Name,
	})
}

// patchLegacyStatus merge-patches the status subresource of a legacy XListenerSet through
// the dynamic client, injecting the per-listener port required by the legacy CRD schema.
func (s *listenerSetStatusSyncer) patchLegacyStatus(ctx context.Context, res statussync.Resource, current *gwv1.ListenerSet, desired gwv1.ListenerSetStatus) error {
	statusMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&desired)
	if err != nil {
		return err
	}
	injectListenerPorts(statusMap, current.Spec.Listeners)
	data, err := json.Marshal(map[string]any{"status": statusMap})
	if err != nil {
		return err
	}
	_, err = s.client.Dynamic().Resource(wellknown.XListenerSetGVR).Namespace(res.Namespace).
		Patch(ctx, res.Name, types.MergePatchType, data, metav1.PatchOptions{}, "status")
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// legacyPortFallback is used when a listener protocol requires an explicit port
// but none is set, matching the v2.2.4 fallback behaviour. 65535 is an out-of-
// range sentinel that satisfies the schema's required field without silently
// routing traffic to a real port.
const legacyPortFallback int64 = 65535

// injectListenerPorts adds the "port" field to each entry in statusMap["listeners"]
// by looking up the matching listener in specListeners by name.
// This is needed because gwv1.ListenerEntryStatus no longer carries Port, but the
// legacy XListenerSet CRD schema still requires it.
// Listeners whose name does not match any spec entry receive legacyPortFallback
// so that the patch payload always satisfies the schema's required constraint.
func injectListenerPorts(statusMap map[string]any, specListeners []gwv1.ListenerEntry) {
	listeners, ok := statusMap["listeners"].([]any)
	if !ok {
		return
	}

	// Precompute name→port to avoid O(n²) scan.
	portByName := make(map[string]int64, len(specListeners))
	for _, spec := range specListeners {
		port, err := kubeutils.DetectListenerPortNumber(spec.Protocol, spec.Port)
		if err != nil {
			port = gwv1.PortNumber(legacyPortFallback)
		}
		portByName[string(spec.Name)] = int64(port)
	}

	for i, entry := range listeners {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		name, _ := entryMap["name"].(string)
		port, matched := portByName[name]
		if !matched {
			// No corresponding spec entry; use the fallback so the patch
			// payload still satisfies the schema's required port constraint.
			port = legacyPortFallback
		}
		entryMap["port"] = port
		listeners[i] = entryMap
	}
}
