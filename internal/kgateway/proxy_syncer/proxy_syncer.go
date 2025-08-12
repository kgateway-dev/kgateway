package proxy_syncer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sync/atomic"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	utilretry "k8s.io/client-go/util/retry"

	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"

	"github.com/avast/retry-go/v4"
	envoycachetypes "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/irtranslator"
	tmetrics "github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/metrics"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	krtinternal "github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	plug "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

// ProxySyncer orchestrates the translation of K8s Gateway CRs to xDS
// and setting the output xDS snapshot in the envoy snapshot cache,
// resulting in each connected proxy getting the correct configuration.
// ProxySyncer also syncs status resulting from translation to K8s apiserver.
type ProxySyncer struct {
	controllerName        string
	agentGatewayClassName string

	mgr        manager.Manager
	commonCols *common.CommonCollections
	translator *translator.CombinedTranslator
	plugins    plug.Plugin

	istioClient     kube.Client
	proxyTranslator ProxyTranslator

	uniqueClients krt.Collection[ir.UniqlyConnectedClient]

	statusReport            krt.Singleton[report]
	backendPolicyReport     krt.Singleton[report]
	mostXdsSnapshots        krt.Collection[GatewayXdsResources]
	perclientSnapCollection krt.Collection[XdsSnapWrapper]

	waitForSync []cache.InformerSynced
	ready       atomic.Bool
}

type GatewayXdsResources struct {
	types.NamespacedName

	reports reports.ReportMap
	// Clusters are items in the CDS response payload.
	Clusters     []envoycachetypes.ResourceWithTTL
	ClustersHash uint64

	// Routes are items in the RDS response payload.
	Routes envoycache.Resources

	// Listeners are items in the LDS response payload.
	Listeners envoycache.Resources
}

func (r GatewayXdsResources) ResourceName() string {
	return xds.OwnerNamespaceNameID(wellknown.GatewayApiProxyValue, r.Namespace, r.Name)
}

func (r GatewayXdsResources) Equals(in GatewayXdsResources) bool {
	return r.NamespacedName == in.NamespacedName && report{r.reports}.Equals(report{in.reports}) && r.ClustersHash == in.ClustersHash &&
		r.Routes.Version == in.Routes.Version && r.Listeners.Version == in.Listeners.Version
}

func sliceToResourcesHash[T proto.Message](slice []T) ([]envoycachetypes.ResourceWithTTL, uint64) {
	var slicePb []envoycachetypes.ResourceWithTTL
	var resourcesHash uint64
	for _, r := range slice {
		var m proto.Message = r
		hash := utils.HashProto(r)
		slicePb = append(slicePb, envoycachetypes.ResourceWithTTL{Resource: m})
		resourcesHash ^= hash
	}

	return slicePb, resourcesHash
}

func sliceToResources[T proto.Message](slice []T) envoycache.Resources {
	r, h := sliceToResourcesHash(slice)
	return envoycache.NewResourcesWithTTL(fmt.Sprintf("%d", h), r)
}

func toResources(gw ir.Gateway, xdsSnap irtranslator.TranslationResult, r reports.ReportMap) *GatewayXdsResources {
	c, ch := sliceToResourcesHash(xdsSnap.ExtraClusters)
	return &GatewayXdsResources{
		NamespacedName: types.NamespacedName{
			Namespace: gw.Obj.GetNamespace(),
			Name:      gw.Obj.GetName(),
		},
		reports:      r,
		ClustersHash: ch,
		Clusters:     c,
		Routes:       sliceToResources(xdsSnap.Routes),
		Listeners:    sliceToResources(xdsSnap.Listeners),
	}
}

// NewProxySyncer returns an implementation of the ProxySyncer
// The provided GatewayInputChannels are used to trigger syncs.
func NewProxySyncer(
	ctx context.Context,
	controllerName string,
	mgr manager.Manager,
	client kube.Client,
	uniqueClients krt.Collection[ir.UniqlyConnectedClient],
	mergedPlugins plug.Plugin,
	commonCols *common.CommonCollections,
	xdsCache envoycache.SnapshotCache,
	agentGatewayClassName string,
) *ProxySyncer {
	return &ProxySyncer{
		controllerName:        controllerName,
		agentGatewayClassName: agentGatewayClassName,
		commonCols:            commonCols,
		mgr:                   mgr,
		istioClient:           client,
		proxyTranslator:       NewProxyTranslator(xdsCache),
		uniqueClients:         uniqueClients,
		translator:            translator.NewCombinedTranslator(ctx, mergedPlugins, commonCols),
		plugins:               mergedPlugins,
	}
}

