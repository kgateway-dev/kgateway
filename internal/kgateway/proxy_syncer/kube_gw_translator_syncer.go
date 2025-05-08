package proxy_syncer

import (
	"context"
	"log/slog"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/logging"
)

func (s *ProxyTranslator) syncXds(
	ctx context.Context,
	snapWrap XdsSnapWrapper,
) {
	logger := logging.New("xds-syncer")

	snap := snapWrap.snap
	proxyKey := snapWrap.proxyKey

	// TODO: handle errored clusters by fetching them from the previous snapshot and using the old cluster

	// stringifying the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	logger.Debug("syncing xds snapshot", slog.String("proxyKey", proxyKey))

	// Check if the default logger is enabled for Debug level
	if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		//	logger.Debug(syncutil.StringifySnapshot(snap), slog.String("proxyKey", proxyKey)) // TODO: also spammy
	}

	// if the snapshot is not consistent, make it so
	// TODO: me may need to copy this to not change krt cache.
	// TODO: this is also may not be needed now that envoy has
	// a default initial fetch timeout
	// snap.MakeConsistent()
	s.xdsCache.SetSnapshot(ctx, proxyKey, snap)
}
