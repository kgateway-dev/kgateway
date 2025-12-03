package syncer

import (
	"context"

	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

type StatusSyncerConfig struct {
	CustomStatusSync func(ctx context.Context, rm reports.ReportMap)
}

type StatusSyncerOption func(*StatusSyncerConfig)

func ProcessStatusSyncerOptions(opts ...StatusSyncerOption) *StatusSyncerConfig {
	cfg := &StatusSyncerConfig{}
	for _, fn := range opts {
		fn(cfg)
	}
	return cfg
}

func WithCustomStatusSync(customSync func(ctx context.Context, rm reports.ReportMap)) StatusSyncerOption {
	return func(cfg *StatusSyncerConfig) {
		cfg.CustomStatusSync = customSync
	}
}
