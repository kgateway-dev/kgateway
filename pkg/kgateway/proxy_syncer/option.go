package proxy_syncer

import (
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
)

type statusSyncerConfig struct {
	statusRegistrations   []StatusRegistration
	policyTargetResolvers policyTargetResolvers
}

// StatusSyncerOption configures the status pipeline. The same options are passed to both
// NewProxySyncer, which produces the status contributions, and NewStatusSyncer, which reduces
// and writes them; each reads only the settings it owns.
type StatusSyncerOption func(*statusSyncerConfig)

// StatusRegistration adds one resource-scoped status pipeline extension. Registrations
// construct per-resource report reductions and writers during controller setup; their event
// handlers are attached only while this replica is leader.
//
// It takes statussync.RegistrationInputs under its own name rather than a local alias.
// RegistrationInputs is deliberately one struct shared by both entry points -- its doc
// comment explains why -- and re-spelling it per package undoes that in the reader's head:
// someone who greps the local name finds a type with no fields.
type StatusRegistration func(statussync.RegistrationInputs)

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

// WithPolicyTargetResolver checks policy targetRefs of kind gk with resolver, so a ref naming
// an object that does not exist is reported as TargetNotFound on the policy's StatusSummary
// ancestor. kgateway already resolves the Gateway API kinds, Service, Backend, and the alias
// kinds backend plugins declare; this is for kinds only an extension knows, such as a
// ListenerSet-like CRD it attaches policies to itself. A later registration for the same kind,
// including a built-in one, replaces the earlier resolver. NewObjectPolicyTargetResolver
// builds one from a collection.
func WithPolicyTargetResolver(gk schema.GroupKind, resolver PolicyTargetResolver) StatusSyncerOption {
	return func(cfg *statusSyncerConfig) {
		if resolver == nil {
			return
		}
		if cfg.policyTargetResolvers == nil {
			cfg.policyTargetResolvers = policyTargetResolvers{}
		}
		cfg.policyTargetResolvers[gk] = resolver
	}
}
