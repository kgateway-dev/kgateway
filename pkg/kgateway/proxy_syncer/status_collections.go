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
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

// initStatusInfra registers existing raw KRT collections and report singletons as compact
// reconciliation sources. No per-object desired-status collection is created: writers build
// status just-in-time from the latest raw object and report fragment on the leader.
func (s *ProxySyncer) initStatusInfra(ctx context.Context, _ krtutil.KrtOptions) {
	s.statusCollections = statussync.NewStatusCollections()
	s.statusWriters = map[schema.GroupVersionKind]statussync.ResourceStatusSyncer{}

	cl := s.apiClient
	f := kclient.Filter{ObjectFilter: cl.ObjectFilter()}
	controllerName := s.controllerName

	gatewayReports := s.statusReport.AsCollection()
	backendReports := s.backendStatusReport.AsCollection()
	policyReports := s.policyReport.AsCollection()

	// Gateway
	statussync.RegisterResource(s.statusCollections, wellknown.GatewayGVK, s.commonCols.RawGateways)
	s.statusWriters[wellknown.GatewayGVK] = statussync.Writer[*gwv1.Gateway, gwv1.GatewayStatus]{
		Name:   "gateway",
		Client: kclient.NewFilteredDelayed[*gwv1.Gateway](cl, wellknown.GatewayGVR, f),
		Desired: func(gw *gwv1.Gateway) (gwv1.GatewayStatus, bool) {
			rm, ok := currentReports(gatewayReports)
			if !ok {
				return gwv1.GatewayStatus{}, false
			}
			status := rm.BuildGWStatus(ctx, *gw, nil)
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

	statussync.RegisterResource(s.statusCollections, wellknown.HTTPRouteGVK, s.commonCols.RawHTTPRoutes)
	statussync.RegisterResource(s.statusCollections, wellknown.GRPCRouteGVK, s.commonCols.RawGRPCRoutes)
	statussync.RegisterResource(s.statusCollections, wellknown.TCPRouteGVK, s.commonCols.RawTCPRoutes)
	statussync.RegisterResource(s.statusCollections, wellknown.TLSRouteGVK, s.commonCols.RawTLSRoutes)

	s.statusWriters[wellknown.HTTPRouteGVK] = routeWriter[*gwv1.HTTPRoute](ctx, cl, f, gatewayReports, "httpRoute", wellknown.HTTPRouteGVR, wellknown.HTTPRouteKind, controllerName,
		func(rm reports.ReportMap) map[types.NamespacedName]*reports.RouteReport { return rm.HTTPRoutes },
		func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.HTTPRoute {
			return &gwv1.HTTPRoute{ObjectMeta: om, Status: gwv1.HTTPRouteStatus{RouteStatus: st}}
		},
		func(o *gwv1.HTTPRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
		func(o *gwv1.HTTPRoute) []gwv1.ParentReference { return o.Spec.ParentRefs },
	)
	s.statusWriters[wellknown.GRPCRouteGVK] = routeWriter[*gwv1.GRPCRoute](ctx, cl, f, gatewayReports, "grpcRoute", wellknown.GRPCRouteGVR, wellknown.GRPCRouteKind, controllerName,
		func(rm reports.ReportMap) map[types.NamespacedName]*reports.RouteReport { return rm.GRPCRoutes },
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
		tcpWriter = routeWriter[*gwv1.TCPRoute](ctx, cl, f, gatewayReports, "tcpRoute", wellknown.TCPRouteV1GVR, wellknown.TCPRouteKind, controllerName,
			func(rm reports.ReportMap) map[types.NamespacedName]*reports.RouteReport { return rm.TCPRoutes },
			func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.TCPRoute {
				return &gwv1.TCPRoute{ObjectMeta: om, Status: gwv1.TCPRouteStatus{RouteStatus: st}}
			},
			func(o *gwv1.TCPRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
			func(o *gwv1.TCPRoute) []gwv1.ParentReference { return o.Spec.ParentRefs },
		)
	} else {
		tcpWriter = routeWriter[*gwv1a2.TCPRoute](ctx, cl, f, gatewayReports, "tcpRoute", wellknown.TCPRouteGVR, wellknown.TCPRouteKind, controllerName,
			func(rm reports.ReportMap) map[types.NamespacedName]*reports.RouteReport { return rm.TCPRoutes },
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
		tlsWriter = routeWriter[*gwv1.TLSRoute](ctx, cl, f, gatewayReports, "tlsRoute", wellknown.TLSRouteV1GVR, wellknown.TLSRouteKind, controllerName,
			func(rm reports.ReportMap) map[types.NamespacedName]*reports.RouteReport { return rm.TLSRoutes },
			func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.TLSRoute {
				return &gwv1.TLSRoute{ObjectMeta: om, Status: gwv1.TLSRouteStatus{RouteStatus: st}}
			},
			func(o *gwv1.TLSRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
			func(o *gwv1.TLSRoute) []gwv1.ParentReference { return o.Spec.ParentRefs },
		)
	case wellknown.TLSRouteV1Alpha3GVR:
		tlsWriter = routeWriter[*gwv1a3.TLSRoute](ctx, cl, f, gatewayReports, "tlsRoute", wellknown.TLSRouteV1Alpha3GVR, wellknown.TLSRouteKind, controllerName,
			func(rm reports.ReportMap) map[types.NamespacedName]*reports.RouteReport { return rm.TLSRoutes },
			func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1a3.TLSRoute {
				return &gwv1a3.TLSRoute{ObjectMeta: om, Status: gwv1.TLSRouteStatus{RouteStatus: st}}
			},
			func(o *gwv1a3.TLSRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
			func(o *gwv1a3.TLSRoute) []gwv1.ParentReference { return o.Spec.ParentRefs },
		)
	default:
		tlsWriter = routeWriter[*gwv1a2.TLSRoute](ctx, cl, f, gatewayReports, "tlsRoute", wellknown.TLSRouteGVR, wellknown.TLSRouteKind, controllerName,
			func(rm reports.ReportMap) map[types.NamespacedName]*reports.RouteReport { return rm.TLSRoutes },
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

	statussync.RegisterResource(s.statusCollections, wellknown.ListenerSetGVK, s.commonCols.RawListenerSets)
	lsWriter := &listenerSetStatusSyncer{
		col:      s.commonCols.RawListenerSets,
		promoted: kclient.NewFilteredDelayed[*gwv1.ListenerSet](cl, wellknown.ListenerSetGVR, f),
		client:   cl,
		reports:  gatewayReports,
		ctx:      ctx,
	}
	s.statusWriters[wellknown.ListenerSetGVK] = lsWriter
	s.statusWriters[wellknown.XListenerSetGVK] = lsWriter

	backendPlugin := s.plugins.ContributesBackends[wellknown.BackendGVK.GroupKind()]
	if backendPlugin.RawBackends != nil {
		statussync.RegisterResource(s.statusCollections, wellknown.BackendGVK, backendPlugin.RawBackends)
	}
	s.statusWriters[wellknown.BackendGVK] = statussync.Writer[*kgateway.Backend, kgateway.BackendStatus]{
		Name:   "backend",
		Client: kclient.NewFilteredDelayed[*kgateway.Backend](cl, wellknown.BackendGVR, f),
		Desired: func(be *kgateway.Backend) (kgateway.BackendStatus, bool) {
			rm, ok := currentReports(backendReports)
			if !ok {
				return kgateway.BackendStatus{}, false
			}
			status := rm.BuildBackendStatus(ctx, be, be.Status)
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

	policyGVKs := map[schema.GroupKind]schema.GroupVersionKind{}
	policyStatusInputs := plug.PolicyStatusInputs{
		Collections:   s.statusCollections,
		PolicyReports: policyReports,
		RegisterWriter: func(gvk schema.GroupVersionKind, syncer statussync.ResourceStatusSyncer) {
			s.statusWriters[gvk] = syncer
			policyGVKs[gvk.GroupKind()] = gvk
		},
	}
	for _, plugin := range s.plugins.ContributesPolicies {
		if plugin.RegisterPolicyStatus != nil {
			plugin.RegisterPolicyStatus(policyStatusInputs)
		}
	}

	statussync.RegisterReports(s.statusCollections, gatewayReports, func(old, current reports.ReportMap) []statussync.Resource {
		return changedGatewayResources(old, current, s.commonCols.TCPRouteWriteGVR, s.commonCols.TLSRouteWriteGVR)
	})
	statussync.RegisterReports(s.statusCollections, backendReports, changedBackendResources)
	statussync.RegisterReports(s.statusCollections, policyReports, func(old, current reports.ReportMap) []statussync.Resource {
		return changedPolicyResources(old, current, policyGVKs)
	})
	// RegisterReports skips its initial replay because the raw collections own the leader
	// sweep. Ensure all report singletons are populated before those raw handlers attach.
	s.waitForSync = append(s.waitForSync, gatewayReports.HasSynced, backendReports.HasSynced, policyReports.HasSynced)
}

func currentReports(col krt.Collection[statussync.ReportsWrapper]) (reports.ReportMap, bool) {
	wrapper := col.GetKey("report")
	if wrapper == nil {
		return reports.ReportMap{}, false
	}
	return wrapper.Reports(), true
}

func changedGatewayResources(
	old, current reports.ReportMap,
	tcpWriteGVR, tlsWriteGVR schema.GroupVersionResource,
) []statussync.Resource {
	resources := resourcesForNames(wellknown.GatewayGVK,
		changedKeys(old.Gateways, current.Gateways, reports.GatewayReportEqual))
	resources = append(resources, resourcesForNames(wellknown.HTTPRouteGVK,
		changedKeys(old.HTTPRoutes, current.HTTPRoutes, reports.RouteReportEqual))...)
	resources = append(resources, resourcesForNames(wellknown.GRPCRouteGVK,
		changedKeys(old.GRPCRoutes, current.GRPCRoutes, reports.RouteReportEqual))...)

	tcpGVK := wellknown.TCPRouteGVK
	if tcpWriteGVR == wellknown.TCPRouteV1GVR {
		tcpGVK = wellknown.TCPRouteV1GVK
	}
	resources = append(resources, resourcesForNames(tcpGVK,
		changedKeys(old.TCPRoutes, current.TCPRoutes, reports.RouteReportEqual))...)

	tlsGVK := wellknown.TLSRouteGVK
	switch tlsWriteGVR {
	case wellknown.TLSRouteV1GVR:
		tlsGVK = wellknown.TLSRouteV1GVK
	case wellknown.TLSRouteV1Alpha3GVR:
		tlsGVK = wellknown.TLSRouteV1Alpha3GVK
	}
	resources = append(resources, resourcesForNames(tlsGVK,
		changedKeys(old.TLSRoutes, current.TLSRoutes, reports.RouteReportEqual))...)

	listenerSetGVKs := map[schema.GroupVersionKind]struct{}{}
	for gvk := range old.ListenerSets {
		listenerSetGVKs[gvk] = struct{}{}
	}
	for gvk := range current.ListenerSets {
		listenerSetGVKs[gvk] = struct{}{}
	}
	for gvk := range listenerSetGVKs {
		resources = append(resources, resourcesForNames(gvk,
			changedKeys(old.ListenerSets[gvk], current.ListenerSets[gvk], reports.ListenerSetReportEqual))...)
	}
	return resources
}

func changedBackendResources(old, current reports.ReportMap) []statussync.Resource {
	return resourcesForNames(wellknown.BackendGVK,
		changedKeys(old.Backends, current.Backends, reports.BackendReportEqual))
}

func changedPolicyResources(
	old, current reports.ReportMap,
	gvks map[schema.GroupKind]schema.GroupVersionKind,
) []statussync.Resource {
	keys := changedKeys(old.Policies, current.Policies, reports.PolicyReportEqual)
	resources := make([]statussync.Resource, 0, len(keys))
	for _, key := range keys {
		gvk, ok := gvks[schema.GroupKind{Group: key.Group, Kind: key.Kind}]
		if !ok {
			continue
		}
		resources = append(resources, statussync.Resource{
			GroupVersionKind: gvk,
			NamespacedName:   types.NamespacedName{Namespace: key.Namespace, Name: key.Name},
		})
	}
	return resources
}

func changedKeys[K comparable, V any](old, current map[K]V, equal func(V, V) bool) []K {
	changed := make([]K, 0)
	for key, oldValue := range old {
		currentValue, found := current[key]
		if !found || !equal(oldValue, currentValue) {
			changed = append(changed, key)
		}
	}
	for key := range current {
		if _, found := old[key]; !found {
			changed = append(changed, key)
		}
	}
	return changed
}

func resourcesForNames(gvk schema.GroupVersionKind, names []types.NamespacedName) []statussync.Resource {
	resources := make([]statussync.Resource, 0, len(names))
	for _, nn := range names {
		resources = append(resources, statussync.Resource{GroupVersionKind: gvk, NamespacedName: nn})
	}
	return resources
}

// routeWriter constructs the status writer for one route kind, wiring the multi-controller
// parent merge and the per-parent status sync metrics.
func routeWriter[T controllers.ComparableObject](
	ctx context.Context,
	cl apiclient.Client,
	f kclient.Filter,
	reportCol krt.Collection[statussync.ReportsWrapper],
	name string,
	gvr schema.GroupVersionResource,
	kind string,
	controllerName string,
	reportsForKind func(reports.ReportMap) map[types.NamespacedName]*reports.RouteReport,
	build func(om metav1.ObjectMeta, st gwv1.RouteStatus) T,
	getStatus func(T) gwv1.RouteStatus,
	parentRefs func(T) []gwv1.ParentReference,
) statussync.Writer[T, gwv1.RouteStatus] {
	return statussync.Writer[T, gwv1.RouteStatus]{
		Name:   name,
		Client: kclient.NewFilteredDelayed[T](cl, gvr, f),
		Desired: func(current T) (gwv1.RouteStatus, bool) {
			rm, ok := currentReports(reportCol)
			if !ok {
				return gwv1.RouteStatus{}, false
			}
			nn := types.NamespacedName{Namespace: current.GetNamespace(), Name: current.GetName()}
			if reportsForKind(rm)[nn] == nil {
				// An empty desired status clears only this controller's stale parents in Merge.
				return gwv1.RouteStatus{}, true
			}
			status := rm.BuildRouteStatus(ctx, current, controllerName)
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
	reports  krt.Collection[statussync.ReportsWrapper]
	ctx      context.Context
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
		rm, ok := currentReports(s.reports)
		if !ok {
			return nil
		}
		lsCopy := *current
		lsCopy.SetGroupVersionKind(res.GroupVersionKind)
		status := rm.BuildListenerSetStatus(s.ctx, lsCopy)
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
