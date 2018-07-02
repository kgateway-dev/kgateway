package copilot

import (
	"context"
	"time"

	"code.cloudfoundry.org/copilot"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/config"
	"github.com/solo-io/gloo/pkg/storage"

	"github.com/solo-io/gloo/pkg/plugins/cloudfoundry"
)

const generatedBy = "cloudfoundry-upstream-discovery"

type UpstreamController struct {
	errors chan error

	resyncDuration time.Duration
	ctx            context.Context
	client         copilot.IstioClient

	syncer config.UpstreamSyncer
}

func NewUpstreamController(ctx context.Context, configStore storage.Interface, client copilot.IstioClient, resyncDuration time.Duration) *UpstreamController {
	sc := &UpstreamController{
		errors: make(chan error),

		resyncDuration: resyncDuration,
		ctx:            ctx,
		client:         client,

		syncer: config.UpstreamSyncer{
			Owner:       generatedBy,
			GlooStorage: configStore,
		},
	}
	sc.syncer.DesiredUpstreams = sc.getDesiredUpstreams

	return sc
}

func (sc *UpstreamController) Run(stop <-chan struct{}) {
	cloudfoundry.ResyncLoop(sc.ctx, stop, sc.resync, sc.resyncDuration)
}

func (c *UpstreamController) Error() <-chan error {
	return c.errors
}

func (sc *UpstreamController) getDesiredUpstreams() ([]*v1.Upstream, error) {
	return cloudfoundry.GetUpstreams(sc.ctx, sc.client)
}

func (sc *UpstreamController) resync() {
	err := sc.resyncWithError()
	if err != nil {
		sc.errors <- err
	}
}

func (sc *UpstreamController) resyncWithError() error {
	return sc.syncer.SyncDesiredState()
}
