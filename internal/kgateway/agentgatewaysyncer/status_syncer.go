package agentgatewaysyncer

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/avast/retry-go/v4"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"istio.io/istio/pkg/kube"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

var _ manager.LeaderElectionRunnable = &AgentGwStatusSyncer{}

// AgentGwStatusSyncer runs only on the leader and syncs the status of agent gateway resources.
// It subscribes to the report queues, parses and updates the resource status.
type AgentGwStatusSyncer struct {
	// Core collections and dependencies
	mgr    manager.Manager
	client kube.Client

	// Configuration
	controllerName        string
	agentGatewayClassName string

	// Report queues
	gatewayReportQueue     utils.AsyncQueue[GatewayReports]
	listenerSetReportQueue utils.AsyncQueue[ListenerSetReports]
	routeReportQueue       utils.AsyncQueue[RouteReports]

	// Synchronization
	cacheSyncs []cache.InformerSynced
}

func NewAgentGwStatusSyncer(
	controllerName string,
	agentGatewayClassName string,
	client kube.Client,
	mgr manager.Manager,
	gatewayReportQueue utils.AsyncQueue[GatewayReports],
	listenerSetReportQueue utils.AsyncQueue[ListenerSetReports],
	routeReportQueue utils.AsyncQueue[RouteReports],
	cacheSyncs []cache.InformerSynced,
) *AgentGwStatusSyncer {
	return &AgentGwStatusSyncer{
		controllerName:         controllerName,
		agentGatewayClassName:  agentGatewayClassName,
		client:                 client,
		mgr:                    mgr,
		gatewayReportQueue:     gatewayReportQueue,
		listenerSetReportQueue: listenerSetReportQueue,
		routeReportQueue:       routeReportQueue,
		cacheSyncs:             cacheSyncs,
	}
}

func (s *AgentGwStatusSyncer) Start(ctx context.Context) error {
	logger.Info("starting agentgateway Status Syncer", "controllername", s.controllerName)
	logger.Info("waiting for agentgateway cache to sync")

	// wait for krt collections to sync
	logger.Info("waiting for cache to sync")
	s.client.WaitForCacheSync(
		"agent gateway status syncer",
		ctx.Done(),
		s.cacheSyncs...,
	)

	// wait for ctrl-rtime caches to sync before accepting events
	if !s.mgr.GetCache().WaitForCacheSync(ctx) {
		return fmt.Errorf("agent gateway status sync loop waiting for all caches to sync failed")
	}
	logger.Info("caches warm!")

	// Start separate goroutines for each status syncer
	routeStatusLogger := logger.With("subcomponent", "routeStatusSyncer")
	listenerSetStatusLogger := logger.With("subcomponent", "listenerSetStatusSyncer")
	gatewayStatusLogger := logger.With("subcomponent", "gatewayStatusSyncer")

	// Gateway status syncer
	go func() {
		for {
			gatewayReports, err := s.gatewayReportQueue.Dequeue(ctx)
			if err != nil {
				logger.Error("failed to dequeue gateway reports", "error", err)
				return
			}
			s.syncGatewayStatus(ctx, gatewayStatusLogger, gatewayReports)
		}
	}()

	// Listener set status syncer
	go func() {
		for {
			listenerSetReports, err := s.listenerSetReportQueue.Dequeue(ctx)
			if err != nil {
				logger.Error("failed to dequeue listener set reports", "error", err)
				return
			}
			s.syncListenerSetStatus(ctx, listenerSetStatusLogger, listenerSetReports)
		}
	}()

	// Route status syncer
	go func() {
		for {
			routeReports, err := s.routeReportQueue.Dequeue(ctx)
			if err != nil {
				logger.Error("failed to dequeue route reports", "error", err)
				return
			}
			s.syncRouteStatus(ctx, routeStatusLogger, routeReports)
		}
	}()

	<-ctx.Done()
	return nil
}

