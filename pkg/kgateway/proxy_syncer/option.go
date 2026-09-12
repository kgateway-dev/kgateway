package proxy_syncer

import (
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
)

type statusSyncerConfig struct {
	statusRegistrations []StatusRegistration
}

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