type ProxyTranslator struct {
	xdsCache envoycache.SnapshotCache
}

func NewProxyTranslator(xdsCache envoycache.SnapshotCache) ProxyTranslator {
	return ProxyTranslator{
		xdsCache: xdsCache,
	}
}

type report struct {
	// lower case so krt doesn't error in debug handler
	reportMap reports.ReportMap
}

func (r report) ResourceName() string {
	return "report"
}

// do we really need this for a singleton?
func (r report) Equals(in report) bool {
	if !maps.Equal(r.reportMap.Gateways, in.reportMap.Gateways) {
		return false
	}
	if !maps.Equal(r.reportMap.ListenerSets, in.reportMap.ListenerSets) {
		return false
	}
	if !maps.Equal(r.reportMap.HTTPRoutes, in.reportMap.HTTPRoutes) {
		return false
	}
	if !maps.Equal(r.reportMap.TCPRoutes, in.reportMap.TCPRoutes) {
		return false
	}
	if !maps.Equal(r.reportMap.TLSRoutes, in.reportMap.TLSRoutes) {
		return false
	}
	if !maps.Equal(r.reportMap.Policies, in.reportMap.Policies) {
		return false
	}
	return true
}

var logger = logging.New("proxy_syncer")

func (s *ProxySyncer) Init(ctx context.Context, krtopts krtinternal.KrtOptions) {
	// all backends with policies attached in a single collection
	finalBackends := krt.JoinCollection(s.commonCols.BackendIndex.BackendsWithPolicy(),
		// WithJoinUnchecked enables a more optimized lookup on the hotpath by assuming we do not have any overlapping ResourceName
		// in the backend collection.
		append(krtopts.ToOptions("FinalBackends"), krt.WithJoinUnchecked())...)

	s.translator.Init(ctx)

	s.mostXdsSnapshots = krt.NewCollection(s.commonCols.GatewayIndex.Gateways, func(kctx krt.HandlerContext, gw ir.Gateway) *GatewayXdsResources {
		// skip agentgateway proxies as they are not envoy-based gateways
		if string(gw.Obj.Spec.GatewayClassName) == s.agentGatewayClassName {
			logger.Debug("skipping envoy proxy sync for agentgateway %s.%s", gw.Obj.Name, gw.Obj.Namespace)
			return nil
		}

		logger.Debug("building proxy for kube gw", "name", client.ObjectKeyFromObject(gw.Obj), "version", gw.Obj.GetResourceVersion())

		xdsSnap, rm := s.translator.TranslateGateway(kctx, ctx, gw)
		if xdsSnap == nil {
			return nil
		}

		return toResources(gw, *xdsSnap, rm)
	}, krtopts.ToOptions("MostXdsSnapshots")...)

	epPerClient := NewPerClientEnvoyEndpoints(
		krtopts,
		s.uniqueClients,
		s.commonCols.Endpoints,
		s.translator.TranslateEndpoints,
	)
	clustersPerClient := NewPerClientEnvoyClusters(
		ctx,
		krtopts,
		s.translator.GetUpstreamTranslator(),
		finalBackends,
		s.uniqueClients,
	)

	s.perclientSnapCollection = snapshotPerClient(
		krtopts,
		s.uniqueClients,
		s.mostXdsSnapshots,
		epPerClient,
		clustersPerClient,
	)

	s.backendPolicyReport = krt.NewSingleton(func(kctx krt.HandlerContext) *report {
		backends := krt.Fetch(kctx, finalBackends)
		merged := generateBackendPolicyReport(backends)
		return &report{merged}
	}, krtopts.ToOptions("BackendsPolicyReport")...)

	// as proxies are created, they also contain a reportMap containing status for the Gateway and associated xRoutes (really parentRefs)
	// here we will merge reports that are per-Proxy to a singleton Report used to persist to k8s on a timer
	s.statusReport = krt.NewSingleton(func(kctx krt.HandlerContext) *report {
		proxies := krt.Fetch(kctx, s.mostXdsSnapshots)
		merged := mergeProxyReports(proxies)
		return &report{merged}
	})

	s.waitForSync = []cache.InformerSynced{
		s.commonCols.HasSynced,
		finalBackends.HasSynced,
		s.perclientSnapCollection.HasSynced,
		s.mostXdsSnapshots.HasSynced,
		s.plugins.HasSynced,
		s.translator.HasSynced,
	}
}