func (s *AgentGwStatusSyncer) syncRouteStatus(ctx context.Context, logger *slog.Logger, routeReports RouteReports) {
	stopwatch := utils.NewTranslatorStopWatch("RouteStatusSyncer")
	stopwatch.Start()
	defer stopwatch.Stop(ctx)

	// TODO: add routeStatusMetrics

	// Helper function to sync route status with retry
	syncStatusWithRetry := func(
		routeType string,
		routeKey client.ObjectKey,
		getRouteFunc func() client.Object,
		statusUpdater func(route client.Object) error,
	) error {
		return retry.Do(
			func() error {
				route := getRouteFunc()
				err := s.mgr.GetClient().Get(ctx, routeKey, route)
				if err != nil {
					if apierrors.IsNotFound(err) {
						// the route is not found, we can't report status on it
						// if it's recreated, we'll retranslate it anyway
						return nil
					}
					logger.Error("error getting route", logKeyError, err, logKeyResourceRef, routeKey, logKeyRouteType, routeType)
					return err
				}
				if err := statusUpdater(route); err != nil {
					logger.Debug("error updating status for route", logKeyError, err, logKeyResourceRef, routeKey, logKeyRouteType, routeType)
					return err
				}
				return nil
			},
			retry.Attempts(maxRetryAttempts),
			retry.Delay(retryDelay),
			retry.DelayType(retry.BackOffDelay),
		)
	}

	// Create a minimal ReportMap with just the route reports for BuildRouteStatus to work
	rm := reports.ReportMap{
		HTTPRoutes: routeReports.HTTPRoutes,
		GRPCRoutes: routeReports.GRPCRoutes,
		TCPRoutes:  routeReports.TCPRoutes,
		TLSRoutes:  routeReports.TLSRoutes,
	}

	// Helper function to build route status and update if needed
	buildAndUpdateStatus := func(route client.Object, routeType string) error {
		var status *gwv1.RouteStatus
		switch r := route.(type) {
		case *gwv1.HTTPRoute: // TODO: beta1?
			status = rm.BuildRouteStatus(ctx, r, s.controllerName)
			if status == nil || isRouteStatusEqual(&r.Status.RouteStatus, status) {
				return nil
			}
			r.Status.RouteStatus = *status
		case *gwv1alpha2.TCPRoute:
			status = rm.BuildRouteStatus(ctx, r, s.controllerName)
			if status == nil || isRouteStatusEqual(&r.Status.RouteStatus, status) {
				return nil
			}
			r.Status.RouteStatus = *status
		case *gwv1alpha2.TLSRoute:
			status = rm.BuildRouteStatus(ctx, r, s.controllerName)
			if status == nil || isRouteStatusEqual(&r.Status.RouteStatus, status) {
				return nil
			}
			r.Status.RouteStatus = *status
		case *gwv1.GRPCRoute:
			status = rm.BuildRouteStatus(ctx, r, s.controllerName)
			if status == nil || isRouteStatusEqual(&r.Status.RouteStatus, status) {
				return nil
			}
			r.Status.RouteStatus = *status
		default:
			logger.Warn("unsupported route type", logKeyRouteType, routeType, logKeyResourceRef, client.ObjectKeyFromObject(route))
			return nil
		}

		// Update the status
		return s.mgr.GetClient().Status().Update(ctx, route)
	}

	for rnn := range routeReports.HTTPRoutes {
		err := syncStatusWithRetry(
			wellknown.HTTPRouteKind,
			rnn,
			func() client.Object {
				return new(gwv1.HTTPRoute)
			},
			func(route client.Object) error {
				return buildAndUpdateStatus(route, wellknown.HTTPRouteKind)
			},
		)
		if err != nil {
			logger.Error("all attempts failed at updating HTTPRoute status", logKeyError, err, "route", rnn)
		}
	}
}

