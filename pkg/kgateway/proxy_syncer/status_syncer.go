package proxy_syncer

import (
	"context"

	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	plug "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

var _ manager.LeaderElectionRunnable = &StatusSyncer{}

// statusSyncMaxWorkers bounds the number of concurrent status writes; the worker queue
// additionally guarantees at most one in-flight write per resource. Keep this conservative:
// at 5k routes, 8 workers retained one route write per route with no conflicts at 0, 10,
// and 50ms write latency, while materially improving convergence under injected latency.
// Higher caps reintroduced intermediate writes and conflicts at low latency.
const statusSyncMaxWorkers = 8

// StatusSyncer runs only on the leader and writes the status of resources.
//
// This runnable attaches raw KRT object and report handlers to a worker-pool write queue
// when leadership is acquired. The queue retains only resource identities; each writer
// builds desired status just-in-time from the latest KRT state. Writes go through the same
// istio informer cache translation reads from, and informer updates self-heal conflicts.
type StatusSyncer struct {
	istioClient    apiclient.Client
	plugins        plug.Plugin
	controllerName string

	statusCollections      *statussync.StatusCollections
	writers                map[schema.GroupVersionKind]statussync.ResourceStatusSyncer
	gatewayStatusSnapshots krt.Collection[GatewayStatusSnapshot]
	cacheSyncs             []cache.InformerSynced

	customStatusSync func(ctx context.Context, rm reports.ReportMap)
}

// StatusSyncerConfig holds the dependencies required to construct a StatusSyncer.
type StatusSyncerConfig struct {
	Plugins                plug.Plugin
	ControllerName         string
	Client                 apiclient.Client
	StatusCollections      *statussync.StatusCollections
	StatusWriters          map[schema.GroupVersionKind]statussync.ResourceStatusSyncer
	GatewayStatusSnapshots krt.Collection[GatewayStatusSnapshot]
	CacheSyncs             []cache.InformerSynced
}

func NewStatusSyncer(cfg StatusSyncerConfig, opts ...StatusSyncerOption) *StatusSyncer {
	optCfg := processStatusSyncerOptions(opts...)
	return &StatusSyncer{
		plugins:                cfg.Plugins,
		istioClient:            cfg.Client,
		controllerName:         cfg.ControllerName,
		statusCollections:      cfg.StatusCollections,
		writers:                cfg.StatusWriters,
		gatewayStatusSnapshots: cfg.GatewayStatusSnapshots,
		cacheSyncs:             cfg.CacheSyncs,
		customStatusSync:       optCfg.CustomStatusSync,
	}
}

func (s *StatusSyncer) Start(ctx context.Context) error {
	logger.Info("starting Status Syncer", "controller", s.controllerName)

	// wait for krt collections to sync
	logger.Info("waiting for cache to sync")
	s.istioClient.WaitForCacheSync(
		"kube gw status syncer",
		ctx.Done(),
		s.cacheSyncs...,
	)
	logger.Info("caches warm!")

	// caches are warm, now we can do registrations
	for _, regFunc := range s.plugins.ContributesLeaderAction {
		if regFunc != nil {
			regFunc()
		}
	}

	if s.customStatusSync != nil {
		// Aggregate once per Gateway snapshot event, rather than once per
		// individual contribution emitted by that snapshot.
		merged := krt.NewSingleton(func(kctx krt.HandlerContext) *customStatusReports {
			var contributions []reports.StatusContribution
			for _, snapshot := range krt.Fetch(kctx, s.gatewayStatusSnapshots) {
				contributions = append(contributions, snapshot.Contributions...)
			}
			return &customStatusReports{Report: reports.MergeStatusContributions(contributions)}
		}, krt.WithStop(ctx.Done()), krt.WithName("CustomStatusReports"))
		registration := merged.Register(func(event krt.Event[customStatusReports]) {
			if event.New != nil {
				s.customStatusSync(ctx, event.New.Report)
			}
		})
		defer registration.UnregisterHandler()
	}

	pool := statussync.NewWorkerPool(ctx, func(ctx context.Context, resource statussync.Resource) {
		s.syncStatus(ctx, resource)
	}, statusSyncMaxWorkers)
	s.statusCollections.SetQueue(pool)
	defer s.statusCollections.UnsetQueue()

	<-ctx.Done()
	return nil
}

type customStatusReports struct {
	Report reports.ReportMap
}

func (r customStatusReports) ResourceName() string {
	return "custom-status-reports"
}

func (r customStatusReports) Equals(other customStatusReports) bool {
	return reports.EqualReportMaps(r.Report, other.Report)
}

// syncStatus dispatches one queued status write to the writer registered for its GVK.
func (s *StatusSyncer) syncStatus(ctx context.Context, resource statussync.Resource) {
	writer, ok := s.writers[resource.GroupVersionKind]
	if !ok {
		logger.Error("sync status: no writer registered for resource type", "gvk", resource.GroupVersionKind.String(), "resource", resource.NamespacedName.String())
		return
	}
	writer.ApplyStatus(ctx, resource)
}

// NeedLeaderElection returns true to ensure that the StatusSyncer runs only on the leader
func (s *StatusSyncer) NeedLeaderElection() bool {
	return true
}
