package syncer

import (
	"context"

	"time"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/utils/contextutils"
)

type syncer struct {
	uds         *discovery.UpstreamDiscovery
	refreshRate time.Duration
}

func NewDiscoverySyncer(disc *discovery.UpstreamDiscovery, refreshRate time.Duration) v1.DiscoverySyncer {
	s := &syncer{
		uds:         disc,
		refreshRate: refreshRate,
	}
	return s
}

func (s *syncer) Sync(ctx context.Context, snap *v1.DiscoverySnapshot) error {
	ctx = contextutils.WithLogger(ctx, "syncer")
	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("begin sync %v (%v upstreams)", snap.Hash(), len(snap.Upstreams.List()))
	defer logger.Infof("end sync %v", snap.Hash())

	logger.Debugf("%v", snap)

	// kick the uds, ensure that desired upstreams are in sync
	return s.uds.Resync(ctx)
}
