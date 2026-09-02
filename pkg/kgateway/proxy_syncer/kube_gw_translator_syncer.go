package proxy_syncer

import (
	"context"

	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
)

func (s *ProxyTranslator) syncXds(
	ctx context.Context,
	snapWrap XdsSnapWrapper,
) {
	snap := snapWrap.snap
	proxyKey := snapWrap.proxyKey

	// Errored clusters intentionally fail closed. Restoring a cluster from the previous
	// snapshot could keep serving with stale configuration that bypasses the security
	// policy whose validation now fails. snapshotPerClient omits both the errored cluster
	// and its endpoint assignment so that only the affected backend fails, without
	// blocking xDS updates for unrelated clusters.

	// stringifying the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	logger.Debug("syncing xds snapshot", "proxy_key", proxyKey)

	logger.Log(ctx, logging.LevelTrace, "syncing xds snapshot", "proxy_key", proxyKey)

	// We deliberately do not call MakeConsistent(): it would mutate the snapshot
	// shared with the krt cache, and strict CDS/EDS consistency is not the
	// invariant this snapshot maintains. Some endpoint resources — notably the
	// bootstrap-defined local cluster — intentionally have no dynamic CDS
	// counterpart. snapshotPerClient prunes only the CLAs that are known to be
	// unrequestable: STATIC clusters and clusters dropped from CDS by a backend
	// translation error. Envoy's initial fetch timeout covers the remaining
	// transient case of an EDS cluster whose CLA has not landed yet.
	if err := s.xdsCache.SetSnapshot(ctx, proxyKey, snap); err != nil {
		// A rejected snapshot leaves the client on its previous config; surface
		// it rather than silently dropping the update.
		logger.Error("failed to set xds snapshot", "proxy_key", proxyKey, "error", err)
	}
}
