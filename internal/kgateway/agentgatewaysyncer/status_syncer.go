package agentgatewaysyncer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"istio.io/istio/pilot/pkg/status"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

var _ manager.LeaderElectionRunnable = &AgentGwStatusSyncer{}

// policyStatusQueue implements status.Queue interface for Istio's StatusCollections
type policyStatusQueue struct {
	asyncQueue *PolicyStatusAsyncQueue
}

func (q *policyStatusQueue) EnqueueStatusUpdateResource(context any, resource status.Resource) {
	// Convert the context back to our expected type
	if obj, ok := context.(krt.ObjectWithStatus[controllers.Object, gwv1alpha2.PolicyStatus]); ok {
		q.asyncQueue.Enqueue(obj)
	}
}

const (
	// Retry configuration constants for status updates
	maxRetryAttempts = 5
	retryDelay       = 100 * time.Millisecond

	// Log message keys
	logKeyError = "error"
)

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
	gatewayReportQueue      utils.AsyncQueue[GatewayReports]
	listenerSetReportQueue  utils.AsyncQueue[ListenerSetReports]
	routeReportQueue        utils.AsyncQueue[RouteReports]
	policyStatusQueue       utils.AsyncQueue[krt.ObjectWithStatus[controllers.Object, gwv1alpha2.PolicyStatus]]
	policyStatusCollections *status.StatusCollections

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
	policyStatusCollections *status.StatusCollections,
	cacheSyncs []cache.InformerSynced,
) *AgentGwStatusSyncer {
	return &AgentGwStatusSyncer{
		controllerName:          controllerName,
		agentGatewayClassName:   agentGatewayClassName,
		client:                  client,
		mgr:                     mgr,
		gatewayReportQueue:      gatewayReportQueue,
		listenerSetReportQueue:  listenerSetReportQueue,
		routeReportQueue:        routeReportQueue,
		policyStatusCollections: policyStatusCollections,
		cacheSyncs:              cacheSyncs,
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

	// Initialize policy status queue from collections
	psq := NewPolicyStatusAsyncQueue()
	s.policyStatusQueue = psq.GetAsyncQueue()
	// Create a controllers.Queue that wraps our async queue for Istio's StatusCollections
	// The policyStatusQueue implements https://github.com/istio/istio/blob/531c61709aaa9bc9187c625e9e460be98f2abf2e/pilot/pkg/status/manager.go#L107
	polStatusQueue := &policyStatusQueue{asyncQueue: psq}
	s.policyStatusCollections.SetQueue(polStatusQueue)

	// Start separate goroutines for each status syncer
	routeStatusLogger := logger.With("subcomponent", "routeStatusSyncer")
	listenerSetStatusLogger := logger.With("subcomponent", "listenerSetStatusSyncer")
	gatewayStatusLogger := logger.With("subcomponent", "gatewayStatusSyncer")
	policyStatusLogger := logger.With("subcomponent", "policyStatusSyncer")

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

	// TrafficPolicy status syncer
	go func() {
		for {
			policyStatusUpdate, err := s.policyStatusQueue.Dequeue(ctx)
			if err != nil {
				logger.Error("failed to dequeue trafficpolicy status", "error", err)
				return
			}
			s.syncTrafficPolicyStatus(ctx, policyStatusLogger, policyStatusUpdate)
		}
	}()

	<-ctx.Done()
	return nil
}

func (s *AgentGwStatusSyncer) syncTrafficPolicyStatus(ctx context.Context, logger *slog.Logger, policyStatusUpdate krt.ObjectWithStatus[controllers.Object, gwv1alpha2.PolicyStatus]) {
	stopwatch := utils.NewTranslatorStopWatch("PolicyStatusSyncer")
	stopwatch.Start()
	defer stopwatch.Stop(ctx)

	policyNameNs := types.NamespacedName{
		Namespace: policyStatusUpdate.Obj.GetNamespace(),
		Name:      policyStatusUpdate.Obj.GetName(),
	}

	err := retry.Do(func() error {
		trafficpolicy := v1alpha1.TrafficPolicy{}
		err := s.mgr.GetClient().Get(ctx, policyNameNs, &trafficpolicy)
		if err != nil {
			if apierrors.IsNotFound(err) {
				logger.Debug("policy not found, skipping status update", "trafficpolicy", policyNameNs.String())
				return nil
			}
			logger.Error("error getting trafficpolicy", logKeyError, err, "trafficpolicy", policyNameNs.String())
			return err
		}

		// Update the trafficpolicy status directly
		var ancestors []gwv1alpha2.PolicyAncestorStatus
		for _, ancestor := range policyStatusUpdate.Status.Ancestors {
			ancestors = append(ancestors, gwv1alpha2.PolicyAncestorStatus{
				AncestorRef:    ancestor.AncestorRef,
				ControllerName: gwv1.GatewayController(ancestor.ControllerName),
				Conditions:     ancestor.Conditions,
			})
		}
		trafficpolicy.Status = gwv1alpha2.PolicyStatus{
			Ancestors: ancestors,
		}
		err = s.mgr.GetClient().Status().Update(ctx, &trafficpolicy)
		if err != nil {
			logger.Error("error updating trafficpolicy status", logKeyError, err, "trafficpolicy", policyNameNs.String())
			return err
		}
		logger.Debug("updated trafficpolicy status", "trafficpolicy", policyNameNs.String(), "status", trafficpolicy.Status)
		return nil
	}, retry.Attempts(maxRetryAttempts), retry.Delay(retryDelay))

	if err != nil {
		logger.Error("failed to sync trafficpolicy status after retries", logKeyError, err, "trafficpolicy", policyNameNs.String())
	}
}

func (s *AgentGwStatusSyncer) syncRouteStatus(ctx context.Context, logger *slog.Logger, routeReports RouteReports) {
	stopwatch := utils.NewTranslatorStopWatch("RouteStatusSyncer")
	stopwatch.Start()
	defer stopwatch.Stop(ctx)
	startTime := time.Now()

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
						if time.Since(startTime) < 5*time.Second {
							return err // Retry
						}
						// After timeout, assume genuinely deleted
						logger.Error("route not found after timeout, skipping", logKeyResourceRef, routeKey)
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
		// Get parentRefs based on route type and ensure namespaces are set
		var parentRefs []gwv1.ParentReference
		switch r := route.(type) {
		case *gwv1.HTTPRoute:
			parentRefs = r.Spec.ParentRefs
		case *gwv1alpha2.TCPRoute:
			parentRefs = r.Spec.ParentRefs
		case *gwv1alpha2.TLSRoute:
			parentRefs = r.Spec.ParentRefs
		case *gwv1.GRPCRoute:
			parentRefs = r.Spec.ParentRefs
		default:
			logger.Warn("unsupported route type", logKeyRouteType, routeType, logKeyResourceRef, client.ObjectKeyFromObject(route))
			return nil
		}

		// Common processing for all route types
		ensureParentRefNamespaces(parentRefs, route.GetNamespace())
		status := rm.BuildRouteStatus(ctx, route, s.controllerName)
		if status == nil {
			return nil
		}

		// Get existing status based on route type
		var existingStatus *gwv1.RouteStatus
		switch r := route.(type) {
		case *gwv1.HTTPRoute:
			existingStatus = &r.Status.RouteStatus
		case *gwv1alpha2.TCPRoute:
			existingStatus = &r.Status.RouteStatus
		case *gwv1alpha2.TLSRoute:
			existingStatus = &r.Status.RouteStatus
		case *gwv1.GRPCRoute:
			existingStatus = &r.Status.RouteStatus
		}

		if isRouteStatusEqual(existingStatus, status) {
			return nil
		}

		// Update status based on route type
		switch r := route.(type) {
		case *gwv1.HTTPRoute:
			r.Status.RouteStatus = *status
		case *gwv1alpha2.TCPRoute:
			r.Status.RouteStatus = *status
		case *gwv1alpha2.TLSRoute:
			r.Status.RouteStatus = *status
		case *gwv1.GRPCRoute:
			r.Status.RouteStatus = *status
		}

		// Update the status
		return s.mgr.GetClient().Status().Update(ctx, route)
	}

	// Process HTTPRoutes
	for rnn := range routeReports.HTTPRoutes {
		err := syncStatusWithRetry(
			wellknown.HTTPRouteKind,
			rnn,
			func() client.Object { return new(gwv1.HTTPRoute) },
			func(route client.Object) error {
				return buildAndUpdateStatus(route, wellknown.HTTPRouteKind)
			},
		)
		if err != nil {
			logger.Error("all attempts failed at updating HTTPRoute status", logKeyError, err, "route", rnn)
		}
	}

	// Process GRPCRoutes
	for rnn := range routeReports.GRPCRoutes {
		err := syncStatusWithRetry(
			wellknown.GRPCRouteKind,
			rnn,
			func() client.Object { return new(gwv1.GRPCRoute) },
			func(route client.Object) error {
				return buildAndUpdateStatus(route, wellknown.GRPCRouteKind)
			},
		)
		if err != nil {
			logger.Error("all attempts failed at updating GRPCRoute status", logKeyError, err, "route", rnn)
		}
	}

	// Process TCPRoutes
	for rnn := range routeReports.TCPRoutes {
		err := syncStatusWithRetry(
			wellknown.TCPRouteKind,
			rnn,
			func() client.Object { return new(gwv1alpha2.TCPRoute) },
			func(route client.Object) error {
				return buildAndUpdateStatus(route, wellknown.TCPRouteKind)
			},
		)
		if err != nil {
			logger.Error("all attempts failed at updating TCPRoute status", logKeyError, err, "route", rnn)
		}
	}

	// Process TLSRoutes
	for rnn := range routeReports.TLSRoutes {
		err := syncStatusWithRetry(
			wellknown.TLSRouteKind,
			rnn,
			func() client.Object { return new(gwv1alpha2.TLSRoute) },
			func(route client.Object) error {
				return buildAndUpdateStatus(route, wellknown.TLSRouteKind)
			},
		)
		if err != nil {
			logger.Error("all attempts failed at updating TLSRoute status", logKeyError, err, "route", rnn)
		}
	}
}

