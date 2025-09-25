package agentgatewaysyncer

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"time"

	"github.com/avast/retry-go/v4"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/slices"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/agentgatewaysyncer/status"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

var _ manager.LeaderElectionRunnable = &AgentGwStatusSyncer{}

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
	controllerName string
	agwClassName   string

	statusCollections *status.StatusCollections

	// Synchronization
	cacheSyncs []cache.InformerSynced
}

func NewAgwStatusSyncer(
	controllerName string,
	agwClassName string,
	client kube.Client,
	mgr manager.Manager,
	statusCollections *status.StatusCollections,
	cacheSyncs []cache.InformerSynced,
) *AgentGwStatusSyncer {
	syncer := &AgentGwStatusSyncer{
		controllerName:    controllerName,
		agwClassName:      agwClassName,
		client:            client,
		mgr:               mgr,
		statusCollections: statusCollections,
		cacheSyncs:        cacheSyncs,
	}

	return syncer
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

	// Create a controllers.Queue that wraps our async queue for Istio's StatusCollections
	// The policyStatusQueue implements https://github.com/istio/istio/blob/531c61709aaa9bc9187c625e9e460be98f2abf2e/pilot/pkg/status/manager.go#L107
	nq := s.NewStatusWorker(ctx)
	s.statusCollections.SetQueue(nq)

	<-ctx.Done()
	return nil
}

func (s *AgentGwStatusSyncer) SyncStatus(ctx context.Context, resource status.Resource, statusObj any) {
	switch resource.GroupVersionKind {
	case wellknown.GatewayGVK:
		gatewayStatusLogger := logger.With("subcomponent", "gatewayStatusSyncer")
		r := status.NamedStatus[*gwv1.GatewayStatus]{
			Name:   resource.NamespacedName,
			Status: statusObj.(*gwv1.GatewayStatus),
		}
		s.syncGatewayStatus(ctx, gatewayStatusLogger, r)
	case wellknown.XListenerSetGVK:
		listenerSetStatusLogger := logger.With("subcomponent", "listenerSetStatusSyncer")
		r := status.NamedStatus[*gwxv1a1.ListenerSetStatus]{
			Name:   resource.NamespacedName,
			Status: statusObj.(*gwxv1a1.ListenerSetStatus),
		}
		s.syncListenerSetStatus(ctx, listenerSetStatusLogger, r)
	case wellknown.GRPCRouteGVK:
		routeStatusLogger := logger.With("subcomponent", "routeStatusSyncer")
		r := status.NamedStatus[*gwv1.GRPCRouteStatus]{
			Name:   resource.NamespacedName,
			Status: statusObj.(*gwv1.GRPCRouteStatus),
		}
		s.syncGRPCRouteStatus(ctx, routeStatusLogger, r)
	case wellknown.TLSRouteGVK:
		routeStatusLogger := logger.With("subcomponent", "routeStatusSyncer")
		r := status.NamedStatus[*gwv1alpha2.TLSRouteStatus]{
			Name:   resource.NamespacedName,
			Status: statusObj.(*gwv1alpha2.TLSRouteStatus),
		}
		s.syncTLSRouteStatus(ctx, routeStatusLogger, r)
	case wellknown.TCPRouteGVK:
		routeStatusLogger := logger.With("subcomponent", "routeStatusSyncer")
		r := status.NamedStatus[*gwv1alpha2.TCPRouteStatus]{
			Name:   resource.NamespacedName,
			Status: statusObj.(*gwv1alpha2.TCPRouteStatus),
		}
		s.syncTCPRouteStatus(ctx, routeStatusLogger, r)
	case wellknown.HTTPRouteGVK:
		routeStatusLogger := logger.With("subcomponent", "routeStatusSyncer")
		r := status.NamedStatus[*gwv1.HTTPRouteStatus]{
			Name:   resource.NamespacedName,
			Status: statusObj.(*gwv1.HTTPRouteStatus),
		}
		s.syncHTTPRouteStatus(ctx, routeStatusLogger, r)
	case wellknown.TrafficPolicyGVK:
		policyStatusLogger := logger.With("subcomponent", "policyStatusSyncer")
		r := status.NamedStatus[*gwv1.PolicyStatus]{
			Name:   resource.NamespacedName,
			Status: statusObj.(*gwv1.PolicyStatus),
		}
		s.syncTrafficPolicyStatus(ctx, policyStatusLogger, r)
	default:
		log.Fatalf("SyncStatus: unknown resource type: %v", resource.GroupVersionKind)
	}
}

