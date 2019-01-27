package syncer

import (
	"context"

	"time"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/discovery"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/utils/contextutils"
)

type syncer struct {
	uds         *discovery.UpstreamDiscovery
	refreshRate time.Duration
	discOpts    discovery.Opts
}

func NewDiscoverySyncer(disc *discovery.UpstreamDiscovery, discOpts discovery.Opts, refreshRate time.Duration) v1.DiscoverySyncer {
	s := &syncer{
		uds:         disc,
		refreshRate: refreshRate,
		discOpts:    discOpts,
	}
	return s
}

func (s *syncer) Sync(ctx context.Context, snap *v1.DiscoverySnapshot) error {
	ctx = contextutils.WithLogger(ctx, "syncer")
	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("begin sync %v (%v upstreams)", snap.Hash(), len(snap.Upstreams.List()))
	defer logger.Infof("end sync %v", snap.Hash())

	logger.Debugf("%v", snap)

	opts := clients.WatchOpts{
		Ctx:         ctx,
		RefreshRate: s.refreshRate,
	}

	udsErrs, err := s.uds.StartUds(opts, s.discOpts)
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case err := <-udsErrs:
				contextutils.LoggerFrom(ctx).Errorf("error in UDS: %v", err)
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}