func mergeProxyReports(
	proxies []GatewayXdsResources,
) reports.ReportMap {
	merged := reports.NewReportMap()
	for _, p := range proxies {
		// 1. merge GW Reports for all Proxies' status reports
		maps.Copy(merged.Gateways, p.reports.Gateways)

		// 2. merge LS Reports for all Proxies' status reports
		maps.Copy(merged.ListenerSets, p.reports.ListenerSets)

		// 3. merge httproute parentRefs into RouteReports
		for rnn, rr := range p.reports.HTTPRoutes {
			// if we haven't encountered this route, just copy it over completely
			old := merged.HTTPRoutes[rnn]
			if old == nil {
				merged.HTTPRoutes[rnn] = rr
				continue
			}
			// else, this route has already been seen for a proxy, merge this proxy's parents
			// into the merged report
			maps.Copy(merged.HTTPRoutes[rnn].Parents, rr.Parents)
		}

		// 4. merge tcproute parentRefs into RouteReports
		for rnn, rr := range p.reports.TCPRoutes {
			// if we haven't encountered this route, just copy it over completely
			old := merged.TCPRoutes[rnn]
			if old == nil {
				merged.TCPRoutes[rnn] = rr
				continue
			}
			// else, this route has already been seen for a proxy, merge this proxy's parents
			// into the merged report
			maps.Copy(merged.TCPRoutes[rnn].Parents, rr.Parents)
		}

		for rnn, rr := range p.reports.TLSRoutes {
			// if we haven't encountered this route, just copy it over completely
			old := merged.TLSRoutes[rnn]
			if old == nil {
				merged.TLSRoutes[rnn] = rr
				continue
			}
			// else, this route has already been seen for a proxy, merge this proxy's parents
			// into the merged report
			maps.Copy(merged.TLSRoutes[rnn].Parents, rr.Parents)
		}

		for rnn, rr := range p.reports.GRPCRoutes {
			// if we haven't encountered this route, just copy it over completely
			old := merged.GRPCRoutes[rnn]
			if old == nil {
				merged.GRPCRoutes[rnn] = rr
				continue
			}
			// else, this route has already been seen for a proxy, merge this proxy's parents
			// into the merged report
			maps.Copy(merged.GRPCRoutes[rnn].Parents, rr.Parents)
		}

		for key, report := range p.reports.Policies {
			// if we haven't encountered this policy, just copy it over completely
			old := merged.Policies[key]
			if old == nil {
				merged.Policies[key] = report
				continue
			}
			// else, let's merge our parentRefs into the existing map
			// obsGen will stay as-is...
			maps.Copy(merged.Policies[key].Ancestors, report.Ancestors)
		}
	}

	return merged
}