func (s *AgentGwStatusSyncer) NewStatusWorker(ctx context.Context) *status.WorkerPool {
	return status.NewWorkerPool(ctx, s.SyncStatus, 100)
}

func (s *AgentGwStatusSyncer) syncTrafficPolicyStatus(ctx context.Context, logger *slog.Logger, policyStatusUpdate status.NamedStatus[*gwv1.PolicyStatus]) {
	stopwatch := utils.NewTranslatorStopWatch("PolicyStatusSyncer")
	stopwatch.Start()
	defer stopwatch.Stop(ctx)

	policyNameNs := policyStatusUpdate.Name

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
		trafficpolicy.Status = gwv1.PolicyStatus{
			Ancestors: slices.Clone(policyStatusUpdate.Status.Ancestors),
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
		logger.Error("failed to sync policy status after retries", logKeyError, err, "policy", policyNameNs.String())
	} else {
		logger.Debug("updated policy status", "policy", policyNameNs.String(), "status", policyStatusUpdate.Status)
	}
}

func (s *AgentGwStatusSyncer) syncHTTPRouteStatus(ctx context.Context, logger *slog.Logger, routeStatus status.NamedStatus[*gwv1.HTTPRouteStatus]) {
	stopwatch := utils.NewTranslatorStopWatch("HTTPRouteStatusSyncer")
	stopwatch.Start()
	defer stopwatch.Stop(ctx)
	startTime := time.Now()

	// Helper function to sync route status with retry
	syncStatusWithRetry := func(
		routeType string,
		routeKey client.ObjectKey,
	) error {
		return retry.Do(
			func() error {
				route := &gwv1.HTTPRoute{}
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

				route.Status = *routeStatus.Status
				if err := s.mgr.GetClient().Status().Update(ctx, route); err != nil {
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

	err := syncStatusWithRetry(
		wellknown.HTTPRouteKind,
		routeStatus.Name,
	)
	if err != nil {
		logger.Error("all attempts failed at updating HTTPRoute status", logKeyError, err, "route", routeStatus.Name)
	}
}

func (s *AgentGwStatusSyncer) syncGRPCRouteStatus(ctx context.Context, logger *slog.Logger, routeStatus status.NamedStatus[*gwv1.GRPCRouteStatus]) {
	stopwatch := utils.NewTranslatorStopWatch("GRPCRouteStatusSyncer")
	stopwatch.Start()
	defer stopwatch.Stop(ctx)
	startTime := time.Now()

	// Helper function to sync route status with retry
	syncStatusWithRetry := func(
		routeType string,
		routeKey client.ObjectKey,
	) error {
		return retry.Do(
			func() error {
				route := &gwv1.GRPCRoute{}
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

				route.Status = *routeStatus.Status
				if err := s.mgr.GetClient().Status().Update(ctx, route); err != nil {
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

	err := syncStatusWithRetry(
		wellknown.GRPCRouteKind,
		routeStatus.Name,
	)
	if err != nil {
		logger.Error("all attempts failed at updating GRPCRoute status", logKeyError, err, "route", routeStatus.Name)
	}
}

func (s *AgentGwStatusSyncer) syncTLSRouteStatus(ctx context.Context, logger *slog.Logger, routeStatus status.NamedStatus[*gwv1alpha2.TLSRouteStatus]) {
	stopwatch := utils.NewTranslatorStopWatch("TLSRouteStatusSyncer")
	stopwatch.Start()
	defer stopwatch.Stop(ctx)
	startTime := time.Now()

	// Helper function to sync route status with retry
	syncStatusWithRetry := func(
		routeType string,
		routeKey client.ObjectKey,
	) error {
		return retry.Do(
			func() error {
				route := &gwv1alpha2.TLSRoute{}
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

				route.Status = *routeStatus.Status
				if err := s.mgr.GetClient().Status().Update(ctx, route); err != nil {
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

	err := syncStatusWithRetry(
		wellknown.TLSRouteKind,
		routeStatus.Name,
	)
	if err != nil {
		logger.Error("all attempts failed at updating TLSRoute status", logKeyError, err, "route", routeStatus.Name)
	}
}

func (s *AgentGwStatusSyncer) syncTCPRouteStatus(ctx context.Context, logger *slog.Logger, routeStatus status.NamedStatus[*gwv1alpha2.TCPRouteStatus]) {
	stopwatch := utils.NewTranslatorStopWatch("TCPRouteStatusSyncer")
	stopwatch.Start()
	defer stopwatch.Stop(ctx)
	startTime := time.Now()

	// Helper function to sync route status with retry
	syncStatusWithRetry := func(
		routeType string,
		routeKey client.ObjectKey,
	) error {
		return retry.Do(
			func() error {
				route := &gwv1alpha2.TCPRoute{}
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

				route.Status = *routeStatus.Status
				if err := s.mgr.GetClient().Status().Update(ctx, route); err != nil {
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

	err := syncStatusWithRetry(
		wellknown.TCPRouteKind,
		routeStatus.Name,
	)
	if err != nil {
		logger.Error("all attempts failed at updating TCPRoute status", logKeyError, err, "route", routeStatus.Name)
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
			Message:            reports.GatewayAcceptedMessage,
			ObservedGeneration: generation,
		})
	}

	// Ensure Programmed condition exists
	if meta.FindStatusCondition(status.Conditions, string(gwv1.GatewayConditionProgrammed)) == nil {
		meta.SetStatusCondition(&status.Conditions, metav1.Condition{
			Type:               string(gwv1.GatewayConditionProgrammed),
			Status:             metav1.ConditionTrue,
			Reason:             string(gwv1.GatewayReasonProgrammed),
			Message:            reports.GatewayProgrammedMessage,
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
func (s *AgentGwStatusSyncer) syncGatewayStatus(ctx context.Context, logger *slog.Logger, gatewayStatus status.NamedStatus[*gwv1.GatewayStatus]) {
	stopwatch := utils.NewTranslatorStopWatch("GatewayStatusSyncer")
	stopwatch.Start()

	gwnn := gatewayStatus.Name
	err := retry.Do(func() error {
		gw := gwv1.Gateway{}
		err := s.mgr.GetClient().Get(ctx, gwnn, &gw)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}

			logger.Info("error getting gw", logKeyError, err, logKeyGateway, gwnn.String())
			return err
		}

		// Only process agentgateway classes - others are handled by ProxySyncer
		if string(gw.Spec.GatewayClassName) != s.agwClassName {
			logger.Debug("skipping status sync for non-agentgateway", logKeyGateway, gwnn.String())
			return nil
		}

		gwStatusWithoutAddress := gw.Status
		gwStatusWithoutAddress.Addresses = nil
		st := gatewayStatus.Status.DeepCopy()
		// Ensure basic Gateway conditions exist (agent-gateway bypasses normal reporter init)
		ensureBasicGatewayConditions(st, gw.Generation)

		original := gw.DeepCopy()
		gw.Status = *st
		if err := s.mgr.GetClient().Status().Patch(ctx, &gw, client.MergeFrom(original)); err != nil {
			if !apierrors.IsConflict(err) {
				logger.Error("error patching gateway status", logKeyError, err, logKeyGateway, gwnn.String())
			}
			return err
		}
		logger.Info("patched gw status", logKeyGateway, gwnn.String(), "generation", gw.Generation)

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
	logger.Debug("synced gw status for gateway", "name", gwnn, "duration", duration)
}

// syncListenerSetStatus will build and update status for all Listener Sets in listener set reports
func (s *AgentGwStatusSyncer) syncListenerSetStatus(ctx context.Context, logger *slog.Logger, listenerSetStatus status.NamedStatus[*gwxv1a1.ListenerSetStatus]) {
	stopwatch := utils.NewTranslatorStopWatch("ListenerSetStatusSyncer")
	stopwatch.Start()

	gwnn := listenerSetStatus.Name
	err := retry.Do(func() error {
		ls := &gwxv1a1.XListenerSet{}
		err := s.mgr.GetClient().Get(ctx, gwnn, ls)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}

			logger.Info("error getting listenerset", logKeyError, err, logKeyGateway, gwnn.String())
			return err
		}

		ls.Status = *listenerSetStatus.Status
		if err := s.mgr.GetClient().Status().Update(ctx, ls); err != nil {
			if !apierrors.IsConflict(err) {
				logger.Error("error updating listenerset status", logKeyError, err, logKeyGateway, gwnn.String())
			}
			return err
		}
		logger.Info("updated gw status", logKeyGateway, gwnn.String(), "generation", ls.Generation)

		return nil
	},
		retry.Attempts(maxRetryAttempts),
		retry.Delay(retryDelay),
		retry.DelayType(retry.BackOffDelay),
	)
	if err != nil {
		logger.Error("all attempts failed at updating listenerSet statuses", logKeyError, err)
	}
	duration := stopwatch.Stop(ctx)
	logger.Debug("synced status for listenerSet", "name", gwnn, "duration", duration)
}

// NeedLeaderElection returns true to ensure that the AgentGwStatusSyncer runs only on the leader
func (r *AgentGwStatusSyncer) NeedLeaderElection() bool {
	return true
}
