package cloudfoundry

import (
	"context"
	"time"

	"code.cloudfoundry.org/copilot"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/config"
	"github.com/solo-io/gloo/pkg/storage"
)

const generatedBy = "cloudfoundry-upstream-discovery"

type ServiceController struct {
	errors chan error

	resyncDuration time.Duration
	ctx            context.Context
	client         copilot.IstioClient

	syncer config.UpstreamSyncer
}

func NewServiceController(ctx context.Context, configStore storage.Interface, client copilot.IstioClient, resyncDuration time.Duration) *ServiceController {
	sc := &ServiceController{
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

func (sc *ServiceController) Run(stop <-chan struct{}) {
	ResyncLoop(sc.ctx, stop, sc.resync, sc.resyncDuration)
}

func (c *ServiceController) Error() <-chan error {
	return c.errors
}

func (sc *ServiceController) getDesiredUpstreams() ([]*v1.Upstream, error) {
	return GetUpstreams(sc.ctx, sc.client)
}

func (sc *ServiceController) resync() {
	err := sc.resyncWithError()
	if err != nil {
		sc.errors <- err
	}
}

func (sc *ServiceController) resyncWithError() error {
	return sc.syncer.SyncDesiredState()
}