func (s *ProxySyncer) Start(ctx context.Context) error {
	logger.Info("starting Proxy Syncer", "controller", s.controllerName)

	// wait for krt collections to sync
	logger.Info("waiting for cache to sync")
	s.istioClient.WaitForCacheSync(
		"kube gw proxy syncer",
		ctx.Done(),
		s.waitForSync...,
	)

	// wait for ctrl-rtime caches to sync before accepting events
	if !s.mgr.GetCache().WaitForCacheSync(ctx) {
		return errors.New("kube gateway sync loop waiting for all caches to sync failed")
	}
	logger.Info("caches warm!")

	// caches are warm, now we can do registrations

	// latestReport will be constantly updated to contain the merged status report for Kube Gateway status
	// when timer ticks, we will use the state of the mergedReports at that point in time to sync the status to k8s
	latestReportQueue := utils.NewAsyncQueue[reports.ReportMap]()
	s.statusReport.Register(func(o krt.Event[report]) {
		if o.Event == controllers.EventDelete {
			// TODO: handle garbage collection (see: https://github.com/solo-io/solo-projects/issues/7086)
			return
		}
		latestReportQueue.Enqueue(o.Latest().reportMap)
	})

	routeStatusLogger := logger.With("subcomponent", "routeStatusSyncer")
	listenerSetStatusLogger := logger.With("subcomponent", "listenerSetStatusSyncer")
	gatewayStatusLogger := logger.With("subcomponent", "gatewayStatusSyncer")
	go func() {
		for {
			latestReport, err := latestReportQueue.Dequeue(ctx)
			if err != nil {
				return
			}
			s.syncGatewayStatus(ctx, gatewayStatusLogger, latestReport)
			s.syncListenerSetStatus(ctx, listenerSetStatusLogger, latestReport)
			s.syncRouteStatus(ctx, routeStatusLogger, latestReport)
			s.syncPolicyStatus(ctx, latestReport)
		}
	}()
	latestBackendPolicyReportQueue := utils.NewAsyncQueue[reports.ReportMap]()
	s.backendPolicyReport.Register(func(o krt.Event[report]) {
		if o.Event == controllers.EventDelete {
			return
		}
		latestBackendPolicyReportQueue.Enqueue(o.Latest().reportMap)
	})
	go func() {
		for {
			latestReport, err := latestBackendPolicyReportQueue.Dequeue(ctx)
			if err != nil {
				return
			}
			s.syncPolicyStatus(ctx, latestReport)
		}
	}()

	for _, regFunc := range s.plugins.ContributesRegistration {
		if regFunc != nil {
			regFunc()
		}
	}

	s.perclientSnapCollection.RegisterBatch(func(o []krt.Event[XdsSnapWrapper]) {
		for _, e := range o {
			if e.Event != controllers.EventDelete {
				snapWrap := e.Latest()
				s.proxyTranslator.syncXds(ctx, snapWrap)
			} else {
				// key := e.Latest().proxyKey
				// if _, err := s.proxyTranslator.xdsCache.GetSnapshot(key); err == nil {
				// 	s.proxyTranslator.xdsCache.ClearSnapshot(e.Latest().proxyKey)
				// }
			}
		}
	}, true)

	s.ready.Store(true)
	<-ctx.Done()
	return nil
}

func (s *ProxySyncer) HasSynced() bool {
	return s.ready.Load()
}