// ensureBasicGatewayConditions ensures that the required Gateway conditions exist
// This is needed because agent-gateway bypasses normal reporter initialization
func ensureBasicGatewayConditions(status *gwv1.GatewayStatus, generation int64) {
	if status == nil {
		return
	}

	// Ensure Accepted condition exists
	if meta.FindStatusCondition(status.Conditions, string(gwv1.GatewayConditionAccepted)) == nil {
		meta.SetStatusCondition(&status.Conditions, metav1.Condition{
			Type:               string(gwv1.GatewayConditionAccepted),
			Status:             metav1.ConditionTrue,
			Reason:             string(gwv1.GatewayReasonAccepted),
			Message:            "Gateway is accepted by agent-gateway controller",
			ObservedGeneration: generation,
		})
	}

	// Ensure Programmed condition exists
	if meta.FindStatusCondition(status.Conditions, string(gwv1.GatewayConditionProgrammed)) == nil {
		meta.SetStatusCondition(&status.Conditions, metav1.Condition{
			Type:               string(gwv1.GatewayConditionProgrammed),
			Status:             metav1.ConditionTrue,
			Reason:             string(gwv1.GatewayReasonProgrammed),
			Message:            "Gateway is programmed by agent-gateway controller",
			ObservedGeneration: generation,
		})
	}

	// Ensure all existing conditions have the correct observedGeneration
	for i := range status.Conditions {
		status.Conditions[i].ObservedGeneration = generation
	}

	// Ensure all listener conditions have the correct observedGeneration
	for i := range status.Listeners {
		for j := range status.Listeners[i].Conditions {
			status.Listeners[i].Conditions[j].ObservedGeneration = generation
		}
	}
}

