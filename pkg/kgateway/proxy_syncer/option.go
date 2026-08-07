package proxy_syncer

import (
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

type statusSyncerConfig struct {
	statusRegistrations []StatusRegistration
}

type StatusSyncerOption func(*statusSyncerConfig)

// StatusRegistrationInputs exposes the keyed status pipeline to downstream resource
// types. Registrations construct per-resource report reductions and writers during
// controller setup; their event handlers are attached only while this replica is leader.
type StatusRegistrationInputs struct {
	// Collections owns the raw-resource and reduced-report event sources that feed the
	// leader's status queue. Use statussync.RegisterResource to register raw collections
	// and statussync.RegisterResourceReports to register per-resource reductions; the
	// latter also enrols the reduction in the StatusSyncer's cache synchronization barrier.
	Collections *statussync.StatusCollections
	// StatusContributions contains all Gateway- and Backend-produced status facts.
	StatusContributions krt.Collection[reports.StatusContribution]
	// ContributionsByTarget selects the facts belonging to one status owner.
	ContributionsByTarget krt.Index[reports.StatusKey, reports.StatusContribution]
	// KrtOpts supplies the standard collection lifecycle and debugging options.
	KrtOpts krtutil.KrtOptions
	// NotReady re-queues resources whose client cannot see them yet, and is shared with
	// every built-in writer. A registration whose writer reads through a delayed client
	// must pass it on, or a resource enqueued before that client's informer loads silently
	// never gets status. See statussync.NotReadyRequeuer.
	NotReady *statussync.NotReadyRequeuer
	// RegisterWriter registers the just-in-time writer for a resource GVK.
	RegisterWriter func(schema.GroupVersionKind, statussync.ResourceStatusSyncer)
}

// StatusRegistration adds one resource-scoped status pipeline extension.
type StatusRegistration func(StatusRegistrationInputs)

func processStatusSyncerOptions(opts ...StatusSyncerOption) *statusSyncerConfig {
	cfg := &statusSyncerConfig{}
	for _, fn := range opts {
		fn(cfg)
	}
	return cfg
}

// WithStatusRegistration registers a downstream resource type with the keyed status
// pipeline. The registration runs on every replica during controller construction; actual
// reconciliation handlers and writes remain leader-gated by StatusCollections.
func WithStatusRegistration(registration StatusRegistration) StatusSyncerOption {
	return func(cfg *statusSyncerConfig) {
		if registration != nil {
			cfg.statusRegistrations = append(cfg.statusRegistrations, registration)
		}
	}
}