func (s *ProxySyncer) syncRouteStatus(ctx context.Context, logger *slog.Logger, rm reports.ReportMap) {
	stopwatch := utils.NewTranslatorStopWatch("RouteStatusSyncer")
	stopwatch.Start()
	defer stopwatch.Stop(ctx)

	// Helper function to sync route status with retry
	syncStatusWithRetry := func(
		routeType string,
		routeKey client.ObjectKey,
		getRouteFunc func() client.Object,
		statusUpdater func(route client.Object) (*gwv1.RouteStatus, error),
	) error {
		return retry.Do(
			func() (rErr error) {
				route := getRouteFunc()

				err := s.mgr.GetClient().Get(ctx, routeKey, route)
				if err != nil {
					if apierrors.IsNotFound(err) {
						// the route is not found, we can't report status on it
						// if it's recreated, we'll retranslate it anyway
						return nil
					}
					logger.Error("error getting route", "error", err, "resource_ref", routeKey, "route_type", routeType)
					return err
				}

				gatewayNames := []string{}

				switch r := route.(type) {
				case *gwv1.HTTPRoute:
					for _, parentRef := range r.Spec.ParentRefs {
						gatewayNames = append(gatewayNames, string(parentRef.Name))
					}
				case *gwv1a2.TCPRoute:
					for _, parentRef := range r.Spec.ParentRefs {
						gatewayNames = append(gatewayNames, string(parentRef.Name))
					}
				case *gwv1a2.TLSRoute:
					for _, parentRef := range r.Spec.ParentRefs {
						gatewayNames = append(gatewayNames, string(parentRef.Name))
					}
				case *gwv1.GRPCRoute:
					for _, parentRef := range r.Spec.ParentRefs {
						gatewayNames = append(gatewayNames, string(parentRef.Name))
					}
				default:
					logger.Warn("unknown route type during status sync", "route_type",
						routeType, "resource_ref", client.ObjectKeyFromObject(route))
				}

				type finishMetricsErrors struct {
					finishFunc  func(error)
					statusError error
				}

				finishMetrics := make(map[string]finishMetricsErrors, len(gatewayNames))

				for _, gatewayName := range gatewayNames {
					finishMetrics[gatewayName] = finishMetricsErrors{
						finishFunc: collectStatusSyncMetrics(statusSyncMetricLabels{
							Name:      gatewayName,
							Namespace: routeKey.Namespace,
							Syncer:    "RouteStatusSyncer",
						}),
					}
				}

				defer func() {
					for _, gatewayName := range gatewayNames {
						tmetrics.EndResourceSync(tmetrics.ResourceSyncDetails{
							Namespace:    routeKey.Namespace,
							Gateway:      gatewayName,
							ResourceType: routeType,
							ResourceName: routeKey.Name,
						}, false, resourcesStatusSyncsCompletedTotal, resourcesStatusSyncDuration)

						if finish, exists := finishMetrics[gatewayName]; exists {
							finish.finishFunc(errors.Join(rErr, finish.statusError))
						}
					}
				}()

				if status, err := statusUpdater(route); err != nil {
					logger.Debug("error updating status for route", "error", err, "resource_ref", routeKey, "route_type", routeType)
					return err
				} else if status != nil {
					// Update metrics status if the conditions indicate an error.
					for _, ps := range status.Parents {
						for _, cond := range ps.Conditions {
							if cond.Type == string(gwv1.RouteConditionPartiallyInvalid) && cond.Status == metav1.ConditionTrue {
								if finish, exists := finishMetrics[string(ps.ParentRef.Name)]; exists {
									finishMetrics[string(ps.ParentRef.Name)] = finishMetricsErrors{
										finishFunc:  finish.finishFunc,
										statusError: fmt.Errorf("partially invalid route condition"),
									}

									break
								}
							}

							if cond.Type != string(gwv1.RouteConditionAccepted) {
								continue
							}

							if cond.Reason != string(gwv1.RouteReasonAccepted) &&
								cond.Reason != string(gwv1.RouteReasonPending) {
								if finish, exists := finishMetrics[string(ps.ParentRef.Name)]; exists {
									finishMetrics[string(ps.ParentRef.Name)] = finishMetricsErrors{
										finishFunc:  finish.finishFunc,
										statusError: fmt.Errorf("invalid route condition"),
									}

									break
								}
							}
						}
					}
				}

				return nil
			},
			retry.Attempts(5),
			retry.Delay(100*time.Millisecond),
			retry.DelayType(retry.BackOffDelay),
		)
	}

	// Helper function to build route status and update if needed
	buildAndUpdateStatus := func(route client.Object, routeType string) (*gwv1.RouteStatus, error) {
		var status *gwv1.RouteStatus
		switch r := route.(type) {
		case *gwv1.HTTPRoute:
			status = rm.BuildRouteStatus(ctx, r, s.controllerName)
			if status == nil || isRouteStatusEqual(&r.Status.RouteStatus, status) {
				return nil, nil
			}
			r.Status.RouteStatus = *status
		case *gwv1a2.TCPRoute:
			status = rm.BuildRouteStatus(ctx, r, s.controllerName)
			if status == nil || isRouteStatusEqual(&r.Status.RouteStatus, status) {
				return nil, nil
			}
			r.Status.RouteStatus = *status
		case *gwv1a2.TLSRoute:
			status = rm.BuildRouteStatus(ctx, r, s.controllerName)
			if status == nil || isRouteStatusEqual(&r.Status.RouteStatus, status) {
				return nil, nil
			}
			r.Status.RouteStatus = *status
		case *gwv1.GRPCRoute:
			status = rm.BuildRouteStatus(ctx, r, s.controllerName)
			if status == nil || isRouteStatusEqual(&r.Status.RouteStatus, status) {
				return nil, nil
			}
			r.Status.RouteStatus = *status
		default:
			logger.Warn("unsupported route type", "route_type", routeType, "resource_ref", client.ObjectKeyFromObject(route))
			return nil, nil
		}

		// Update the status
		return status, s.mgr.GetClient().Status().Update(ctx, route)
	}

	// Sync HTTPRoute statuses
	for rnn := range rm.HTTPRoutes {
		err := syncStatusWithRetry(
			wellknown.HTTPRouteKind,
			rnn,
			func() client.Object { return new(gwv1.HTTPRoute) },
			func(route client.Object) (*gwv1.RouteStatus, error) {
				return buildAndUpdateStatus(route, wellknown.HTTPRouteKind)
			},
		)
		if err != nil {
			logger.Error("all attempts failed at updating HTTPRoute status", "error", err, "route", rnn)
		}
	}

	// Sync TCPRoute statuses
	for rnn := range rm.TCPRoutes {
		err := syncStatusWithRetry(wellknown.TCPRouteKind, rnn,
			func() client.Object { return new(gwv1a2.TCPRoute) },
			func(route client.Object) (*gwv1.RouteStatus, error) {
				return buildAndUpdateStatus(route, wellknown.TCPRouteKind)
			})
		if err != nil {
			logger.Error("all attempts failed at updating TCPRoute status", "error", err, "route", rnn)
		}
	}

	// Sync TLSRoute statuses
	for rnn := range rm.TLSRoutes {
		err := syncStatusWithRetry(wellknown.TLSRouteKind, rnn,
			func() client.Object { return new(gwv1a2.TLSRoute) },
			func(route client.Object) (*gwv1.RouteStatus, error) {
				return buildAndUpdateStatus(route, wellknown.TLSRouteKind)
			})
		if err != nil {
			logger.Error("all attempts failed at updating TLSRoute status", "error", err, "route", rnn)
		}
	}

	// Sync GRPCRoute statuses
	for rnn := range rm.GRPCRoutes {
		err := syncStatusWithRetry(wellknown.GRPCRouteKind, rnn,
			func() client.Object { return new(gwv1.GRPCRoute) },
			func(route client.Object) (*gwv1.RouteStatus, error) {
				return buildAndUpdateStatus(route, wellknown.GRPCRouteKind)
			})
		if err != nil {
			logger.Error("all attempts failed at updating GRPCRoute status", "error", err, "route", rnn)
		}
	}
}