// syncGatewayStatus will build and update status for all Gateways in gateway reports
func (s *AgentGwStatusSyncer) syncGatewayStatus(ctx context.Context, logger *slog.Logger, gatewayReports GatewayReports) {
	stopwatch := utils.NewTranslatorStopWatch("GatewayStatusSyncer")
	stopwatch.Start()

	// TODO: add gatewayStatusMetrics

	// Create a minimal ReportMap with just the gateway reports for BuildGWStatus to work
	rm := reports.ReportMap{
		Gateways: gatewayReports.Reports,
	}

	// TODO: retry within loop per GW rather that as a full block
	err := retry.Do(func() error {
		for gwnn := range gatewayReports.Reports {
			gw := gwv1.Gateway{}
			err := s.mgr.GetClient().Get(ctx, gwnn, &gw)
			if err != nil {
				if apierrors.IsNotFound(err) {
					// the gateway is not found, we can't report status on it
					// if it's recreated, we'll retranslate it anyway
					continue
				}
				logger.Info("error getting gw", logKeyError, err, logKeyGateway, gwnn.String())
				return err
			}

			// Only process agentgateway classes - others are handled by ProxySyncer
			if string(gw.Spec.GatewayClassName) != s.agentGatewayClassName {
				logger.Debug("skipping status sync for non-agentgateway", logKeyGateway, gwnn.String())
				continue
			}

			gwStatusWithoutAddress := gw.Status
			gwStatusWithoutAddress.Addresses = nil
			var attachedRoutesForGw map[string]uint
			if gatewayReports.AttachedRoutes != nil {
				attachedRoutesForGw = gatewayReports.AttachedRoutes[gwnn]
			}

			if status := rm.BuildGWStatus(ctx, gw, attachedRoutesForGw); status != nil {
				if !isGatewayStatusEqual(&gwStatusWithoutAddress, status) {
					gw.Status = *status
					if err := s.mgr.GetClient().Status().Patch(ctx, &gw, client.Merge); err != nil {
						logger.Error("error patching gateway status", logKeyError, err, logKeyGateway, gwnn.String())
						return err
					}
					logger.Info("patched gw status", logKeyGateway, gwnn.String())
				} else {
					logger.Info("skipping k8s gateway status update, status equal", logKeyGateway, gwnn.String())
				}
			}
		}
		return nil
	},
		retry.Attempts(maxRetryAttempts),
		retry.Delay(retryDelay),
		retry.DelayType(retry.BackOffDelay),
	)
	if err != nil {
		logger.Error("all attempts failed at updating gateway statuses", logKeyError, err)
	}
	duration := stopwatch.Stop(ctx)
	logger.Debug("synced gw status for gateways", "count", len(gatewayReports.Reports), "duration", duration)
}

// syncListenerSetStatus will build and update status for all Listener Sets in listener set reports
func (s *AgentGwStatusSyncer) syncListenerSetStatus(ctx context.Context, logger *slog.Logger, listenerSetReports ListenerSetReports) {
	stopwatch := utils.NewTranslatorStopWatch("ListenerSetStatusSyncer")
	stopwatch.Start()

	// TODO: add listenerStatusMetrics

	// Create a minimal ReportMap with just the listener set reports for BuildListenerSetStatus to work
	rm := reports.ReportMap{
		ListenerSets: listenerSetReports.Reports,
	}

	// TODO: retry within loop per LS rathen that as a full block
	err := retry.Do(func() error {
		for lsnn := range listenerSetReports.Reports {
			ls := gwxv1a1.XListenerSet{}
			err := s.mgr.GetClient().Get(ctx, lsnn, &ls)
			if err != nil {
				if apierrors.IsNotFound(err) {
					// the listener set is not found, we can't report status on it
					// if it's recreated, we'll retranslate it anyway
					continue
				}
				logger.Info("error getting ls", "error", err.Error())
				return err
			}
			lsStatus := ls.Status
			if status := rm.BuildListenerSetStatus(ctx, ls); status != nil {
				if !isListenerSetStatusEqual(&lsStatus, status) {
					ls.Status = *status
					if err := s.mgr.GetClient().Status().Patch(ctx, &ls, client.Merge); err != nil {
						logger.Error("error patching listener set status", logKeyError, err, logKeyGateway, lsnn.String())
						return err
					}
					logger.Info("patched ls status", "listenerset", lsnn.String())
				} else {
					logger.Info("skipping k8s ls status update, status equal", "listenerset", lsnn.String())
				}
			}
		}
		return nil
	},
		retry.Attempts(maxRetryAttempts),
		retry.Delay(retryDelay),
		retry.DelayType(retry.BackOffDelay),
	)
	if err != nil {
		logger.Error("all attempts failed at updating listener set statuses", logKeyError, err)
	}
	duration := stopwatch.Stop(ctx)
	logger.Debug("synced listener sets status for listener set", "count", len(listenerSetReports.Reports), "duration", duration.String())
}

// NeedLeaderElection returns true to ensure that the AgentGwStatusSyncer runs only on the leader
func (r *AgentGwStatusSyncer) NeedLeaderElection() bool {
	return true
}

var opts = cmp.Options{
	cmpopts.IgnoreFields(metav1.Condition{}, "LastTransitionTime"),
	cmpopts.IgnoreMapEntries(func(k string, _ any) bool {
		return k == "lastTransitionTime"
	}),
}

// isRouteStatusEqual compares two RouteStatus objects directly
func isRouteStatusEqual(objA, objB *gwv1.RouteStatus) bool {
	return cmp.Equal(objA, objB, opts)
}

func isListenerSetStatusEqual(objA, objB *gwxv1a1.ListenerSetStatus) bool {
	return cmp.Equal(objA, objB, opts)
}

func isGatewayStatusEqual(objA, objB *gwv1.GatewayStatus) bool {
	return cmp.Equal(objA, objB, opts)
}
