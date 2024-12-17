package proxy_syncer

import (
	"context"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func (s *ProxyTranslator) syncXds(
	ctx context.Context,
	snapWrap XdsSnapWrapper,
) {
	ctx = contextutils.WithLogger(ctx, "kube-gateway-xds-syncer")
	logger := contextutils.LoggerFrom(ctx).Desugar()

	snap := snapWrap.snap
	proxyKey := snapWrap.proxyKey

	// TODO: handle errored clusters by fetching them from the previous snapshot and using the old cluster

	// stringifying the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	logger.Debug("syncing xds snapshot", zap.String("proxyKey", proxyKey))
	if contextutils.GetLogLevel() == zapcore.DebugLevel {
		//	logger.Debugw(syncutil.StringifySnapshot(snap), "proxyKey", proxyKey) // TODO: also spammy
	}

	// if the snapshot is not consistent, make it so
	// TODO: me may need to copy this to not change krt cache.
	// TODO: this is also may not be needed now that envoy has
	// a default initial fetch timeout
	snap.MakeConsistent()
	s.xdsCache.SetSnapshot(proxyKey, snap)

}

func (s *ProxyTranslator) syncStatus(
	ctx context.Context,
	proxyKey string,
	reports reporter.ResourceReports,
) error {
	ctx = contextutils.WithLogger(ctx, "kube-gateway-xds-syncer")
	logger := contextutils.LoggerFrom(ctx)
	logger = logger

	//TODO: handle statuses
	return nil
}