// syncGatewayStatus will build and update status for all Gateways in a reportMap
func (s *ProxySyncer) syncGatewayStatus(ctx context.Context, logger *slog.Logger, rm reports.ReportMap) {
	stopwatch := utils.NewTranslatorStopWatch("GatewayStatusSyncer")
	stopwatch.Start()

	for gwnn := range rm.Gateways {
		finishMetrics := collectStatusSyncMetrics(statusSyncMetricLabels{
			Name:      gwnn.Name,
			Namespace: gwnn.Namespace,
			Syncer:    "GatewayStatusSyncer",
		})

		var statusErr error

		err := utilretry.RetryOnConflict(utilretry.DefaultRetry, func() error {
			// Fetch the latest Gateway
			var gw gwv1.Gateway
			if err := s.mgr.GetClient().Get(ctx, gwnn, &gw); err != nil {
				logger.Info("error getting gateway", "error", err, "gateway", gwnn.String())
				return err
			}

			// Skip agentgateway classes, they are handled by agentgateway syncer
			if string(gw.Spec.GatewayClassName) == s.agentGatewayClassName {
				logger.Debug("skipping status sync for agentgateway", "gateway", gwnn.String())
			}

			// Build the desired status
			newStatus := rm.BuildGWStatus(ctx, gw)
			if newStatus == nil {
				return nil
			}

			// Skip if status hasn’t changed (ignoring Addresses)
			old := gw.Status
			old.Addresses = nil
			if isGatewayStatusEqual(&old, newStatus) {
				logger.Debug("gateway status is equal; skipping status update", "gateway", gwnn.String())
				return nil
			}

			// Prepare and apply the status patch
			original := gw.DeepCopy()
			gw.Status = *newStatus
			if err := s.mgr.GetClient().Status().Patch(ctx, &gw, client.MergeFrom(original)); err != nil {
				logger.Error("error patching gateway status", "error", err, "gateway", gwnn.String())
				return err
			}
			logger.Info("patched gateway status", "gateway", gwnn.String())

			for _, cond := range gw.Status.Conditions {
				if cond.Type != string(gwv1.GatewayConditionAccepted) &&
					cond.Type != string(gwv1.GatewayConditionProgrammed) {
					continue
				}

				if cond.Reason != string(gwv1.GatewayReasonAccepted) &&
					cond.Reason != string(gwv1.GatewayReasonProgrammed) &&
					cond.Reason != string(gwv1.GatewayReasonPending) {
					statusErr = fmt.Errorf("invalid gateway condition")

					break
				}
			}

			return nil
		})
		if err != nil {
			logger.Error("failed to update gateway status after retries", "error", err, "gateway", gwnn.String())
		}

		// Record metrics for this gateway
		tmetrics.EndResourceSync(tmetrics.ResourceSyncDetails{
			Namespace:    gwnn.Namespace,
			Gateway:      gwnn.Name,
			ResourceType: wellknown.GatewayKind,
			ResourceName: gwnn.Name,
		}, false, resourcesStatusSyncsCompletedTotal, resourcesStatusSyncDuration)

		finishMetrics(errors.Join(err, statusErr))
	}

	duration := stopwatch.Stop(ctx)
	logger.Debug("synced gw status for gateways", "count", len(rm.Gateways), "duration", duration)
}

