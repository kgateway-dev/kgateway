package agentgatewaysyncer

import (
	"context"
	"log"
	"time"

	"github.com/avast/retry-go/v4"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/kclient"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/agentgatewaysyncer/status"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
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
	client kube.Client

	trafficPolicies StatusSyncer[*v1alpha1.TrafficPolicy, *gwv1.PolicyStatus]

	// Configuration
	controllerName string
	agwClassName   string

	statusCollections *status.StatusCollections

	cacheSyncs []cache.InformerSynced

	listenerSets StatusSyncer[*gwxv1a1.XListenerSet, *gwxv1a1.ListenerSetStatus]
	gateways     StatusSyncer[*gwv1.Gateway, *gwv1.GatewayStatus]
	httpRoutes   StatusSyncer[*gwv1.HTTPRoute, *gwv1.HTTPRouteStatus]
	grpcRoutes   StatusSyncer[*gwv1.GRPCRoute, *gwv1.GRPCRouteStatus]
	tcpRoutes    StatusSyncer[*gwv1alpha2.TCPRoute, *gwv1alpha2.TCPRouteStatus]
	tlsRoutes    StatusSyncer[*gwv1alpha2.TLSRoute, *gwv1alpha2.TLSRouteStatus]
}

func NewAgwStatusSyncer(
	controllerName string,
	agwClassName string,
	client kube.Client,
	statusCollections *status.StatusCollections,
	cacheSyncs []cache.InformerSynced,
) *AgentGwStatusSyncer {
	f := kclient.Filter{ObjectFilter: client.ObjectFilter()}
	syncer := &AgentGwStatusSyncer{
		controllerName:    controllerName,
		agwClassName:      agwClassName,
		client:            client,
		statusCollections: statusCollections,
		cacheSyncs:        cacheSyncs,

		trafficPolicies: StatusSyncer[*v1alpha1.TrafficPolicy, *gwv1.PolicyStatus]{
			name:   "trafficPolicy",
			client: kclient.NewFilteredDelayed[*v1alpha1.TrafficPolicy](client, wellknown.TrafficPolicyGVR, f),
			build: func(om metav1.ObjectMeta, s *gwv1.PolicyStatus) *v1alpha1.TrafficPolicy {
				return &v1alpha1.TrafficPolicy{
					ObjectMeta: om,
					Status: gwv1.PolicyStatus{
						Ancestors: s.Ancestors,
					},
				}
			},
		},
		httpRoutes: StatusSyncer[*gwv1.HTTPRoute, *gwv1.HTTPRouteStatus]{
			name:   "httpRoute",
			client: kclient.NewFilteredDelayed[*gwv1.HTTPRoute](client, wellknown.HTTPRouteGVR, f),
			build: func(om metav1.ObjectMeta, s *gwv1.HTTPRouteStatus) *gwv1.HTTPRoute {
				return &gwv1.HTTPRoute{
					ObjectMeta: om,
					Status:     *s,
				}
			},
		},
		grpcRoutes: StatusSyncer[*gwv1.GRPCRoute, *gwv1.GRPCRouteStatus]{
			name:   "grpcRoute",
			client: kclient.NewFilteredDelayed[*gwv1.GRPCRoute](client, wellknown.GRPCRouteGVR, f),
			build: func(om metav1.ObjectMeta, s *gwv1.GRPCRouteStatus) *gwv1.GRPCRoute {
				return &gwv1.GRPCRoute{
					ObjectMeta: om,
					Status:     *s,
				}
			},
		},
		tlsRoutes: StatusSyncer[*gwv1alpha2.TLSRoute, *gwv1alpha2.TLSRouteStatus]{
			name:   "tlsRoute",
			client: kclient.NewFilteredDelayed[*gwv1alpha2.TLSRoute](client, wellknown.TLSRouteGVR, f),
			build: func(om metav1.ObjectMeta, s *gwv1alpha2.TLSRouteStatus) *gwv1alpha2.TLSRoute {
				return &gwv1alpha2.TLSRoute{
					ObjectMeta: om,
					Status:     *s,
				}
			},
		},
		tcpRoutes: StatusSyncer[*gwv1alpha2.TCPRoute, *gwv1alpha2.TCPRouteStatus]{
			name:   "tcpRoute",
			client: kclient.NewFilteredDelayed[*gwv1alpha2.TCPRoute](client, wellknown.TCPRouteGVR, f),
			build: func(om metav1.ObjectMeta, s *gwv1alpha2.TCPRouteStatus) *gwv1alpha2.TCPRoute {
				return &gwv1alpha2.TCPRoute{
					ObjectMeta: om,
					Status:     *s,
				}
			},
		},
		listenerSets: StatusSyncer[*gwxv1a1.XListenerSet, *gwxv1a1.ListenerSetStatus]{
			name:   "listenerSet",
			client: kclient.NewFilteredDelayed[*gwxv1a1.XListenerSet](client, wellknown.XListenerSetGVR, f),
			build: func(om metav1.ObjectMeta, s *gwxv1a1.ListenerSetStatus) *gwxv1a1.XListenerSet {
				return &gwxv1a1.XListenerSet{
					ObjectMeta: om,
					Status:     *s,
				}
			},
		},
		gateways: StatusSyncer[*gwv1.Gateway, *gwv1.GatewayStatus]{
			name:   "gateway",
			client: kclient.NewFilteredDelayed[*gwv1.Gateway](client, wellknown.GatewayGVR, f),
			build: func(om metav1.ObjectMeta, s *gwv1.GatewayStatus) *gwv1.Gateway {
				return &gwv1.Gateway{
					ObjectMeta: om,
					Status:     *s,
				}
			},
		},
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
	s.client.WaitForCacheSync(
		"agent gateway status clients",
		ctx.Done(),
		s.listenerSets.client.HasSynced,
		s.gateways.client.HasSynced,
		s.httpRoutes.client.HasSynced,
		s.grpcRoutes.client.HasSynced,
		s.tcpRoutes.client.HasSynced,
		s.tlsRoutes.client.HasSynced,
	)

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
		s.gateways.ApplyStatus(ctx, resource, statusObj)
	case wellknown.XListenerSetGVK:
		s.listenerSets.ApplyStatus(ctx, resource, statusObj)
	case wellknown.GRPCRouteGVK:
		s.grpcRoutes.ApplyStatus(ctx, resource, statusObj)
	case wellknown.TLSRouteGVK:
		s.tlsRoutes.ApplyStatus(ctx, resource, statusObj)
	case wellknown.TCPRouteGVK:
		s.tcpRoutes.ApplyStatus(ctx, resource, statusObj)
	case wellknown.HTTPRouteGVK:
		s.httpRoutes.ApplyStatus(ctx, resource, statusObj)
	case wellknown.TrafficPolicyGVK:
		s.trafficPolicies.ApplyStatus(ctx, resource, statusObj)
	default:
		log.Fatalf("SyncStatus: unknown resource type: %v", resource.GroupVersionKind)
	}
}

