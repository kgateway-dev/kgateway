package proxy_syncer

import (
	"fmt"
	"slices"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1a3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// initStatusInfra registers every status kind with the status pipeline: the built-in Gateway
// API and Backend kinds, the policy plugins' kinds, then the downstream registrations. Each
// gets a per-owner reduction of translation's status contributions, raw-object and reduced-
// report reconciliation sources, and a writer. Desired Kubernetes status is still built just in
// time by the leader's writer.
func (s *ProxySyncer) initStatusInfra(contributions krt.Collection[reports.StatusContribution], krtopts krtutil.KrtOptions) {
	s.status.init(s.controllerName, contributions, krtopts)
	in := s.status.in
	cl := s.apiClient
	f := kclient.Filter{ObjectFilter: cl.ObjectFilter()}
	controllerName := s.controllerName

	// Gateway
	gatewayReports := statussync.Register(in, wellknown.GatewayGVK, s.commonCols.RawGateways)
	in.RegisterWriter(wellknown.GatewayGVK, gatewayWriter(cl, f, s.commonCols.RawGateways, gatewayReports))

	httpRouteReports := statussync.Register(in, wellknown.HTTPRouteGVK, s.commonCols.RawHTTPRoutes)
	grpcRouteReports := statussync.Register(in, wellknown.GRPCRouteGVK, s.commonCols.RawGRPCRoutes)
	// TCP and TLS routes are normalized to one Go type but keep the API version they were
	// served as in TypeMeta, so an enqueued resource already names the version its status
	// must be written back through. Key the reductions and the queue by that GVK, and give
	// each served version its own writer.
	tcpRouteReports := statussync.RegisterByObjectGVK(in, wellknown.TCPRouteGVK, s.commonCols.RawTCPRoutes)
	tlsRouteReports := statussync.RegisterByObjectGVK(in, wellknown.TLSRouteGVK, s.commonCols.RawTLSRoutes)

	in.RegisterWriter(wellknown.HTTPRouteGVK,
		routeWriter(cl, f, s.commonCols.RawHTTPRoutes, httpRouteReports, wellknown.HTTPRouteGVK, wellknown.HTTPRouteGVR, controllerName,
			func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.HTTPRoute {
				return &gwv1.HTTPRoute{ObjectMeta: om, Status: gwv1.HTTPRouteStatus{RouteStatus: st}}
			},
			func(o *gwv1.HTTPRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
		))
	in.RegisterWriter(wellknown.GRPCRouteGVK,
		routeWriter(cl, f, s.commonCols.RawGRPCRoutes, grpcRouteReports, wellknown.GRPCRouteGVK, wellknown.GRPCRouteGVR, controllerName,
			func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.GRPCRoute {
				return &gwv1.GRPCRoute{ObjectMeta: om, Status: gwv1.GRPCRouteStatus{RouteStatus: st}}
			},
			func(o *gwv1.GRPCRoute) gwv1.RouteStatus { return o.Status.RouteStatus },
		))

	// One writer per served version. A version we do not watch produces no objects, so it
	// never appears as an enqueued GVK and needs no writer; that is why this iterates the
	// same list the watches were built from rather than every version we understand.
	// Reads are the same for every version -- they all come from the normalized collection --
	// so only the object the status is written back as differs.
	tcpStatus := func(o *gwv1a2.TCPRoute) gwv1.RouteStatus { return o.Status.RouteStatus }
	for _, gvr := range s.commonCols.TCPRouteWriteVersions() {
		switch gvr {
		case wellknown.TCPRouteV1GVR:
			in.RegisterWriter(wellknown.TCPRouteV1GVK,
				routeWriter(cl, f, s.commonCols.RawTCPRoutes, tcpRouteReports, wellknown.TCPRouteV1GVK, gvr, controllerName,
					func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.TCPRoute {
						return &gwv1.TCPRoute{ObjectMeta: om, Status: gwv1.TCPRouteStatus{RouteStatus: st}}
					},
					tcpStatus,
				))
		default:
			in.RegisterWriter(wellknown.TCPRouteGVK,
				routeWriter(cl, f, s.commonCols.RawTCPRoutes, tcpRouteReports, wellknown.TCPRouteGVK, gvr, controllerName,
					func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1a2.TCPRoute {
						return &gwv1a2.TCPRoute{ObjectMeta: om, Status: gwv1a2.TCPRouteStatus{RouteStatus: st}}
					},
					tcpStatus,
				))
		}
	}

	tlsStatus := func(o *gwv1a2.TLSRoute) gwv1.RouteStatus { return o.Status.RouteStatus }
	for _, gvr := range s.commonCols.TLSRouteWriteVersions() {
		switch gvr {
		case wellknown.TLSRouteV1GVR:
			in.RegisterWriter(wellknown.TLSRouteV1GVK,
				routeWriter[*gwv1a2.TLSRoute, *gwv1.TLSRoute](cl, f, s.commonCols.RawTLSRoutes, tlsRouteReports, wellknown.TLSRouteV1GVK, gvr, controllerName,
					func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1.TLSRoute {
						return &gwv1.TLSRoute{ObjectMeta: om, Status: gwv1.TLSRouteStatus{RouteStatus: st}}
					},
					tlsStatus,
				))
		case wellknown.TLSRouteV1Alpha3GVR:
			in.RegisterWriter(wellknown.TLSRouteV1Alpha3GVK,
				routeWriter(cl, f, s.commonCols.RawTLSRoutes, tlsRouteReports, wellknown.TLSRouteV1Alpha3GVK, gvr, controllerName,
					func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1a3.TLSRoute {
						return &gwv1a3.TLSRoute{ObjectMeta: om, Status: gwv1.TLSRouteStatus{RouteStatus: st}}
					},
					tlsStatus,
				))
		default:
			in.RegisterWriter(wellknown.TLSRouteGVK,
				routeWriter(cl, f, s.commonCols.RawTLSRoutes, tlsRouteReports, wellknown.TLSRouteGVK, gvr, controllerName,
					func(om metav1.ObjectMeta, st gwv1.RouteStatus) *gwv1a2.TLSRoute {
						return &gwv1a2.TLSRoute{ObjectMeta: om, Status: gwv1a2.TLSRouteStatus{RouteStatus: st}}
					},
					tlsStatus,
				))
		}
	}

	// Both ListenerSet flavors normalize to one collection and one report reducer keyed by the
	// object's own GVK, but they are written back through different APIs, so each GVK gets its
	// own writer. The map dispatches on the enqueued GVK, which is the object's, so a promoted
	// ListenerSet can never reach the legacy writer or vice versa.
	listenerSetReports := statussync.RegisterByObjectGVK(in, wellknown.ListenerSetGVK, s.commonCols.RawListenerSets)
	in.RegisterWriter(wellknown.ListenerSetGVK,
		listenerSetWriter(cl, f, s.commonCols.RawListenerSets, listenerSetReports))
	// ON_EXPERIMENTAL_PROMOTION : Remove this registration together with xlistenerset_status.go.
	// Ref: https://github.com/kgateway-dev/kgateway/issues/12827
	in.RegisterWriter(wellknown.XListenerSetGVK,
		xListenerSetWriter(cl, s.commonCols.RawListenerSets, listenerSetReports))

	backendPlugin, hasBackendPlugin := s.plugins.ContributesBackends[wellknown.BackendGVK.GroupKind()]
	var backendReports krt.Collection[statussync.ResourceReports]
	if backendPlugin.RawBackends != nil {
		backendReports = statussync.Register(in, wellknown.BackendGVK, backendPlugin.RawBackends)
	} else if hasBackendPlugin {
		// A registered Backend plugin that does not expose its informer-backed collection is a
		// wiring bug. No Backend plugin at all is a legitimate configuration (nothing serves
		// the Backend GVK), so it gets no error.
		logger.Error("backend plugin is missing RawBackends; Backend status reconciliation is disabled",
			"group_kind", wellknown.BackendGVK.GroupKind().String())
	}
	in.RegisterWriter(wellknown.BackendGVK, statussync.Writer[*kgateway.Backend, kgateway.BackendStatus]{
		Name:    "backend",
		Current: statussync.CollectionSource(backendPlugin.RawBackends),
		Desired: func(res statussync.Resource, be *kgateway.Backend) (kgateway.BackendStatus, bool) {
			report, ok := statussync.ReportFor(backendReports, res)
			if !ok {
				return kgateway.BackendStatus{}, false
			}
			status := reports.BuildBackendStatus(report.Backend, be.Status)
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
	})

	for _, plugin := range s.plugins.ContributesPolicies {
		if plugin.RegisterPolicyStatus != nil {
			plugin.RegisterPolicyStatus(in)
		}
	}
	s.status.registerDownstream()

	// The single entry covering every report reducer registered above:
	// statussync.RegisterResourceReports is what enrolls them, and HasSynced re-reads that
	// set on each call. The StatusSyncer waits on the same barrier, cloned, so it must not be
	// added there too.
	s.waitForSync = append(s.waitForSync, in.Collections.HasSynced)
	s.status.cacheSyncs = slices.Clone(s.waitForSync)
}

// gatewayWriter constructs the Gateway status writer, wiring the deployer-owned address
// merge and the Gateway status sync metrics. It is a function rather than a literal inside
// initStatusInfra so tests can exercise the writer this controller actually runs — the
// convergence invariant in TestStatusWritersConvergeAfterOneWrite is only worth anything
// against the real builder and merge.
func gatewayWriter(
	cl apiclient.Client,
	f kclient.Filter,
	gateways krt.Collection[*gwv1.Gateway],
	reportCol krt.Collection[statussync.ResourceReports],
) statussync.Writer[*gwv1.Gateway, gwv1.GatewayStatus] {
	return statussync.Writer[*gwv1.Gateway, gwv1.GatewayStatus]{
		Name:    "gateway",
		Current: statussync.CollectionSource(gateways),
		Desired: func(res statussync.Resource, gw *gwv1.Gateway) (gwv1.GatewayStatus, bool) {
			report, ok := statussync.ReportFor(reportCol, res)
			if !ok {
				return gwv1.GatewayStatus{}, false
			}
			status := reports.BuildGWStatus(report.Gateway, *gw, nil)
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
	}
}

// routeWriter constructs the status writer for one route kind, wiring the multi-controller
// parent merge and the per-parent status sync metrics.
//
// R is the type the route collection holds and W the type its status is written back as.
// They differ for TCP and TLS routes, whose collections are normalized to one Go type while
// the write must go through the API version the object is actually served as.
//
// gvk is the identity this writer is registered under; the writer's log name is derived from
// it rather than passed alongside it, so a caller cannot spell the same kind two ways in one
// call.
func routeWriter[R, W controllers.ComparableObject](
	cl apiclient.Client,
	f kclient.Filter,
	routes krt.Collection[R],
	reportCol krt.Collection[statussync.ResourceReports],
	gvk schema.GroupVersionKind,
	gvr schema.GroupVersionResource,
	controllerName string,
	build func(om metav1.ObjectMeta, st gwv1.RouteStatus) W,
	getStatus func(R) gwv1.RouteStatus,
) statussync.Writer[R, gwv1.RouteStatus] {
	return statussync.Writer[R, gwv1.RouteStatus]{
		Name:    gvk.Kind,
		Current: statussync.CollectionSource(routes),
		UpdateStatus: statussync.ClientWriter(
			kclient.NewFilteredDelayed[W](cl, gvr, f), build),
		Desired: func(res statussync.Resource, current R) (gwv1.RouteStatus, bool) {
			report, ok := statussync.ReportFor(reportCol, res)
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
			status := reports.BuildRouteStatus(report.Route, current, controllerName)
			if status == nil {
				// The route has a report, so the only way the builder returns nil is that it
				// does not recognise this Go type. Suppress the write instead of publishing an
				// empty status, which Merge reads as "clear every parent we own" — a missing
				// type switch case must not erase good status.
				logger.Error("route status builder does not support this type; skipping status update",
					"gvk", res.GroupVersionKind.String(), "resource", res.NamespacedName.String(),
					"type", fmt.Sprintf("%T", current))
				return gwv1.RouteStatus{}, false
			}
			return *status, true
		},
		GetStatus: getStatus,
		Merge: func(current R, desired gwv1.RouteStatus) gwv1.RouteStatus {
			desired.Parents = statussync.MergeRouteParentStatuses(controllerName, getStatus(current).Parents, desired.Parents)
			return desired
		},
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

// listenerSetWriter constructs the status writer for promoted ListenerSets, which are
// written back through the typed client like every other Gateway API kind.
//
// It is a function rather than a literal inside initStatusInfra for the same reason
// gatewayWriter is: the convergence and idempotence checks are only worth anything against
// the writer this controller actually runs.
func listenerSetWriter(
	cl apiclient.Client,
	f kclient.Filter,
	listenerSets krt.Collection[*gwv1.ListenerSet],
	reportCol krt.Collection[statussync.ResourceReports],
) statussync.Writer[*gwv1.ListenerSet, gwv1.ListenerSetStatus] {
	return statussync.Writer[*gwv1.ListenerSet, gwv1.ListenerSetStatus]{
		Name:    "listenerSet",
		Current: statussync.CollectionSource(listenerSets),
		Desired: listenerSetDesired(reportCol),
		UpdateStatus: statussync.ClientWriter(
			kclient.NewFilteredDelayed[*gwv1.ListenerSet](cl, wellknown.ListenerSetGVR, f),
			func(om metav1.ObjectMeta, st gwv1.ListenerSetStatus) *gwv1.ListenerSet {
				return &gwv1.ListenerSet{ObjectMeta: om, Status: st}
			}),
		GetStatus: func(o *gwv1.ListenerSet) gwv1.ListenerSetStatus { return o.Status },
	}
}

// listenerSetDesired builds the desired status for either ListenerSet flavor, satisfying
// Writer.Desired.
//
// The promoted and legacy flavors share one normalized collection whose reports are keyed by
// the object's own GVK, and the enqueued Resource already carries that GVK -- which is the
// same one the writers are dispatched on. So there is nothing left for the two writers to
// differ on here, and no captured GVK that can disagree with the queue.
func listenerSetDesired(
	reportCol krt.Collection[statussync.ResourceReports],
) func(statussync.Resource, *gwv1.ListenerSet) (gwv1.ListenerSetStatus, bool) {
	return func(res statussync.Resource, current *gwv1.ListenerSet) (gwv1.ListenerSetStatus, bool) {
		report, ok := statussync.ReportFor(reportCol, res)
		if !ok {
			return gwv1.ListenerSetStatus{}, false
		}
		status := reports.BuildListenerSetStatus(report.ListenerSet, *current)
		if status == nil {
			return gwv1.ListenerSetStatus{}, false
		}
		return *status, true
	}
}
