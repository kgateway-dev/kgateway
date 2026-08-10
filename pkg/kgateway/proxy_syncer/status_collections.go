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
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
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
	s.statusContributionsByTarget = krtpkg.UnnamedIndex(s.statusContributions, func(contribution reports.StatusContribution) []reports.StatusKey {
		return []reports.StatusKey{contribution.Target.Key()}
	})
	contributionsByTarget := s.statusContributionsByTarget

	// Gateway
	gatewayReports := statussync.RegisterKind(s.statusCollections, wellknown.GatewayGVK, s.commonCols.RawGateways,
		s.statusContributions, contributionsByTarget, krtopts.ToOptions("GatewayStatusReports")...)
	s.statusWriters[wellknown.GatewayGVK] = statussync.Writer[*gwv1.Gateway, gwv1.GatewayStatus]{
		Name:    "gateway",
		Current: statussync.CollectionSource(s.commonCols.RawGateways),
		Desired: func(gw *gwv1.Gateway) (gwv1.GatewayStatus, bool) {
			report, ok := statussync.ReportFor(gatewayReports, wellknown.GatewayGVK, types.NamespacedName{Namespace: gw.Namespace, Name: gw.Name})
			if !ok {
				return gwv1.GatewayStatus{}, false
			}
			status := reports.BuildGWStatus(ctx, report.Gateway, *gw, nil)
			if status == nil {
				return gwv1.GatewayStatus{}, false
			}
			return *status, true
		},
		UpdateStatus: statussync.ClientWriter(
			kclient.NewFilteredDelayed[*gwv1.Gateway](cl, wellknown.GatewayGVR, f),
			func(om metav1.ObjectMeta, st gwv1.GatewayStatus) *gwv1.Gateway {
				return &gwv1.Gateway{ObjectMeta: om, Status: st}
			}),
		GetStatus: func(o *gwv1.Gateway) gwv1.GatewayStatus { return o.Status },
		Merge:     mergeGatewayStatusAddresses,
		OnSync:    gatewayStatusMetricsHook(),
	}

	httpRouteReports := statussync.RegisterKind(s.statusCollections, wellknown.HTTPRouteGVK, s.commonCols.RawHTTPRoutes,
		s.statusContributions, contributionsByTarget, krtopts.ToOptions("HTTPRouteStatusReports")...)
	grpcRouteReports := statussync.RegisterKind(s.statusCollections, wellknown.GRPCRouteGVK, s.commonCols.RawGRPCRoutes,
		s.statusContributions, contributionsByTarget, krtopts.ToOptions("GRPCRouteStatusReports")...)
	// TCP and TLS routes are normalized to one Go type but keep the API version they were
	// served as in TypeMeta, so an enqueued resource already names the version its status
	// must be written back through. Key the reductions and the queue by that GVK, and give
	// each served version its own writer.
	tcpRouteReports := statussync.RegisterKindByObjectGVK(s.statusCollections, wellknown.TCPRouteGVK,
		s.commonCols.RawTCPRoutes, s.statusContributions, contributionsByTarget,
		krtopts.ToOptions("TCPRouteStatusReports")...)
	tlsRouteReports := statussync.RegisterKindByObjectGVK(s.statusCollections, wellknown.TLSRouteGVK,
		s.commonCols.RawTLSRoutes, s.statusContributions, contributionsByTarget,
		krtopts.ToOptions("TLSRouteStatusReports")...)

	s.statusWriters[wellknown.HTTPRouteGVK] = routeWriter[*gwv1.HTTPRoute, *gwv1.HTTPRoute](ctx, cl, f, s.commonCols.RawHTTPRoutes, httpRouteReports, wellknown.HTTPRouteGVK, "httpRoute", wellknown.HTTPRouteGVR, wellknown.HTTPRouteKind, controllerName,
		func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.HTTPRoute {
			return &gwv1.HTTPRoute{ObjectMeta: om, Status: gwv1.HTTPRouteStatus{RouteStatus: st}}
		},
		func(o *gwv1.HTTPRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
		func(o *gwv1.HTTPRoute) []gwv1.ParentReference { return o.Spec.ParentRefs },
	)
	s.statusWriters[wellknown.GRPCRouteGVK] = routeWriter[*gwv1.GRPCRoute, *gwv1.GRPCRoute](ctx, cl, f, s.commonCols.RawGRPCRoutes, grpcRouteReports, wellknown.GRPCRouteGVK, "grpcRoute", wellknown.GRPCRouteGVR, wellknown.GRPCRouteKind, controllerName,
		func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.GRPCRoute {
			return &gwv1.GRPCRoute{ObjectMeta: om, Status: gwv1.GRPCRouteStatus{RouteStatus: st}}
		},
		func(o *gwv1.GRPCRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
		func(o *gwv1.GRPCRoute) []gwv1.ParentReference { return o.Spec.ParentRefs },
	)

	// One writer per served version. A version we do not watch produces no objects, so it
	// never appears as an enqueued GVK and needs no writer; that is why this iterates the
	// same list the watches were built from rather than every version we understand.
	// Reads are the same for every version -- they all come from the normalized collection --
	// so only the object the status is written back as differs.
	tcpStatus := func(o *gwv1a2.TCPRoute) gwv1.RouteStatus { return o.Status.RouteStatus }
	tcpParentRefs := func(o *gwv1a2.TCPRoute) []gwv1.ParentReference { return o.Spec.ParentRefs }
	for _, gvr := range s.commonCols.TCPRouteWriteVersions() {
		switch gvr {
		case wellknown.TCPRouteV1GVR:
			registerStatusWriter(s.statusWriters, wellknown.TCPRouteV1GVK,
				routeWriter[*gwv1a2.TCPRoute, *gwv1.TCPRoute](ctx, cl, f, s.commonCols.RawTCPRoutes, tcpRouteReports, wellknown.TCPRouteV1GVK, "tcpRoute", gvr, wellknown.TCPRouteKind, controllerName,
					func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.TCPRoute {
						return &gwv1.TCPRoute{ObjectMeta: om, Status: gwv1.TCPRouteStatus{RouteStatus: st}}
					},
					tcpStatus, tcpParentRefs,
				))
		default:
			registerStatusWriter(s.statusWriters, wellknown.TCPRouteGVK,
				routeWriter[*gwv1a2.TCPRoute, *gwv1a2.TCPRoute](ctx, cl, f, s.commonCols.RawTCPRoutes, tcpRouteReports, wellknown.TCPRouteGVK, "tcpRoute", gvr, wellknown.TCPRouteKind, controllerName,
					func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1a2.TCPRoute {
						return &gwv1a2.TCPRoute{ObjectMeta: om, Status: gwv1a2.TCPRouteStatus{RouteStatus: st}}
					},
					tcpStatus, tcpParentRefs,
				))
		}
	}

	tlsStatus := func(o *gwv1a2.TLSRoute) gwv1.RouteStatus { return o.Status.RouteStatus }
	tlsParentRefs := func(o *gwv1a2.TLSRoute) []gwv1.ParentReference { return o.Spec.ParentRefs }
	for _, gvr := range s.commonCols.TLSRouteWriteVersions() {
		switch gvr {
		case wellknown.TLSRouteV1GVR:
			registerStatusWriter(s.statusWriters, wellknown.TLSRouteV1GVK,
				routeWriter[*gwv1a2.TLSRoute, *gwv1.TLSRoute](ctx, cl, f, s.commonCols.RawTLSRoutes, tlsRouteReports, wellknown.TLSRouteV1GVK, "tlsRoute", gvr, wellknown.TLSRouteKind, controllerName,
					func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.TLSRoute {
						return &gwv1.TLSRoute{ObjectMeta: om, Status: gwv1.TLSRouteStatus{RouteStatus: st}}
					},
					tlsStatus, tlsParentRefs,
				))
		case wellknown.TLSRouteV1Alpha3GVR:
			registerStatusWriter(s.statusWriters, wellknown.TLSRouteV1Alpha3GVK,
				routeWriter[*gwv1a2.TLSRoute, *gwv1a3.TLSRoute](ctx, cl, f, s.commonCols.RawTLSRoutes, tlsRouteReports, wellknown.TLSRouteV1Alpha3GVK, "tlsRoute", gvr, wellknown.TLSRouteKind, controllerName,
					func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1a3.TLSRoute {
						return &gwv1a3.TLSRoute{ObjectMeta: om, Status: gwv1.TLSRouteStatus{RouteStatus: st}}
					},
					tlsStatus, tlsParentRefs,
				))
		default:
			registerStatusWriter(s.statusWriters, wellknown.TLSRouteGVK,
				routeWriter[*gwv1a2.TLSRoute, *gwv1a2.TLSRoute](ctx, cl, f, s.commonCols.RawTLSRoutes, tlsRouteReports, wellknown.TLSRouteGVK, "tlsRoute", gvr, wellknown.TLSRouteKind, controllerName,
					func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1a2.TLSRoute {
						return &gwv1a2.TLSRoute{ObjectMeta: om, Status: gwv1a2.TLSRouteStatus{RouteStatus: st}}
					},
					tlsStatus, tlsParentRefs,
				))
		}
	}

	listenerSetReports := statussync.RegisterKindByObjectGVK(s.statusCollections, wellknown.ListenerSetGVK,
		s.commonCols.RawListenerSets, s.statusContributions, contributionsByTarget,
		krtopts.ToOptions("ListenerSetStatusReports")...)
	lsWriter := &listenerSetStatusSyncer{
		col:      s.commonCols.RawListenerSets,
		promoted: kclient.NewFilteredDelayed[*gwv1.ListenerSet](cl, wellknown.ListenerSetGVR, f),
		client:   cl,
		reports:  listenerSetReports,
	}
	s.statusWriters[wellknown.ListenerSetGVK] = lsWriter
	s.statusWriters[wellknown.XListenerSetGVK] = lsWriter

	backendPlugin, hasBackendPlugin := s.plugins.ContributesBackends[wellknown.BackendGVK.GroupKind()]
	var backendReports krt.Collection[statussync.ResourceReports]
	if backendPlugin.RawBackends != nil {
		backendReports = statussync.RegisterKind(s.statusCollections, wellknown.BackendGVK,
			backendPlugin.RawBackends, s.statusContributions, contributionsByTarget,
			krtopts.ToOptions("BackendStatusReports")...)
	} else if hasBackendPlugin {
		// A registered Backend plugin that does not expose its informer-backed collection is a
		// wiring bug. No Backend plugin at all is a legitimate configuration (nothing serves
		// the Backend GVK), so it gets no error.
		logger.Error("backend plugin is missing RawBackends; Backend status reconciliation is disabled",
			"group_kind", wellknown.BackendGVK.GroupKind().String())
	}
	s.statusWriters[wellknown.BackendGVK] = statussync.Writer[*kgateway.Backend, kgateway.BackendStatus]{
		Name:    "backend",
		Current: statussync.CollectionSource(backendPlugin.RawBackends),
		Desired: func(be *kgateway.Backend) (kgateway.BackendStatus, bool) {
			report, ok := statussync.ReportFor(backendReports, wellknown.BackendGVK, types.NamespacedName{Namespace: be.Namespace, Name: be.Name})
			if !ok {
				return kgateway.BackendStatus{}, false
			}
			status := reports.BuildBackendStatus(ctx, report.Backend, be.Status)
			if status == nil {
				return kgateway.BackendStatus{}, false
			}
			return *status, true
		},
		UpdateStatus: statussync.ClientWriter(
			kclient.NewFilteredDelayed[*kgateway.Backend](cl, wellknown.BackendGVR, f),
			func(om metav1.ObjectMeta, st kgateway.BackendStatus) *kgateway.Backend {
				return &kgateway.Backend{ObjectMeta: om, Status: st}
			}),
		GetStatus: func(o *kgateway.Backend) kgateway.BackendStatus { return o.Status },
		OnSync:    simpleStatusMetricsHook[*kgateway.Backend, kgateway.BackendStatus]("BackendStatusSyncer", wellknown.BackendGVK.Kind),
	}

	policyStatusInputs := plug.PolicyStatusInputs{
		Ctx:                   ctx,
		Collections:           s.statusCollections,
		StatusContributions:   s.statusContributions,
		ContributionsByTarget: contributionsByTarget,
		KrtOpts:               krtopts,
		RegisterWriter: func(gvk schema.GroupVersionKind, syncer statussync.ResourceStatusSyncer) {
			registerStatusWriter(s.statusWriters, gvk, syncer)
		},
	}
	for _, plugin := range s.plugins.ContributesPolicies {
		if plugin.RegisterPolicyStatus != nil {
			plugin.RegisterPolicyStatus(policyStatusInputs)
		}
	}

	// The single entry covering every report reducer, here and in any plugin or downstream
	// registration that runs later: statussync.RegisterResourceReports is what enrols them,
	// and HasSynced re-reads that set on each call. StatusSyncer inherits this through
	// CacheSyncs, so it must not be added there too.
	s.waitForSync = append(s.waitForSync, s.statusCollections.HasSynced)
}

// registerStatusWriter records the writer for a GVK. Exactly one writer may own a GVK:
// a second registration means two plugins both claim to persist that resource's status,
// and whichever registered last would silently win. Keep the first and report the conflict
// rather than letting registration order decide who writes status.
func registerStatusWriter(
	writers map[schema.GroupVersionKind]statussync.ResourceStatusSyncer,
	gvk schema.GroupVersionKind,
	syncer statussync.ResourceStatusSyncer,
) {
	if _, exists := writers[gvk]; exists {
		logger.Error("status writer already registered for resource type; ignoring duplicate registration",
			"gvk", gvk.String())
		return
	}
	writers[gvk] = syncer
}

// routeWriter constructs the status writer for one route kind, wiring the multi-controller
// parent merge and the per-parent status sync metrics.
//
// R is the type the route collection holds and W the type its status is written back as.
// They differ for TCP and TLS routes, whose collections are normalized to one Go type while
// the write must go through the API version the object is actually served as.
func routeWriter[R, W controllers.ComparableObject](
	ctx context.Context,
	cl apiclient.Client,
	f kclient.Filter,
	routes krt.Collection[R],
	reportCol krt.Collection[statussync.ResourceReports],
	gvk schema.GroupVersionKind,
	name string,
	gvr schema.GroupVersionResource,
	kind string,
	controllerName string,
	build func(om metav1.ObjectMeta, st gwv1.RouteStatus) W,
	getStatus func(R) gwv1.RouteStatus,
	parentRefs func(R) []gwv1.ParentReference,
) statussync.Writer[R, gwv1.RouteStatus] {
	return statussync.Writer[R, gwv1.RouteStatus]{
		Name:    name,
		Current: statussync.CollectionSource(routes),
		UpdateStatus: statussync.ClientWriter(
			kclient.NewFilteredDelayed[W](cl, gvr, f), build),
		Desired: func(current R) (gwv1.RouteStatus, bool) {
			nn := types.NamespacedName{Namespace: current.GetNamespace(), Name: current.GetName()}
			report, ok := statussync.ReportFor(reportCol, gvk, nn)
			if !ok {
				return gwv1.RouteStatus{}, false
			}
			if report.Route == nil {
				// An empty desired status clears only this controller's stale parents in Merge,
				// so it is worth writing only if we have parents to clear. Every route in the
				// watched namespaces lands here, most of them ours to translate but many not.
				if !statussync.OwnsAnyRouteParent(controllerName, getStatus(current).Parents) {
					return gwv1.RouteStatus{}, false
				}
				return gwv1.RouteStatus{}, true
			}
			status := reports.BuildRouteStatus(ctx, report.Route, current, controllerName)
			if status == nil {
				// The route has a report, so the only way the builder returns nil is that it
				// does not recognise this Go type. Suppress the write instead of publishing an
				// empty status, which Merge reads as "clear every parent we own" — a missing
				// type switch case must not erase good status.
				logger.Error("route status builder does not support this type; skipping status update",
					"gvk", gvk.String(), "resource", nn.String(), "type", fmt.Sprintf("%T", current))
				return gwv1.RouteStatus{}, false
			}
			return *status, true
		},
		GetStatus: getStatus,
		Merge: func(current R, desired gwv1.RouteStatus) gwv1.RouteStatus {
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

// The Accepted/Programmed condition types and the reasons considered healthy, used to
// derive the status-sync error result the previous syncer reported.
var (
	gatewayConditionTypes = []string{
		string(gwv1.GatewayConditionAccepted),
		string(gwv1.GatewayConditionProgrammed),
	}
	gatewayAcceptedReasons = []string{
		string(gwv1.GatewayReasonAccepted),
		string(gwv1.GatewayReasonProgrammed),
		string(gwv1.GatewayReasonPending),
	}
	listenerSetConditionTypes = []string{
		string(gwv1.ListenerSetConditionAccepted),
		string(gwv1.ListenerSetConditionProgrammed),
	}
	listenerSetAcceptedReasons = []string{
		string(gwv1.ListenerSetReasonAccepted),
		string(gwv1.ListenerSetReasonProgrammed),
		string(gwv1.ListenerSetReasonPending),
	}
)

// gatewayStatusMetricsHook records status sync metrics for Gateways, deriving an error
// result from invalid Accepted/Programmed condition reasons like the previous syncer did.
func gatewayStatusMetricsHook() func(res statussync.Resource, current *gwv1.Gateway, status gwv1.GatewayStatus, took time.Duration, err error) {
	return func(res statussync.Resource, current *gwv1.Gateway, status gwv1.GatewayStatus, took time.Duration, err error) {
		statusErr := statussync.ConditionError(err, status.Conditions,
			gatewayConditionTypes, gatewayAcceptedReasons, "invalid gateway condition")
		statussync.RecordStatusSync(statussync.SyncMetricLabels{
			Name:      res.Name,
			Namespace: res.Namespace,
			Syncer:    "GatewayStatusSyncer",
		}, took, statusErr)
		statussync.EndResourceStatusSyncOnWriteSuccess(err, kmetrics.ResourceSyncDetails{
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
		// Every emitter below is a no-op while metrics are disabled, so skip the scan and
		// the allocations entirely rather than building them for nothing on each write.
		if !metrics.Active() {
			return
		}
		// Allocated on the first invalid condition: routes are healthy in the common case.
		var statusErrByGateway map[string]error
		setStatusErr := func(gwName string, statusErr error) {
			if statusErrByGateway == nil {
				statusErrByGateway = map[string]error{}
			}
			statusErrByGateway[gwName] = statusErr
		}
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
					setStatusErr(gwName, errors.New("partially invalid route condition"))
				case cond.Type == conditions.KgatewayConditionProgrammed && cond.Status != metav1.ConditionTrue:
					setStatusErr(gwName, errors.New("invalid route condition"))
				case cond.Type == string(gwv1.RouteConditionAccepted) &&
					cond.Reason != string(gwv1.RouteReasonAccepted) &&
					cond.Reason != string(gwv1.RouteReasonPending):
					setStatusErr(gwName, errors.New("invalid route condition"))
				}
			}
		}

		if controllers.IsNil(current) {
			return
		}
		for _, pr := range parentRefs(current) {
			gwName := string(pr.Name)
			statussync.RecordStatusSync(statussync.SyncMetricLabels{
				Name:      gwName,
				Namespace: res.Namespace,
				Syncer:    "RouteStatusSyncer",
			}, took, errors.Join(err, statusErrByGateway[gwName]))
			statussync.EndResourceStatusSyncOnWriteSuccess(err, kmetrics.ResourceSyncDetails{
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
		statussync.EndResourceStatusSyncOnWriteSuccess(err, kmetrics.ResourceSyncDetails{
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
		report, ok := statussync.ReportFor(s.reports, res.GroupVersionKind, res.NamespacedName)
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

	statusErr := statussync.ConditionError(err, desired.Conditions,
		listenerSetConditionTypes, listenerSetAcceptedReasons, "invalid listener condition")
	parentName := ""
	if current != nil {
		parentName = string(current.Spec.ParentRef.Name)
	}
	statussync.RecordStatusSync(statussync.SyncMetricLabels{
		Name:      parentName,
		Namespace: res.Namespace,
		Syncer:    "ListenerSetStatusSyncer",
	}, time.Since(start), statusErr)
	statussync.EndResourceStatusSyncOnWriteSuccess(err, kmetrics.ResourceSyncDetails{
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
//
// The patch body carries metadata.resourceVersion so the API server rejects the write when
// the object has moved on, matching the optimistic concurrency the promoted path gets from
// UpdateStatus. Without it a merge patch applies unconditionally, so a status built from a
// stale read silently overwrites a newer one and nothing re-enqueues to correct it.
func (s *listenerSetStatusSyncer) patchLegacyStatus(ctx context.Context, res statussync.Resource, current *gwv1.ListenerSet, desired gwv1.ListenerSetStatus) error {
	statusMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&desired)
	if err != nil {
		return err
	}
	injectListenerPorts(statusMap, current.Spec.Listeners)
	data, err := json.Marshal(map[string]any{
		"metadata": map[string]any{"resourceVersion": current.ResourceVersion},
		"status":   statusMap,
	})
	if err != nil {
		return err
	}
	_, err = s.client.Dynamic().Resource(wellknown.XListenerSetGVR).Namespace(res.Namespace).
		Patch(ctx, res.Name, types.MergePatchType, data, metav1.PatchOptions{}, "status")
	if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
		// Same self-healing contract as the promoted path: the raw collection re-enqueues
		// once the informer delivers the newer object.
		logger.Debug("skipping stale legacy listener set status update", "resource", res.NamespacedName.String(), "error", err)
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
