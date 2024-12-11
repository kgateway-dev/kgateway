package proxy_syncer

import (
	"context"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"

	"github.com/solo-io/gloo/projects/gloo/pkg/xds"

	"go.uber.org/zap/zapcore"
)

func (s *ProxyTranslator) syncXds(
	ctx context.Context,
	snap *xds.EnvoySnapshot,
	proxyKey string,
) {
	ctx = contextutils.WithLogger(ctx, "kube-gateway-xds-syncer")
	logger := contextutils.LoggerFrom(ctx)

	// stringifying the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	if contextutils.GetLogLevel() == zapcore.DebugLevel {
		logger.Debugw("syncing xds snapshot", "proxyKey", proxyKey)
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
