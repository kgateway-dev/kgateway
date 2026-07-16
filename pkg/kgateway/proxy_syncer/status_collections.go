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

// initStatusInfra builds the per-object desired-status collections and the per-GVK status
// writers. Desired statuses are derived from the merged report singletons using the same
// builders as before; writes go through the istio kclient (the same informer cache that
// translation reads from), eliminating the second (controller-runtime) cache from the
// status path.
func (s *ProxySyncer) initStatusInfra(ctx context.Context, krtopts krtutil.KrtOptions) {
	s.statusCollections = statussync.NewStatusCollections()
	s.statusWriters = map[schema.GroupVersionKind]statussync.ResourceStatusSyncer{}

	cl := s.apiClient
	f := kclient.Filter{ObjectFilter: cl.ObjectFilter()}
	controllerName := s.controllerName

	gatewayReports := s.statusReport.AsCollection()
	backendReports := s.backendStatusReport.AsCollection()

	// Gateway
	gatewayStatuses := krt.NewCollection(s.commonCols.RawGateways, func(kctx krt.HandlerContext, gw *gwv1.Gateway) *krt.ObjectWithStatus[*gwv1.Gateway, gwv1.GatewayStatus] {
		rw := krt.FetchOne(kctx, gatewayReports)
		if rw == nil {
			return nil
		}
		rm := rw.Reports()
		status := rm.BuildGWStatus(ctx, *gw, nil)
		if status == nil {
			// Not in the report: not translated by us (e.g. another controller's Gateway).
			return nil
		}
		// Addresses are written by the deployer, not by translation; carry the live value
		// (normalized through the same merge the writer applies) so the live-vs-desired
		// comparison stays quiet. The writer additionally merges addresses at write time.
		status.Addresses = statussync.MergeGatewayAddresses(gw.Status.Addresses, nil)
		return &krt.ObjectWithStatus[*gwv1.Gateway, gwv1.GatewayStatus]{Obj: gw, Status: *status}
	}, krtopts.ToOptions("GatewayStatuses")...)
	statussync.RegisterStatus(s.statusCollections, wellknown.GatewayGVK, gatewayStatuses, func(o *gwv1.Gateway) gwv1.GatewayStatus {
		return o.Status
	})
	s.statusWriters[wellknown.GatewayGVK] = statussync.Writer[*gwv1.Gateway, gwv1.GatewayStatus]{
		Name:   "gateway",
		Client: kclient.NewFilteredDelayed[*gwv1.Gateway](cl, wellknown.GatewayGVR, f),
		Build: func(om metav1.ObjectMeta, st gwv1.GatewayStatus) *gwv1.Gateway {
			return &gwv1.Gateway{ObjectMeta: om, Status: st}
		},
		GetStatus: func(o *gwv1.Gateway) gwv1.GatewayStatus { return o.Status },
		Merge: func(current *gwv1.Gateway, desired gwv1.GatewayStatus) gwv1.GatewayStatus {
			desired.Addresses = statussync.MergeGatewayAddresses(current.Status.Addresses, desired.Addresses)
			return desired
		},
		OnSync: gatewayStatusMetricsHook(),
	}

	// Routes. Desired statuses carry the full RouteStatus (including preserved entries from
	// other controllers, merged again at write time from the freshest informer state).
	registerRouteStatus(s, ctx, krtopts, wellknown.HTTPRouteGVK, s.commonCols.RawHTTPRoutes,
		func(rm reports.ReportMap, nn types.NamespacedName) bool { return rm.HTTPRoutes[nn] != nil })
	registerRouteStatus(s, ctx, krtopts, wellknown.GRPCRouteGVK, s.commonCols.RawGRPCRoutes,
		func(rm reports.ReportMap, nn types.NamespacedName) bool { return rm.GRPCRoutes[nn] != nil })
	registerRouteStatus(s, ctx, krtopts, wellknown.TCPRouteGVK, s.commonCols.RawTCPRoutes,
		func(rm reports.ReportMap, nn types.NamespacedName) bool { return rm.TCPRoutes[nn] != nil })
	registerRouteStatus(s, ctx, krtopts, wellknown.TLSRouteGVK, s.commonCols.RawTLSRoutes,
		func(rm reports.ReportMap, nn types.NamespacedName) bool { return rm.TLSRoutes[nn] != nil })

	s.statusWriters[wellknown.HTTPRouteGVK] = routeWriter[*gwv1.HTTPRoute](cl, f, "httpRoute", wellknown.HTTPRouteGVR, wellknown.HTTPRouteKind, controllerName,
		func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.HTTPRoute {
			return &gwv1.HTTPRoute{ObjectMeta: om, Status: gwv1.HTTPRouteStatus{RouteStatus: st}}
		},
		func(o *gwv1.HTTPRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
		func(o *gwv1.HTTPRoute) []gwv1.ParentReference { return o.Spec.ParentRefs },
	)
	s.statusWriters[wellknown.GRPCRouteGVK] = routeWriter[*gwv1.GRPCRoute](cl, f, "grpcRoute", wellknown.GRPCRouteGVR, wellknown.GRPCRouteKind, controllerName,
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
		tcpWriter = routeWriter[*gwv1.TCPRoute](cl, f, "tcpRoute", wellknown.TCPRouteV1GVR, wellknown.TCPRouteKind, controllerName,
			func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.TCPRoute {
				return &gwv1.TCPRoute{ObjectMeta: om, Status: gwv1.TCPRouteStatus{RouteStatus: st}}
			},
			func(o *gwv1.TCPRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
			func(o *gwv1.TCPRoute) []gwv1.ParentReference { return o.Spec.ParentRefs },
		)
	} else {
		tcpWriter = routeWriter[*gwv1a2.TCPRoute](cl, f, "tcpRoute", wellknown.TCPRouteGVR, wellknown.TCPRouteKind, controllerName,
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
		tlsWriter = routeWriter[*gwv1.TLSRoute](cl, f, "tlsRoute", wellknown.TLSRouteV1GVR, wellknown.TLSRouteKind, controllerName,
			func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.TLSRoute {
				return &gwv1.TLSRoute{ObjectMeta: om, Status: gwv1.TLSRouteStatus{RouteStatus: st}}
			},
			func(o *gwv1.TLSRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
			func(o *gwv1.TLSRoute) []gwv1.ParentReference { return o.Spec.ParentRefs },
		)
	case wellknown.TLSRouteV1Alpha3GVR:
		tlsWriter = routeWriter[*gwv1a3.TLSRoute](cl, f, "tlsRoute", wellknown.TLSRouteV1Alpha3GVR, wellknown.TLSRouteKind, controllerName,
			func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1a3.TLSRoute {
				return &gwv1a3.TLSRoute{ObjectMeta: om, Status: gwv1.TLSRouteStatus{RouteStatus: st}}
			},
			func(o *gwv1a3.TLSRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
			func(o *gwv1a3.TLSRoute) []gwv1.ParentReference { return o.Spec.ParentRefs },
		)
	default:
		tlsWriter = routeWriter[*gwv1a2.TLSRoute](cl, f, "tlsRoute", wellknown.TLSRouteGVR, wellknown.TLSRouteKind, controllerName,
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

	// ListenerSet (promoted and legacy XListenerSet variants share one collection; the
	// legacy variant is distinguished by its preserved GroupVersionKind).
	listenerSetStatuses := krt.NewCollection(s.commonCols.RawListenerSets, func(kctx krt.HandlerContext, ls *gwv1.ListenerSet) *krt.ObjectWithStatus[*gwv1.ListenerSet, gwv1.ListenerSetStatus] {
		rw := krt.FetchOne(kctx, gatewayReports)
		if rw == nil {
			return nil
		}
		rm := rw.Reports()
		lsCopy := *ls
		if lsCopy.GroupVersionKind().Empty() {
			lsCopy.SetGroupVersionKind(wellknown.ListenerSetGVK)
		}
		status := rm.BuildListenerSetStatus(ctx, lsCopy)
		if status == nil {
			return nil
		}
		return &krt.ObjectWithStatus[*gwv1.ListenerSet, gwv1.ListenerSetStatus]{Obj: ls, Status: *status}
	}, krtopts.ToOptions("ListenerSetStatuses")...)
	statussync.RegisterStatus(s.statusCollections, wellknown.ListenerSetGVK, listenerSetStatuses, func(o *gwv1.ListenerSet) gwv1.ListenerSetStatus {
		return o.Status
	})
	lsWriter := &listenerSetStatusSyncer{
		col:      s.commonCols.RawListenerSets,
		promoted: kclient.NewFilteredDelayed[*gwv1.ListenerSet](cl, wellknown.ListenerSetGVR, f),
		client:   cl,
	}
	s.statusWriters[wellknown.ListenerSetGVK] = lsWriter
	s.statusWriters[wellknown.XListenerSetGVK] = lsWriter

	// Backend
	rawBackends := krt.WrapClient(kclient.NewFilteredDelayed[*kgateway.Backend](cl, wellknown.BackendGVR, f), krtopts.ToOptions("RawBackendsForStatus")...)
	backendStatuses := krt.NewCollection(rawBackends, func(kctx krt.HandlerContext, be *kgateway.Backend) *krt.ObjectWithStatus[*kgateway.Backend, kgateway.BackendStatus] {
		rw := krt.FetchOne(kctx, backendReports)
		if rw == nil {
			return nil
		}
		rm := rw.Reports()
		status := rm.BuildBackendStatus(ctx, be, be.Status)
		if status == nil {
			return nil
		}
		return &krt.ObjectWithStatus[*kgateway.Backend, kgateway.BackendStatus]{Obj: be, Status: *status}
	}, krtopts.ToOptions("BackendStatuses")...)
	statussync.RegisterStatus(s.statusCollections, wellknown.BackendGVK, backendStatuses, func(o *kgateway.Backend) kgateway.BackendStatus {
		return o.Status
	})
	s.statusWriters[wellknown.BackendGVK] = statussync.Writer[*kgateway.Backend, kgateway.BackendStatus]{
		Name:   "backend",
		Client: kclient.NewFilteredDelayed[*kgateway.Backend](cl, wellknown.BackendGVR, f),
		Build: func(om metav1.ObjectMeta, st kgateway.BackendStatus) *kgateway.Backend {
			return &kgateway.Backend{ObjectMeta: om, Status: st}
		},
		GetStatus: func(o *kgateway.Backend) kgateway.BackendStatus { return o.Status },
		OnSync:    simpleStatusMetricsHook[*kgateway.Backend, kgateway.BackendStatus]("BackendStatusSyncer", wellknown.BackendGVK.Kind),
	}

	// Policies: each policy plugin derives its own desired-status collection from the
	// merged (gateway + backend path) report singleton and registers its writer.
	policyStatusInputs := plug.PolicyStatusInputs{
		Collections:   s.statusCollections,
		PolicyReports: s.policyReport.AsCollection(),
		RegisterWriter: func(gvk schema.GroupVersionKind, syncer statussync.ResourceStatusSyncer) {
			s.statusWriters[gvk] = syncer
		},
		KrtOpts: krtopts,
	}
	for _, plugin := range s.plugins.ContributesPolicies {
		if plugin.RegisterPolicyStatus != nil {
			plugin.RegisterPolicyStatus(policyStatusInputs)
		}
	}

	s.waitForSync = append(s.waitForSync,
		gatewayStatuses.HasSynced,
		listenerSetStatuses.HasSynced,
		backendStatuses.HasSynced,
	)
}

// registerRouteStatus creates a derived desired-status collection for one route kind and
// registers it with the status collections.
func registerRouteStatus[T controllers.ComparableObject](
	s *ProxySyncer,
	ctx context.Context,
	krtopts krtutil.KrtOptions,
	gvk schema.GroupVersionKind,
	col krt.Collection[T],
	hasReport func(rm reports.ReportMap, nn types.NamespacedName) bool,
) {
	gatewayReports := s.statusReport.AsCollection()
	controllerName := s.controllerName
	statuses := krt.NewCollection(col, func(kctx krt.HandlerContext, route T) *krt.ObjectWithStatus[T, gwv1.RouteStatus] {
		rw := krt.FetchOne(kctx, gatewayReports)
		if rw == nil {
			return nil
		}
		rm := rw.Reports()
		nn := types.NamespacedName{Namespace: route.GetNamespace(), Name: route.GetName()}
		// Skip routes with no report entry (e.g. routes owned by another controller) to
		// avoid building (and logging about) statuses we will never write.
		if !hasReport(rm, nn) {
			return nil
		}
		status := rm.BuildRouteStatus(ctx, route, controllerName)
		if status == nil {
			return nil
		}
		// Normalize through the same merge the writer applies so the desired status is
		// byte-identical to the written result; otherwise ordering differences would
		// re-enqueue (suppressed) writes on every recompute.
		status.Parents = statussync.MergeRouteParentStatuses(controllerName, routeStatusOf(route).Parents, status.Parents)
		return &krt.ObjectWithStatus[T, gwv1.RouteStatus]{Obj: route, Status: *status}
	}, krtopts.ToOptions(gvk.Kind+"Statuses")...)
	statussync.RegisterStatus(s.statusCollections, gvk, statuses, func(o T) gwv1.RouteStatus {
		return routeStatusOf(o)
	})
	s.waitForSync = append(s.waitForSync, statuses.HasSynced)
}

// routeStatusOf extracts the common RouteStatus from any supported route type.
func routeStatusOf(o controllers.Object) gwv1.RouteStatus {
	switch r := o.(type) {
	case *gwv1.HTTPRoute:
		return r.Status.RouteStatus
	case *gwv1.GRPCRoute:
		return r.Status.RouteStatus
	case *gwv1.TCPRoute:
		return r.Status.RouteStatus
	case *gwv1.TLSRoute:
		return r.Status.RouteStatus
	case *gwv1a2.TCPRoute:
		return r.Status.RouteStatus
	case *gwv1a2.TLSRoute:
		return r.Status.RouteStatus
	case *gwv1a3.TLSRoute:
		return r.Status.RouteStatus
	}
	return gwv1.RouteStatus{}
}

// routeWriter constructs the status writer for one route kind, wiring the multi-controller
// parent merge and the per-parent status sync metrics.
func routeWriter[T controllers.ComparableObject](
	cl apiclient.Client,
	f kclient.Filter,
	name string,
	gvr schema.GroupVersionResource,
	kind string,
	controllerName string,
	build func(om metav1.ObjectMeta, st gwv1.RouteStatus) T,
	getStatus func(T) gwv1.RouteStatus,
	parentRefs func(T) []gwv1.ParentReference,
) statussync.Writer[T, gwv1.RouteStatus] {
	return statussync.Writer[T, gwv1.RouteStatus]{
		Name:      name,
		Client:    kclient.NewFilteredDelayed[T](cl, gvr, f),
		Build:     build,
		GetStatus: getStatus,
		Merge: func(current T, desired gwv1.RouteStatus) gwv1.RouteStatus {
			desired.Parents = statussync.MergeRouteParentStatuses(controllerName, getStatus(current).Parents, desired.Parents)
			return desired
		},
		OnSync: routeStatusMetricsHook(kind, parentRefs),
	}
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
	parentRefs func(T) []gwv1.ParentReference,
) func(res statussync.Resource, current T, status gwv1.RouteStatus, took time.Duration, err error) {
	return func(res statussync.Resource, current T, status gwv1.RouteStatus, took time.Duration, err error) {
		statusErrByGateway := map[string]error{}
		for _, ps := range status.Parents {
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
}

func (s *listenerSetStatusSyncer) ApplyStatus(ctx context.Context, res statussync.Resource, statusObj any) {
	desired, ok := statusObj.(gwv1.ListenerSetStatus)
	if !ok {
		logger.Error("unexpected listener set status type", "resource", res.NamespacedName.String(), "status_type", fmt.Sprintf("%T", statusObj))
		return
	}
	start := time.Now()

	var current *gwv1.ListenerSet
	err := func() error {
		cur := s.col.GetKey(res.Namespace + "/" + res.Name)
		if cur == nil || *cur == nil {
			logger.Debug("listener set not found, skipping status update", "resource", res.NamespacedName.String())
			return nil
		}
		current = *cur

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
				// The status collection re-enqueues once the informer delivers the newer object.
				logger.Debug("skipping stale listener set status update", "resource", res.NamespacedName.String(), "error", err)
				return nil
			}
			return err
		}
		return nil
	}()
	if err != nil {
		logger.Error("error updating listener set status", "resource", res.NamespacedName.String(), "error", err)
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
