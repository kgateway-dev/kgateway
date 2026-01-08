package proxy_syncer

import (
	"context"

	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

type statusSyncerConfig struct {
	CustomStatusSync func(ctx context.Context, rm reports.ReportMap)
}

type StatusSyncerOption func(*statusSyncerConfig)

func processStatusSyncerOptions(opts ...StatusSyncerOption) *statusSyncerConfig {
	cfg := &statusSyncerConfig{}
	for _, fn := range opts {
		fn(cfg)
	}
	return cfg
}

func WithCustomStatusSync(customSync func(ctx context.Context, rm reports.ReportMap)) StatusSyncerOption {
	return func(cfg *statusSyncerConfig) {
		if customSync != nil {
			cfg.CustomStatusSync = customSync
		}
	}
}