func (s *AgentGwStatusSyncer) NewStatusWorker(ctx context.Context) *status.WorkerPool {
	return status.NewWorkerPool(ctx, s.SyncStatus, 100)
}

type StatusSyncer[O controllers.ComparableObject, S any] struct {
	// Name for logging
	name string

	client kclient.Client[O]

	build func(om metav1.ObjectMeta, s S) O
}

func (s StatusSyncer[O, S]) ApplyStatus(ctx context.Context, obj status.Resource, statusObj any) {
	status := statusObj.(S)
	stopwatch := utils.NewTranslatorStopWatch(s.name + "Status")
	stopwatch.Start()
	defer stopwatch.Stop(ctx)

	logger := logger.With("kind", s.name, "resource", obj.NamespacedName.String())
	// TODO: move this to retry by putting it back on the queue, with some limit on the retry attempts allowed
	err := retry.Do(func() error {
		// Pass only the status and minimal part of ObjectMetadata to find the resource and validate it.
		// Passing Spec is ignored by the API server but has costs.
		// Passing ResourceVersion is important to ensure we are not writing stale data. The collection is responsible for
		// re-enqueuing a resource if it ends up being rejected due to stale ResourceVersion.
		_, err := s.client.UpdateStatus(s.build(metav1.ObjectMeta{
			Name:            obj.Name,
			Namespace:       obj.Namespace,
			ResourceVersion: obj.ResourceVersion,
		}, status))

		if err != nil {
			if errors.IsConflict(err) {
				// This is normal. It is expected the collection will re-enqueue the write
				logger.Debug("updating stale status, skipping", logKeyError, err)
				return nil
			}
			logger.Error("error updating status", logKeyError, err)
			return err
		}
		logger.Debug("updated status")
		return nil
	}, retry.Attempts(maxRetryAttempts), retry.Delay(retryDelay))

	if err != nil {
		logger.Error("failed to sync status after retries", logKeyError, err, "policy", obj.NamespacedName.String())
	} else {
		logger.Debug("updated policy status")
	}
}

// NeedLeaderElection returns true to ensure that the AgentGwStatusSyncer runs only on the leader
func (r *AgentGwStatusSyncer) NeedLeaderElection() bool {
	return true
}