// syncListenerSetStatus will build and update status for all Listener Sets in a reportMap
func (s *ProxySyncer) syncListenerSetStatus(ctx context.Context, logger *slog.Logger, rm reports.ReportMap) {
	stopwatch := utils.NewTranslatorStopWatch("ListenerSetStatusSyncer")
	stopwatch.Start()

	// TODO: retry within loop per LS rather than as a full block
	err := retry.Do(func() (rErr error) {
		for lsnn := range rm.ListenerSets {
			ls := gwxv1a1.XListenerSet{}
			err := s.mgr.GetClient().Get(ctx, lsnn, &ls)
			if err != nil {
				logger.Info("error getting ls", "erro", err.Error())
				return err
			}

			var statusErr error

			finishMetrics := collectStatusSyncMetrics(statusSyncMetricLabels{
				Name:      string(ls.Spec.ParentRef.Name),
				Namespace: lsnn.Namespace,
				Syncer:    "ListenerSetStatusSyncer",
			})
			defer func() {
				finishMetrics(errors.Join(rErr, statusErr))
			}()

			lsStatus := ls.Status
			if status := rm.BuildListenerSetStatus(ctx, ls); status != nil {
				if !isListenerSetStatusEqual(&lsStatus, status) {
					ls.Status = *status
					if err := s.mgr.GetClient().Status().Patch(ctx, &ls, client.Merge); err != nil {
						if apierrors.IsConflict(err) {
							return err // Expected conflict, retry will handle.
						}
						logger.Error("error patching listener set status", "error", err, "gateway", lsnn.String())
						return err
					}
					logger.Info("patched ls status", "listenerset", lsnn.String())

					for _, cond := range status.Conditions {
						if cond.Type != string(gwxv1a1.ListenerSetConditionAccepted) &&
							cond.Type != string(gwxv1a1.ListenerSetConditionProgrammed) {
							continue
						}

						if cond.Reason != string(gwxv1a1.ListenerSetReasonAccepted) &&
							cond.Reason != string(gwxv1a1.ListenerSetReasonProgrammed) &&
							cond.Reason != string(gwxv1a1.ListenerSetReasonPending) {
							statusErr = fmt.Errorf("invalid listener condition")

							break
						}
					}
				} else {
					logger.Debug("skipping k8s ls status update, status equal", "listenerset", lsnn.String())
				}

				tmetrics.EndResourceSync(tmetrics.ResourceSyncDetails{
					Namespace:    ls.Namespace,
					Gateway:      string(ls.Spec.ParentRef.Name),
					ResourceType: "XListenerSet",
					ResourceName: ls.Name,
				}, false, resourcesStatusSyncsCompletedTotal, resourcesStatusSyncDuration)
			}
		}
		return nil
	},
		retry.Attempts(5),
		retry.Delay(100*time.Millisecond),
		retry.DelayType(retry.BackOffDelay),
	)
	if err != nil {
		logger.Error("all attempts failed at updating listener set statuses", "error", err)
	}
	duration := stopwatch.Stop(ctx)
	logger.Debug("synced listener sets status for listener set", "count", len(rm.ListenerSets), "duration", duration.String())
}

