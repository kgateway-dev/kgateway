package proxy_syncer

import (
	"context"
	"errors"

	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/kgateway-dev/kgateway/v2/pkg/apiclient"
	plug "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	krtpkg "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

var _ manager.LeaderElectionRunnable = &StatusSyncer{}

// statusSyncMaxWorkers bounds the number of concurrent status writes; the worker queue
// additionally guarantees at most one in-flight write per resource. Keep this conservative:
// at 5k routes, 8 workers retained one route write per route with no conflicts at 0, 10,
// and 50ms write latency, while materially improving convergence under injected latency.
// Higher caps reintroduced intermediate writes and conflicts at low latency.
const statusSyncMaxWorkers = 8

// statusPipeline is everything between translation's status contributions and the leader's
// status writes: the per-owner report reducers and the raw-object sources that enqueue them,
// the writer for each GVK, and the cache-sync barrier the leader's first sweep waits on.
//
// The ProxySyncer owns it. It is built on every replica during ProxySyncer.Init, and only
// the StatusSyncer, which runs on the leader, attaches it to a write queue.
type statusPipeline struct {
	// registrations are the downstream kinds from WithStatusRegistration. They run after the
	// built-in kinds and the policy plugins, so a downstream writer cannot displace a built-in
	// one (registerWriter keeps the first).
	registrations []StatusRegistration

	// The fields below are set by init.

	// in is what every registration -- built-in kind, policy plugin, or downstream -- is
	// handed. Its RegisterWriter is registerWriter.
	in      statussync.RegistrationInputs
	writers map[schema.GroupVersionKind]statussync.ResourceStatusSyncer
	// cacheSyncs gate the leader's first status sweep. It includes in.Collections.HasSynced,
	// which re-reads the reducer set on each call and so covers every registration.
	cacheSyncs []cache.InformerSynced
}

// init builds the pipeline's inputs from translation's status contributions. Registrations
// follow, through in.
func (p *statusPipeline) init(controllerName string, contributions krt.Collection[reports.StatusContribution], krtopts krtutil.KrtOptions) {
	p.writers = map[schema.GroupVersionKind]statussync.ResourceStatusSyncer{}
	p.in = statussync.RegistrationInputs{
		Collections:         statussync.NewStatusCollections(),
		StatusContributions: contributions,
		ContributionsByTarget: krtpkg.UnnamedIndex(contributions, func(contribution reports.StatusContribution) []reports.StatusKey {
			return []reports.StatusKey{contribution.Target}
		}),
		KrtOpts:        krtopts,
		ControllerName: controllerName,
		RegisterWriter: p.registerWriter,
	}
}

func (p *statusPipeline) initialized() bool {
	return p.in.Collections != nil
}

// registerDownstream runs the WithStatusRegistration registrations.
func (p *statusPipeline) registerDownstream() {
	for _, register := range p.registrations {
		register(p.in)
	}
}

// registerWriter records the writer for a GVK. Exactly one writer may own a GVK: a second
// registration means two plugins both claim to persist that resource's status, and whichever
// registered last would silently win. Keep the first and report the conflict rather than
// letting registration order decide who writes status.
//
// Every registration goes through here, including the built-in kinds. A guard that half the
// entries bypass is worse than no guard: the built-in kinds are exactly the ones a plugin is
// most likely to collide with, and assigning into the map directly is what made that
// collision silent.
func (p *statusPipeline) registerWriter(gvk schema.GroupVersionKind, writer statussync.ResourceStatusSyncer) {
	if _, exists := p.writers[gvk]; exists {
		logger.Error("status writer already registered for resource type; ignoring duplicate registration",
			"gvk", gvk.String())
		return
	}
	p.writers[gvk] = writer
}

// syncStatus dispatches one queued status write to the writer registered for its GVK.
func (p *statusPipeline) syncStatus(ctx context.Context, resource statussync.Resource) {
	writer, ok := p.writers[resource.GroupVersionKind]
	if !ok {
		logger.Error("sync status: no writer registered for resource type", "gvk", resource.GroupVersionKind.String(), "resource", resource.NamespacedName.String())
		return
	}
	writer.ApplyStatus(ctx, resource)
}

// StatusSyncer runs only on the leader and writes the status of resources.
//
// This runnable attaches raw KRT object and report handlers to a worker-pool write queue
// when leadership is acquired. The queue retains only resource identities; each writer
// builds desired status just-in-time from the latest KRT state. Writes go through the same
// istio informer cache translation reads from, and informer updates self-heal conflicts.
//
// Get it from ProxySyncer.StatusSyncer.
type StatusSyncer struct {
	istioClient    apiclient.Client
	plugins        plug.Plugin
	controllerName string

	pipeline *statusPipeline
}

func (s *StatusSyncer) Start(ctx context.Context) error {
	if !s.pipeline.initialized() {
		return errors.New("status syncer started before ProxySyncer.Init built its status pipeline")
	}
	logger.Info("starting Status Syncer", "controller", s.controllerName)

	// wait for krt collections to sync
	logger.Info("waiting for cache to sync")
	s.istioClient.WaitForCacheSync(
		"kube gw status syncer",
		ctx.Done(),
		s.pipeline.cacheSyncs...,
	)
	logger.Info("caches warm!")

	// caches are warm, now we can do registrations
	for _, regFunc := range s.plugins.ContributesLeaderAction {
		if regFunc != nil {
			regFunc()
		}
	}

	pool := statussync.NewWorkerPool(ctx, s.pipeline.syncStatus, statusSyncMaxWorkers)
	s.pipeline.in.Collections.SetQueue(pool)
	defer s.pipeline.in.Collections.UnsetQueue()

	<-ctx.Done()
	return nil
}

// NeedLeaderElection returns true to ensure that the StatusSyncer runs only on the leader
func (s *StatusSyncer) NeedLeaderElection() bool {
	return true
}
