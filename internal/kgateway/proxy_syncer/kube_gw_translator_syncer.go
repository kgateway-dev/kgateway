package proxy_syncer

import (
	"context"

	tmetrics "github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
)

func (s *ProxyTranslator) syncXds(
	ctx context.Context,
	snapWrap XdsSnapWrapper,
) {
	snap := snapWrap.snap
	proxyKey := snapWrap.proxyKey

	// TODO: handle errored clusters by fetching them from the previous snapshot and using the old cluster

	// stringifying the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	logger.Debug("syncing xds snapshot", "proxy_key", proxyKey)

	logger.Log(ctx, logging.LevelTrace, "syncing xds snapshot", "proxy_key", proxyKey)

	cd := getDetailsFromXDSClientResourceName(snapWrap.ResourceName())

	// if the snapshot is not consistent, make it so
	// TODO: me may need to copy this to not change krt cache.
	// TODO: this is also may not be needed now that envoy has
	// a default initial fetch timeout
	// snap.MakeConsistent()
	s.xdsCache.SetSnapshot(ctx, proxyKey, snap)

	tmetrics.IncXDSSnapshotSync(cd.Gateway, cd.Namespace)

	tmetrics.EndResourceSync(tmetrics.ResourceSyncDetails{
		Namespace:    cd.Namespace,
		Gateway:      cd.Gateway,
		ResourceType: "XDSSnapshot",
		ResourceName: cd.Gateway,
	}, true, resourcesXDSSyncsTotal, resourcesXDSSyncDuration)
}