func (s *ProxySyncer) syncPolicyStatus(ctx context.Context, rm reports.ReportMap) {
	stopwatch := utils.NewTranslatorStopWatch("RouteStatusSyncer")
	stopwatch.Start()
	defer stopwatch.Stop(ctx)

	// Sync Policy statuses
	for key := range rm.Policies {
		gk := schema.GroupKind{Group: key.Group, Kind: key.Kind}
		nsName := types.NamespacedName{Namespace: key.Namespace, Name: key.Name}

		plugin, ok := s.plugins.ContributesPolicies[gk]
		if !ok {
			logger.Error("policy plugin not registered for policy", "group_kind", gk, "resource", nsName)
			continue
		}
		if plugin.GetPolicyStatus == nil {
			logger.Error("GetPolicyStatus handler not registered for policy", "group_kind", gk, "resource", nsName) //nolint:sloglint // ignore PascalCase at start
			continue
		}
		if plugin.PatchPolicyStatus == nil {
			logger.Error("PatchPolicyStatus handler not registered for policy", "group_kind", gk, "resource", nsName) //nolint:sloglint // ignore PascalCase at start
			continue
		}
		currentStatus, err := plugin.GetPolicyStatus(ctx, nsName)
		if err != nil {
			logger.Error("error getting policy status", "error", err, "resource_ref", nsName)
			continue
		}
		status := rm.BuildPolicyStatus(ctx, key, s.controllerName, currentStatus)
		if status == nil {
			continue
		}

		var statusErr error

		for _, ancestor := range status.Ancestors {
			for _, cond := range ancestor.Conditions {
				if cond.Type != string(v1alpha1.PolicyConditionAccepted) {
					continue
				}

				if cond.Reason != string(v1alpha1.PolicyReasonValid) &&
					cond.Reason != string(v1alpha1.PolicyReasonPending) {
					statusErr = fmt.Errorf("invalid policy condition")

					break
				}
			}

			if statusErr != nil {
				break
			}
		}

		finishMetrics := collectStatusSyncMetrics(statusSyncMetricLabels{
			Name:      gk.Kind,
			Namespace: nsName.Namespace,
			Syncer:    "PolicyStatusSyncer",
		})

		err = retry.Do(
			func() error {
				return plugin.PatchPolicyStatus(ctx, nsName, *status)
			},
			retry.Attempts(5),
			retry.Delay(100*time.Millisecond),
			retry.DelayType(retry.BackOffDelay),
		)
		if err != nil {
			logger.Error("error updating policy status", "error", err, "group_kind", gk, "resource_ref", nsName)
			finishMetrics(errors.Join(err, statusErr))
			continue
		}

		for _, ancestor := range status.Ancestors {
			if ancestor.AncestorRef.Kind != nil && *ancestor.AncestorRef.Kind == "Gateway" {
				namespace := nsName.Namespace
				if ancestor.AncestorRef.Namespace != nil {
					namespace = string(*ancestor.AncestorRef.Namespace)
				}

				tmetrics.EndResourceSync(tmetrics.ResourceSyncDetails{
					Namespace:    namespace,
					Gateway:      string(ancestor.AncestorRef.Name),
					ResourceType: gk.Kind,
					ResourceName: nsName.Name,
				}, false, resourcesStatusSyncsCompletedTotal, resourcesStatusSyncDuration)
			}
		}

		finishMetrics(statusErr)
	}
}

var opts = cmp.Options{
	cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"),
	cmpopts.IgnoreMapEntries(func(k string, _ any) bool {
		return k == "lastTransitionTime"
	}),
}

func isGatewayStatusEqual(objA, objB *gwv1.GatewayStatus) bool {
	return cmp.Equal(objA, objB, opts)
}

func isListenerSetStatusEqual(objA, objB *gwxv1a1.ListenerSetStatus) bool {
	return cmp.Equal(objA, objB, opts)
}

// isRouteStatusEqual compares two RouteStatus objects directly
func isRouteStatusEqual(objA, objB *gwv1.RouteStatus) bool {
	return cmp.Equal(objA, objB, opts)
}

type resourcesStringer envoycache.Resources

func (r resourcesStringer) String() string {
	return fmt.Sprintf("len: %d, version %s", len(r.Items), r.Version)
}