// syncGatewayStatus will build and update status for all Gateways in gateway reports
func (s *AgentGwStatusSyncer) syncGatewayStatus(ctx context.Context, logger *slog.Logger, gatewayReports GatewayReports) {
	stopwatch := utils.NewTranslatorStopWatch("GatewayStatusSyncer")
	stopwatch.Start()
	startTime := time.Now()
	// Create a minimal ReportMap with just the gateway reports for BuildGWStatus to work
	rm := reports.ReportMap{
		Gateways: gatewayReports.Reports,
	}

	err := retry.Do(func() error {
		for gwnn := range gatewayReports.Reports {
			gw := gwv1.Gateway{}
			err := s.mgr.GetClient().Get(ctx, gwnn, &gw)
			if err != nil {
				if apierrors.IsNotFound(err) {
					if time.Since(startTime) < 5*time.Second {
						return err // Retry
					}
					// After timeout, assume genuinely deleted
					logger.Error("gateway not found after timeout, skipping", logKeyGateway, gwnn.String())
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
			gwReporter := rm.Gateway(&gw)
			for _, listener := range gw.Spec.Listeners {
				supportedKinds := calculateSupportedKinds(listener)
				gwReporter.Listener(&listener).SetSupportedKinds(supportedKinds)
			}

			if status := rm.BuildGWStatus(ctx, gw, attachedRoutesForGw); status != nil {
				// Ensure basic Gateway conditions exist (agent-gateway bypasses normal reporter init)
				ensureBasicGatewayConditions(status, gw.Generation)

				// normalize per-listener AttachedRoutes, defaulting to 0 where absent.
				normalizeListenerAttachedRoutes(&gw, status, attachedRoutesForGw)
				setObservedGen(&gw, status)

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

// normalizeListenerAttachedRoutes ensures every spec listener has a ListenerStatus entry
// and that AttachedRoutes reflects the provided counts (defaulting to 0).
func normalizeListenerAttachedRoutes(gw *gwv1.Gateway, st *gwv1.GatewayStatus, counts map[string]uint) {
	// Index existing listener statuses by name.
	idx := make(map[string]int, len(st.Listeners))
	for i := range st.Listeners {
		idx[string(st.Listeners[i].Name)] = i
	}
	// Ensure each spec listener exists and set the count (0 if missing in counts).
	for _, lis := range gw.Spec.Listeners {
		name := string(lis.Name)
		c := uint(0)
		if counts != nil {
			c = counts[name]
		}

		if j, ok := idx[name]; ok {
			// Preserve any existing conditions, just set the count.
			st.Listeners[j].AttachedRoutes = int32(c)
		} else {
			// Create a minimal status for the listener with the correct count.
			st.Listeners = append(st.Listeners, gwv1.ListenerStatus{
				Name:           lis.Name,
				AttachedRoutes: int32(c),
			})
			idx[name] = len(st.Listeners) - 1
		}
	}

	// Keep deterministic order: match spec.Listeners order.
	ordered := make([]gwv1.ListenerStatus, 0, len(gw.Spec.Listeners))
	for _, lis := range gw.Spec.Listeners {
		if j, ok := idx[string(lis.Name)]; ok {
			ordered = append(ordered, st.Listeners[j])
		}
	}
	st.Listeners = ordered
}

// syncListenerSetStatus will build and update status for all Listener Sets in listener set reports
func (s *AgentGwStatusSyncer) syncListenerSetStatus(ctx context.Context, logger *slog.Logger, listenerSetReports ListenerSetReports) {
	stopwatch := utils.NewTranslatorStopWatch("ListenerSetStatusSyncer")
	stopwatch.Start()
	startTime := time.Now()

	// TODO: add listenerStatusMetrics

	// Create a minimal ReportMap with just the listener set reports for BuildListenerSetStatus to work
	rm := reports.ReportMap{
		ListenerSets: listenerSetReports.Reports,
	}

	// TODO: retry within loop per LS rather than as a full block
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
				if apierrors.IsNotFound(err) {
					if time.Since(startTime) < 5*time.Second {
						return err // Retry
					}
					// After timeout, assume genuinely deleted
					logger.Error("listener set not found after timeout, skipping", "listenerset", lsnn.String())
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

// setObservedGen stamps ObservedGeneration on all gateway + listener conditions.
func setObservedGen(gw *gwv1.Gateway, st *gwv1.GatewayStatus) {
	if st == nil {
		return
	}
	for i := range st.Conditions {
		st.Conditions[i].ObservedGeneration = gw.Generation
	}
	for li := range st.Listeners {
		for ci := range st.Listeners[li].Conditions {
			st.Listeners[li].Conditions[ci].ObservedGeneration = gw.Generation
		}
	}
}

func calculateSupportedKinds(listener gwv1.Listener) []gwv1.RouteGroupKind {
	gatewayGroup := gwv1.Group("gateway.networking.k8s.io")
	allSupportedKinds := []gwv1.RouteGroupKind{
		{Group: &gatewayGroup, Kind: "HTTPRoute"},
		{Group: &gatewayGroup, Kind: "GRPCRoute"},
		{Group: &gatewayGroup, Kind: "TCPRoute"},
		{Group: &gatewayGroup, Kind: "TLSRoute"},
	}

	if listener.AllowedRoutes == nil || len(listener.AllowedRoutes.Kinds) == 0 {
		return allSupportedKinds
	}

	// Initialize with empty slice, not nil - Kubernetes API requires non-nil slice
	supportedKinds := make([]gwv1.RouteGroupKind, 0)

	for _, allowedKind := range listener.AllowedRoutes.Kinds {
		for _, supportedKind := range allSupportedKinds {
			if allowedKind.Kind == supportedKind.Kind {
				if allowedKind.Group == nil || supportedKind.Group == nil ||
					*allowedKind.Group == *supportedKind.Group {
					supportedKinds = append(supportedKinds, supportedKind)
					break
				}
			}
		}
	}
	return supportedKinds
}

func ensureParentRefNamespaces(parentRefs []gwv1.ParentReference, routeNamespace string) {
	for i := range parentRefs {
		if parentRefs[i].Namespace == nil {
			routeNs := gwv1.Namespace(routeNamespace)
			parentRefs[i].Namespace = &routeNs
		}
	}
}
